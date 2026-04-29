package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/asbm77/go-musthave-shortener-tpl/internal/logger"
	"github.com/asbm77/go-musthave-shortener-tpl/internal/middleware"
	"github.com/asbm77/go-musthave-shortener-tpl/internal/storage"
	"github.com/google/uuid"
)

type ShortenRequest struct {
	URL string `json:"url"`
}

type ShortenResponse struct {
	Result string `json:"result"`
}

type BatchShortenRequest struct {
	CorrelationID string `json:"correlation_id"`
	OriginalURL   string `json:"original_url"`
}

// BatchShortenResponse представляет ответ на пакетное сокращение URL
type BatchShortenResponse struct {
	CorrelationID string `json:"correlation_id"`
	ShortURL      string `json:"short_url"`
}

func apiPost(store storage.Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(res, "400 Bad Request", http.StatusBadRequest)
			return
		}

		userID := middleware.GetUserID(req.Context())

		// Если userID пустой и аутентификация выключена, используем временный ID
		if userID == "" {
			// Для обратной совместимости с тестами итерации 1
			userID = "anonymous"
		}

		//url := req.FormValue("url")
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(res, "400 Bad Request", http.StatusBadRequest)
			return
		}

		defer req.Body.Close()

		url := string(body)
		shortUrla := uuid.NewString()[:8]
		//shortUrlares := flagShortAddr + "/" + shortUrla

		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()

		resultShortKey, err := store.SaveUserURL(ctx, userID, shortUrla, url)
		if err != nil {
			if err == storage.ErrExists {
				// URL уже существует - возвращаем 409 Conflict
				existingShortURL := flagShortAddr + "/" + resultShortKey
				res.Header().Set("Content-Type", "text/plain")
				res.WriteHeader(http.StatusConflict)
				fmt.Fprintf(res, "%s", existingShortURL)
				return
			}
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Успешно создан новый URL
		shortURL := flagShortAddr + "/" + resultShortKey
		res.WriteHeader(http.StatusCreated)
		fmt.Fprintf(res, "%s", shortURL)

	}
}

