package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// gzipResponseWriter wraps http.ResponseWriter to support GZIP compression.
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	return g.Writer.Write(b)
}

func UnzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "Invalid gzip data", http.StatusBadRequest)
				return
			}
			defer gz.Close()

			// Заменяем тело запроса на распакованное
			r.Body = gz
			// Удаляем заголовок, чтобы последующие обработчики не пытались распаковать снова
			r.Header.Del("Content-Encoding")
		}

		next.ServeHTTP(w, r)
	})
}

// GzipResponseWriterWithContentType supports content‑type filtering for GZIP compression.
type GzipResponseWriterWithContentType struct {
	http.ResponseWriter
	header http.Header
	writer io.WriteCloser
}

func (g *GzipResponseWriterWithContentType) Write(data []byte) (int, error) {
	if g.writer == nil {
		contentType := g.header.Get("Content-Type")
		// Compress only JSON and HTML content
		if strings.HasPrefix(contentType, "application/json") ||
			strings.HasPrefix(contentType, "text/html") {
			g.header.Set("Content-Encoding", "gzip")
			g.writer = gzip.NewWriter(g.ResponseWriter)
		} else {
			// Write directly if content type is not supported
			return g.ResponseWriter.Write(data)
		}
	}
	return g.writer.Write(data)
}

func (g *GzipResponseWriterWithContentType) WriteHeader(code int) {
	g.ResponseWriter.WriteHeader(code)
}

// GzipMiddlewareWithContentType compresses responses if:
// 1. Client supports GZIP (Accept-Encoding: gzip)
// 2. Content type is application/json or text/html
func GzipMiddlewareWithContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client supports GZIP
		if strings.HasPrefix(r.URL.Path, "/api") {
			next.ServeHTTP(w, r)
			return
		}

		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Wrap the response writer with GZIP support
		gzw := &GzipResponseWriterWithContentType{
			ResponseWriter: w,
			header:         w.Header(),
		}

		defer func() {
			if gzw.writer != nil {
				gzw.writer.Close()
			}
		}()

		next.ServeHTTP(gzw, r)
	})
}
