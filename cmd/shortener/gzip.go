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

// gzipResponseWriter — обёртка для сжатия ответа.
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter

	headers     http.Header
	status      int
	wroteHeader bool
	disableGzip bool // Флаг для отключения сжатия
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

// Write переопределяет запись тела.
func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	// Если сжатие отключено или тело пустое, пишем напрямую в исходный ResponseWriter.
	if w.disableGzip || len(b) == 0 {
		if !w.wroteHeader {
			w.WriteHeader(http.StatusOK)
		}
		// Записываем сохраненные заголовки и статус
		w.ResponseWriter.WriteHeader(w.status)
		for k, vv := range w.headers {
			w.ResponseWriter.Header()[k] = vv
		}
		return w.ResponseWriter.Write(b)
	}

	// Если мы дошли сюда, значит есть тело и его нужно сжать.
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}

	// Устанавливаем заголовок о сжатии перед отправкой данных
	w.ResponseWriter.Header().Set("Content-Encoding", "gzip")
	// Записываем статус и остальные заголовки
	w.ResponseWriter.WriteHeader(w.status)

	return w.Writer.Write(b)
}