func apiGet(store storage.Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {

		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()

		id := req.URL.Path

		originalURL, err := store.Get(ctx, id)
		if err != nil || originalURL == "" {
			if err == storage.ErrNotFound {
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

func apiGetPing(store storage.Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()

		if err := store.Ping(ctx); err != nil {
			res.WriteHeader(http.StatusInternalServerError)
			return
		}
		res.WriteHeader(http.StatusOK)
	}
}

func apiPostShorten(store storage.Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		// 1. Проверяем метод запроса
		if req.Method != http.MethodPost {
			http.Error(res, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID := middleware.GetUserID(req.Context())

		// Если userID пустой и аутентификация выключена, используем временный ID
		if userID == "" {
			// Для обратной совместимости с тестами итерации 1
			userID = "anonymous"
		}

		// 2. Декодируем JSON из тела запроса
		var sreq ShortenRequest
		err := json.NewDecoder(req.Body).Decode(&sreq)
		if err != nil {
			log.Printf("JSON decode error: %v", err)
			http.Error(res, "Bad Request: Invalid JSON", http.StatusBadRequest)
			return
		}

		// 3. Проверяем наличие URL
		if sreq.URL == "" {
			http.Error(res, "'url' field is required", http.StatusBadRequest)
			return
		}

		// 4. Валидация URL: должен содержать схему (http:// или https://)
		if !isValidURL(sreq.URL) {
			http.Error(res, "URL must include protocol (http:// or https://)", http.StatusBadRequest)
			return
		}

		shortKey := uuid.NewString()[:8]
		//shortURL := flagShortAddr + "/" + shortKey

		log.Printf("Saving URL for key %q: %q", shortKey, sreq.URL)

		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()

		// 6. Сохраняем в хранилище
		resultShortKey, err := store.SaveUserURL(ctx, userID, shortKey, sreq.URL)
		if err != nil {
			if err == storage.ErrExists {
				// URL уже существует - возвращаем 409 Conflict
				existingShortURL := flagShortAddr + "/" + resultShortKey
				response := ShortenResponse{Result: existingShortURL}
				res.Header().Set("Content-Type", "application/json")
				res.WriteHeader(http.StatusConflict)
				json.NewEncoder(res).Encode(response)
				return
			}
			if logger.Logger != nil {
				logger.Logger.Errorw("Storage error in apiPostShorten", "error", err)
			}
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Успешно создан новый URL
		shortURL := flagShortAddr + "/" + resultShortKey
		response := ShortenResponse{Result: shortURL}
		res.Header().Set("Content-Type", "application/json")
		res.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(res).Encode(response); err != nil {
			if logger.Logger != nil {
				logger.Logger.Errorw("Error encoding response", "error", err)
			}
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
}

func apiGetUserURLs(store storage.Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		// Проверяем метод
		if req.Method != http.MethodGet {
			http.Error(res, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Получаем userID из контекста
		userID := middleware.GetUserID(req.Context())

		if userID == "" || userID == "anonymous" {
			http.Error(res, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()

		// Получаем URL пользователя
		urls, err := store.GetUserURLs(ctx, userID)
		if err != nil {
			log.Printf("Failed to get user URLs: %v", err)
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Если URL нет, возвращаем 204
		if len(urls) == 0 {
			res.WriteHeader(http.StatusNoContent)
			return
		}

		// Формируем полные короткие URL
		response := make([]map[string]string, len(urls))
		for i, url := range urls {
			response[i] = map[string]string{
				"short_url":    flagShortAddr + "/" + url.ShortURL,
				"original_url": url.OriginalURL,
			}
		}

		res.Header().Set("Content-Type", "application/json")
		res.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(res).Encode(response); err != nil {
			log.Printf("Error encoding response: %v", err)
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
}

func redirectHandler(store storage.Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		id := strings.TrimPrefix(req.URL.Path, "/")

		if id == "" {
			http.NotFound(res, req)
			return
		}

		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()

		originalURL, err := store.Get(ctx, id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				http.NotFound(res, req)
				return
			}
			log.Printf("Storage error in redirect: %v", err)
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if originalURL == "" {
			log.Printf("Empty URL found for key: %s", id)
			http.NotFound(res, req)
			return
		}

		if !isValidURL(originalURL) {
			log.Printf("Invalid URL format in storage for key %s: %s", id, originalURL)
			http.Error(res, "Invalid redirect URL", http.StatusInternalServerError)
			return
		}

		http.Redirect(res, req, originalURL, http.StatusTemporaryRedirect)
	}
}

func isValidURL(urlString string) bool {
	parsed, err := url.Parse(urlString)
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

func apiPostShortenBatch(store storage.Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(res, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID := middleware.GetUserID(req.Context())

		// Если userID пустой и аутентификация выключена, используем временный ID
		if userID == "" {
			// Для обратной совместимости с тестами итерации 1
			userID = "anonymous"
		}

		var requests []BatchShortenRequest
		err := json.NewDecoder(req.Body).Decode(&requests)
		if err != nil {
			logger.Logger.Errorw("Failed to decode batch request", "error", err)
			http.Error(res, "Bad Request: Invalid JSON", http.StatusBadRequest)
			return
		}

		if len(requests) == 0 {
			http.Error(res, "Bad Request: Empty batch", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
		defer cancel()

		// Подготавливаем элементы для пакетного сохранения
		batchItems := make([]storage.BatchItem, 0, len(requests))
		responses := make([]BatchShortenResponse, 0, len(requests))

		for _, reqItem := range requests {
			// Валидация URL
			if reqItem.OriginalURL == "" {
				if logger.Logger != nil {
					logger.Logger.Warnw("Empty URL in batch",
						"correlation_id", reqItem.CorrelationID,
						"userID", userID)
				}
				continue
			}

			if !isValidURL(reqItem.OriginalURL) {
				if logger.Logger != nil {
					logger.Logger.Warnw("Invalid URL in batch",
						"correlation_id", reqItem.CorrelationID,
						"url", reqItem.OriginalURL,
						"userID", userID)
				}
				continue
			}

			// Генерируем короткий ключ
			shortKey := uuid.NewString()[:8]
			shortURL := flagShortAddr + "/" + shortKey

			// Добавляем в список для пакетного сохранения
			batchItems = append(batchItems, storage.BatchItem{
				CorrelationID: reqItem.CorrelationID,
				ShortURL:      shortKey,
				OriginalURL:   reqItem.OriginalURL,
				UserID:        userID,
			})

			responses = append(responses, BatchShortenResponse{
				CorrelationID: reqItem.CorrelationID,
				ShortURL:      shortURL,
			})
		}

		// Проверяем, что есть хотя бы один элемент для сохранения
		if len(batchItems) == 0 {
			http.Error(res, "Bad Request: No valid URLs to process", http.StatusBadRequest)
			return
		}

		// Сохраняем все элементы одной транзакцией/операцией
		if err := store.SaveBatch(ctx, batchItems); err != nil {
			if logger.Logger != nil {
				logger.Logger.Errorw("Failed to save batch",
					"error", err,
					"userID", userID,
					"batch_size", len(batchItems))
			}
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Отправляем ответ
		res.Header().Set("Content-Type", "application/json")
		res.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(res).Encode(responses); err != nil {
			if logger.Logger != nil {
				logger.Logger.Errorw("Failed to encode batch response",
					"error", err,
					"userID", userID)
			}
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		logger.Logger.Infow("Batch processed successfully",
			"total_requests", len(requests),
			"successful", len(batchItems))
	}
}
