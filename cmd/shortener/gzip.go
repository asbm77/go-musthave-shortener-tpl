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
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			// Создаем обертку для ResponseWriter
			gw := gzip.NewWriter(w)
			defer gw.Close()

			// Создаем кастомный ResponseWriter
			gzw := &gzipResponseWriter{Writer: gw, ResponseWriter: w}

			// Устанавливаем заголовок Content-Encoding
			w.Header().Set("Content-Encoding", "gzip")

			// Передаем управление следующему обработчику с нашей оберткой
			next.ServeHTTP(gzw, r)
			return
		}

		// Если клиент не поддерживает сжатие, просто передаем управление
		next.ServeHTTP(w, r)
	})
}

// Вспомогательная структура для обертки ResponseWriter
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter // Исходный ResponseWriter для записи заголовков

	headers     http.Header // Буфер для хранения заголовков
	status      int         // Хранение статуса кода
	wroteHeader bool        // Флаг: были ли записаны заголовки
}

func (w *gzipResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.status = statusCode

	// Копируем все заголовки из исходного ResponseWriter в наш буфер,
	// кроме Content-Length и Content-Encoding, которые мы установим сами.
	w.headers = w.ResponseWriter.Header().Clone()

	w.wroteHeader = true
}

// writeHeadersAndStatus — вспомогательный метод.
// Записывает сохраненные заголовки и статус-код в исходный ResponseWriter.
func (w *gzipResponseWriter) writeHeadersAndStatus() {
	if w.wroteHeader {
		// Записываем статус-код
		w.ResponseWriter.WriteHeader(w.status)

		// Записываем сохраненные заголовки
		dst := w.ResponseWriter.Header()
		for k, vv := range w.headers {
			dst[k] = vv
		}
	}
}

// Write переопределяет стандартный метод записи тела.
// Перед записью тела она гарантирует, что заголовки и статус отправлены.
func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		// Если WriteHeader еще не вызывался, считаем это кодом 200 OK
		w.WriteHeader(http.StatusOK)
	}

	// Перед записью данных в gzip.Writer, записываем заголовки в исходный ResponseWriter
	w.writeHeadersAndStatus()

	return w.Writer.Write(b)
}
