package main

//11_
import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/asbm77/go-musthave-shortener-tpl/internal/logger"
	"github.com/asbm77/go-musthave-shortener-tpl/internal/storage"
	"github.com/go-chi/chi/v5"
)

import (
	"syscall"
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

func main() {
	parseFlags()

	store, err := createStorage()
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Настройка graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	r := chi.NewRouter()

	r.Use(UnzipMiddleware)
	r.Use(GzipMiddlewareWithContentType)
	r.Use(LoggingMiddleware)

	r.Get("/{id}", redirectHandler(store))
	r.Get("/ping", apiGetPing(store))

	r.Post("/api/shorten", apiPostShorten(store))
	r.Post("/", apiPost(store))

	err = http.ListenAndServe(flagRunAddr, r)
	if err != nil {
		panic(err)
	}
}
