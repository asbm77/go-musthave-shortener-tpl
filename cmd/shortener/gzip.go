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
	http.ResponseWriter
}

// Переопределяем Write для записи в gzip.Writer
func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}
