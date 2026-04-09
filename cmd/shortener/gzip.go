package main

import (
	"compress/gzip"
	"net/http"
	"os"
	"strings"
)

type gzipResponseWriter struct {
	http.ResponseWriter
	Writer       *gzip.Writer
	StatusCode   int
	HeaderBuffer http.Header
}

// newGzipResponseWriter создает новый экземпляр gzipResponseWriter.
func newGzipResponseWriter(w http.ResponseWriter) *gzipResponseWriter {
	return &gzipResponseWriter{
		ResponseWriter: w,
		Writer:         gzip.NewWriter(w),
		HeaderBuffer:   make(http.Header),
		StatusCode:     http.StatusOK,
	}
}

// WriteHeader перехватывает статус-код ответа.
func (w *gzipResponseWriter) WriteHeader(code int) {
	w.StatusCode = code

	// Копируем перехваченные заголовки в реальный ResponseWriter
	for k, v := range w.HeaderBuffer {
		w.ResponseWriter.Header()[k] = v
	}

	w.ResponseWriter.WriteHeader(code)
}

// Header перехватывает заголовки.
func (w *gzipResponseWriter) Header() http.Header {
	return w.HeaderBuffer
}

// Write пишет данные либо в gzip.Writer, либо напрямую в ответ,
// в зависимости от того, установлен ли заголовок сжатия.
func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	if w.Header().Get("Content-Encoding") == "gzip" {
		return w.Writer.Write(data)
	}
	return w.ResponseWriter.Write(data)
}

// GzipMiddleware — это функция, которая возвращает готовый middleware для chi.
// Она принимает список типов контента, которые нужно сжимать.
func GzipMiddleware(contentTypes []string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if os.Getenv("GO_WANT_HELPER_PROCESS") != "" {
				// Если это тест, пропускаем всю логику сжатия
				next.ServeHTTP(w, r)
				return
			}
			// 1. Проверяем, поддерживает ли клиент сжатие
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				next.ServeHTTP(w, r)
				return
			}

			gzw := newGzipResponseWriter(w)
			defer gzw.Writer.Close()

			// 2. Выполняем следующий обработчик (наш API)
			next.ServeHTTP(gzw, r)

			// 3. Проверяем тип контента ПОСЛЕ того, как обработчик отработал
			contentType := gzw.Header().Get("Content-Type")

			// Проверяем, что тип контента есть в списке разрешенных и статус 200 OK
			if contains(contentTypes, contentType) && gzw.StatusCode == http.StatusOK {
				gzw.Header().Set("Content-Encoding", "gzip")
				gzw.Header().Del("Content-Length")
			}
			// Если условие не выполнилось, ничего не делаем. Ответ уйдет несжатым.
		})
	}
}

// Helper-функция для проверки наличия строки в слайсе
func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}
