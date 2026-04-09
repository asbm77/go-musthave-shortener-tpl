package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

func gzipRequestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Проверяем, что тело запроса сжато gzip
		if r.Header.Get("Content-Encoding") == "gzip" {
			// Проверяем, что тип контента один из поддерживаемых
			contentType := r.Header.Get("Content-Type")
			if strings.Contains(contentType, "application/json") ||
				strings.Contains(contentType, "text/html") {

				gzipReader, err := gzip.NewReader(r.Body)
				if err != nil {
					http.Error(w, "Bad Request: Invalid gzip data", http.StatusBadRequest)
					return
				}
				defer gzipReader.Close()

				// Заменяем тело запроса на распакованное
				r.Body = io.NopCloser(gzipReader)
			}
		}
		// Передаем управление следующему обработчику
		next.ServeHTTP(w, r)
	})
}

func gzipResponseMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Проверяем, поддерживает ли клиент gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Создаем нашу обертку. writerFunc здесь — это конструктор gzip.NewWriter.
		gzw := &gzipResponseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
			writerFunc:     func(inner io.Writer) io.Writer { return gzip.NewWriter(inner) },
			// По умолчанию пишем в оригинальный ResponseWriter
			writer: w,
		}

		// Передаем управление следующему хендлеру с нашей оберткой
		next.ServeHTTP(gzw, r)
	})
}

// gzipResponseWriter — обёртка для сжатия ответа.
type gzipResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	headers     http.Header
	disableGzip bool

	writerFunc func(io.Writer) io.Writer

	writer io.Writer
}

// WriteHeader сохраняет заголовки и статус.
func (w *gzipResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.status = statusCode
	w.headers = w.ResponseWriter.Header().Clone()
	w.wroteHeader = true
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	// Случай 1: Сжатие отключено (например, для редиректа) или тело пустое.
	if w.disableGzip || len(b) == 0 {
		if !w.wroteHeader {
			w.WriteHeader(http.StatusOK)
		}
		// Пишем напрямую в исходный ResponseWriter, без всякого gzip.
		w.ResponseWriter.WriteHeader(w.status)
		return w.ResponseWriter.Write(b)
	}

	if w.writer == w.ResponseWriter {
		// Создаем gzip.Writer, который будет писать в исходный ResponseWriter.
		gw := w.writerFunc(w.ResponseWriter)
		w.writer = gw

		// Устанавливаем заголовок о сжатии.
		w.ResponseWriter.Header().Set("Content-Encoding", "gzip")

		// Записываем статус-код и накопленные заголовки.
		w.ResponseWriter.WriteHeader(w.status)
	}

	// Пишем сжатые данные в наш (теперь уже точно) gzip.Writer.
	return w.writer.Write(b)
}
