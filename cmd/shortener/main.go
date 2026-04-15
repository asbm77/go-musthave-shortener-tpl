package main

//10
import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/asbm77/go-musthave-shortener-tpl/internal/logger"
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

func main() {
	parseFlags()

	store := NewInMemoryStorage()

	if err := store.LoadFromFile(flagFileBD); err != nil {
		log.Fatalf("Ошибка загрузки данных: %v", err)
	}

	// Обработчик завершения работы — сохраняем данные перед выходом
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Println("Получен сигнал завершения, сохраняем данные...")
		if err := store.SaveToFile(flagFileBD); err != nil {
			log.Printf("Ошибка сохранения данных: %v", err)
		} else {
			log.Println("Данные успешно сохранены")
		}
		os.Exit(0)
	}()

	r := chi.NewRouter()

	r.Use(UnzipMiddleware)
	r.Use(GzipMiddlewareWithContentType)
	r.Use(LoggingMiddleware)

	r.Get("/{id}", redirectHandler(store))
	r.Get("/ping", apiGetPing(store))

	r.Post("/api/shorten", apiPostShorten(store))
	r.Post("/", apiPost(store))

	err := http.ListenAndServe(flagRunAddr, r)
	if err != nil {
		panic(err)
	}
}
