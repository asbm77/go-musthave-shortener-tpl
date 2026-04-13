package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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

		if req.URL == "" {
			http.Error(w, "'url' field is required", http.StatusBadRequest)
			return
		}

		// Валидация URL: должен содержать схему (http:// или https://)
		if !isValidURL(req.URL) {
			http.Error(w, "URL must include protocol (http:// or https://)", http.StatusBadRequest)
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

func redirectHandler(store Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path

		if id == "" {
			http.NotFound(w, r)
			return
		}

		originalURL, err := store.Get(id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			log.Printf("Storage error in redirect: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Проверка на пустой URL
		if originalURL == "" {
			log.Printf("Empty URL found for key: %s", id)
			http.NotFound(w, r)
			return
		}

		// Дополнительная проверка корректности URL
		if !isValidURL(originalURL) {
			log.Printf("Invalid URL format in storage for key %s: %s", id, originalURL)
			http.Error(w, "Invalid redirect URL", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, originalURL, http.StatusTemporaryRedirect)
	}
}

func isValidURL(urlString string) bool {
	parsed, err := url.Parse(urlString)
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}
