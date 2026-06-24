package main

//16_
import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/asbm77/go-musthave-shortener-tpl/internal/audit"
	"github.com/asbm77/go-musthave-shortener-tpl/internal/logger"
	"github.com/asbm77/go-musthave-shortener-tpl/internal/middleware"
	"github.com/asbm77/go-musthave-shortener-tpl/internal/storage"
	"github.com/asbm77/go-musthave-shortener-tpl/internal/worker"
	"github.com/go-chi/chi/v5"
)

type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
	bodySize   int
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
func (rw *responseWriterWrapper) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.bodySize += size
	return size, err
}

// LoggingMiddleware теперь использует pkg/logger
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrappedWriter := &responseWriterWrapper{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(wrappedWriter, r)

		latency := time.Since(start)

		// Используем структурированное логирование из нашего пакета
		logger.Logger.Infow("Request processed",
			"uri", r.RequestURI,
			"method", r.Method,
			"latency", latency,
			"status", wrappedWriter.statusCode,
			"size", wrappedWriter.bodySize,
		)
	})
}

func createStorage() (storage.Storage, error) {
	// Приоритет 1: PostgreSQL
	if flagConnDB != "" {
		log.Println("Using PostgreSQL storage")
		pgStorage, err := storage.NewPostgresStorage(flagConnDB)
		if err != nil {
			return nil, err
		}

		err = pgStorage.RunMigrations()
		if err != nil {
			log.Fatalf("Ошибка выполнения миграций: %v", err)
		}

		log.Println("Миграции выполнены успешно")

		return pgStorage, nil
	}

	// Приоритет 2: Файловое хранилище
	if flagFileBD != "" {
		log.Println("Using file storage")
		fileStorage := storage.NewFileStorage(flagFileBD)
		if err := fileStorage.LoadFromFile(); err != nil {
			log.Printf("Warning: failed to load from file: %v", err)
		}
		return fileStorage, nil
	}

	// Приоритет 3: Память
	log.Println("Using in-memory storage")
	return storage.NewInMemoryStorage(), nil
}

func setupAudit() *audit.Manager {
	auditManager := audit.NewManager()

	// Настройка файлового наблюдателя
	if flagAuditFile != "" {
		fileObserver, err := audit.NewFileObserver(flagAuditFile)
		if err != nil {
			log.Printf("Warning: failed to create file observer: %v", err)
		} else {
			auditManager.Register(fileObserver)
			log.Printf("Audit file logging enabled: %s", flagAuditFile)
		}
	}

	// Настройка HTTP наблюдателя
	if flagAuditURL != "" {
		httpObserver := audit.NewHTTPObserver(flagAuditURL)
		auditManager.Register(httpObserver)
		log.Printf("Audit HTTP logging enabled: %s", flagAuditURL)
	}

	if flagAuditFile == "" && flagAuditURL == "" {
		log.Println("Audit is disabled")
	}

	return auditManager
}

func main() {
	parseFlags()
	SetEnableAuth(flagEnableAuth)

	store, err := createStorage()
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	deleteManager := worker.NewDeleteManager(store, flagDeleteBufferSize, flagDeleteFlushInterval)
	deleteManager.Start()
	defer deleteManager.Stop()

	// Настройка аудита
	auditManager := setupAudit()
	defer auditManager.Close()

	// Настройка graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Println("Завершение программы...")
		os.Exit(0)
	}()

	r := chi.NewRouter()

	r.Use(UnzipMiddleware)
	r.Use(GzipMiddlewareWithContentType)
	r.Use(LoggingMiddleware)

	if flagEnableAuth {
		// Публичные маршруты (без аутентификации)
		r.Get("/ping", apiGetPing(store))
		r.Get("/{id}", redirectHandler(store, auditManager))

		// Защищенные маршруты - применяем AuthMiddleware
		r.Group(func(r chi.Router) {
			r.Use(middleware.AuthMiddleware)
			r.Get("/api/user/urls", apiGetUserURLs(store))
			r.Post("/api/shorten", apiPostShorten(store, auditManager))
			r.Post("/api/shorten/batch", apiPostShortenBatch(store))
			r.Post("/", apiPost(store, auditManager))

			// Новый эндпоинт для удаления
			r.Delete("/api/user/urls", apiDeleteUserURLs(deleteManager))
		})
	} else {
		// Режим совместимости - все маршруты без аутентификации
		r.Get("/ping", apiGetPing(store))
		r.Get("/{id}", redirectHandler(store, auditManager))
		r.Get("/api/user/urls", apiGetUserURLs(store))
		r.Post("/", apiPost(store, auditManager))
		r.Post("/api/shorten", apiPostShorten(store, auditManager))
		r.Post("/api/shorten/batch", apiPostShortenBatch(store))
		r.Delete("/api/user/urls", apiDeleteUserURLs(deleteManager))
	}

	err = http.ListenAndServe(flagRunAddr, r)
	if err != nil {
		panic(err)
	}
}
