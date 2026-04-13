package main

//10
import (
	"net/http"
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

	r := chi.NewRouter()

	r.Use(UnzipMiddleware)
	r.Use(GzipMiddlewareWithContentType)
	r.Use(LoggingMiddleware)

	r.Get("/{id}", redirectHandler(store))

	r.Post("/api/shorten", apiPostShorten(store))
	r.Post("/", apiPost(store))

	err := http.ListenAndServe(flagRunAddr, r)
	if err != nil {
		panic(err)
	}
}
