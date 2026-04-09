package main

//10
import (
	"compress/gzip"
	"net/http"
	"strings"
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

	r.Use(LoggingMiddleware)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Проверяем, поддерживает ли клиент gzip
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				// Если клиент не поддерживает, просто передаем запрос дальше
				next.ServeHTTP(w, r)
				return
			}

			// 2. Создаем обертку, которая перехватит заголовки и статус-код
			gzw := newGzipResponseWriter(w)
			defer gzw.Writer.Close()
			next.ServeHTTP(gzw, r)

			contentType := gzw.Header().Get("Content-Type")

			// Проверяем, является ли тип одним из тех, которые мы хотим сжимать
			if (strings.Contains(contentType, "application/json") ||
				strings.Contains(contentType, "text/html")) &&
				gzw.StatusCode == http.StatusOK { // Сжимаем только успешные ответы

				// Устанавливаем заголовок о сжатии
				gzw.Header().Set("Content-Encoding", "gzip")
				gzw.Header().Del("Content-Length") // Длина после сжатия изменится
			} else {

			}
		})
	})

	r.Get("/{id}", redirectToOriginal(store))
	r.Post("/", apiPost(store))
	r.Post("/api/shorten", apiPostShorten(store))

	err := http.ListenAndServe(flagRunAddr, r)
	if err != nil {
		panic(err)
	}
}

type gzipResponseWriter struct {
	http.ResponseWriter
	Writer       *gzip.Writer
	StatusCode   int         // Для проверки кода ответа (например, 200 OK)
	HeaderBuffer http.Header // Чтобы перехватить заголовки
}

func newGzipResponseWriter(w http.ResponseWriter) *gzipResponseWriter {
	return &gzipResponseWriter{
		ResponseWriter: w,
		Writer:         gzip.NewWriter(w),
		HeaderBuffer:   make(http.Header),
		StatusCode:     http.StatusOK, // По умолчанию считаем ответ успешным
	}
}

// Перехватываем WriteHeader, чтобы сохранить статус-код
func (w *gzipResponseWriter) WriteHeader(code int) {
	w.StatusCode = code

	// Копируем все заголовки в наш буфер
	for k, v := range w.HeaderBuffer {
		w.ResponseWriter.Header()[k] = v
	}

	w.ResponseWriter.WriteHeader(code)
}

// Перехватываем Header(), чтобы сохранять заголовки для анализа после обработки запроса
func (w *gzipResponseWriter) Header() http.Header {
	return w.HeaderBuffer
}

// Метод Write: если сжатие разрешено, пишем в gzip.Writer, иначе - в стандартный ResponseWriter
func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	// Если заголовок Content-Encoding уже установлен (мы решили сжимать),
	// то пишем в gzip.Writer.
	if w.Header().Get("Content-Encoding") == "gzip" {
		return w.Writer.Write(data)
	}

	// Если нет - пишем напрямую в ответ (без сжатия)
	return w.ResponseWriter.Write(data)
}
