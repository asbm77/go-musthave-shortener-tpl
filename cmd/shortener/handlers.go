package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	//"shortener/storage"

	"github.com/google/uuid"
)

type ShortenRequest struct {
	URL string `json:"url"`
}

type ShortenResponse struct {
	Result string `json:"result"`
}

func apiPost(store Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(res, "400 Bad Request", http.StatusBadRequest)
			return
		}

		//url := req.FormValue("url")
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(res, "400 Bad Request", http.StatusBadRequest)
			return
		}

		defer req.Body.Close()

		url := string(body)
		shortUrla := "/" + uuid.NewString()[:8]
		//urlMap[shortUrla] = url
		shortUrlares := flagShortAddr + shortUrla

		if err := store.Set(shortUrla, url); err != nil {
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return

		}

		res.WriteHeader(http.StatusCreated)
		fmt.Fprintf(res, "%s", shortUrlares)

	}
}

func apiGet(store Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {

		id := req.URL.Path

		originalURL, err := store.Get(id)
		if err != nil || originalURL == "" {
			if err == ErrNotFound {
				http.NotFound(res, req)
				return
			}
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			log.Printf("Ошибка получения из хранилища: %v", err)
			return
		}

		http.Redirect(res, req, originalURL, http.StatusTemporaryRedirect)
	}
}

func apiPostShorten(store Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Проверяем метод запроса
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 2. Декодируем JSON из тела запроса
		var req ShortenRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil || req.URL == "" {
			http.Error(w, "Bad Request: Invalid JSON or missing 'url'", http.StatusBadRequest)
			return
		}

		// 3. Генерируем короткий ключ и полный URL
		shortKey := "/" + uuid.NewString()[:8]
		shortURL := flagShortAddr + shortKey

		// 4. Сохраняем в хранилище
		if err := store.Set(shortKey, req.URL); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			log.Printf("Ошибка сохранения в хранилище: %v", err)
			return
		}

		// 5. Формируем и отправляем JSON-ответ
		response := ShortenResponse{Result: shortURL}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			log.Printf("Ошибка кодирования ответа: %v", err)
			return
		}
	}
}

func redirectToOriginal(store Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {

		//id := chi.URLParam(r, "*")
		id := req.URL.Path

		originalURL, err := store.Get(id)
		if err != nil {
			http.NotFound(res, req)
			return
		}

		if gzw, ok := res.(*gzipResponseWriter); ok {
			// Если да, отключаем для него сжатие
			gzw.disableGzip = true
		}

		http.Redirect(res, req, originalURL, http.StatusTemporaryRedirect)
	}
}
