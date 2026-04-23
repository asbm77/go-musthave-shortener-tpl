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

		//url := req.FormValue("url")
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(res, "400 Bad Request", http.StatusBadRequest)
			return
		}

		defer req.Body.Close()

		url := string(body)
		shortUrla := uuid.NewString()[:8]
		shortUrlares := flagShortAddr + "/" + shortUrla

		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()

		if err := store.Set(ctx, shortUrla, url); err != nil {
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return

		}

		res.WriteHeader(http.StatusCreated)
		fmt.Fprintf(res, "%s", shortUrlares)

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

		// 2. Декодируем JSON из тела запроса
		var sreq ShortenRequest
		err := json.NewDecoder(req.Body).Decode(&req)
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
		shortURL := flagShortAddr + "/" + shortKey

		log.Printf("Saving URL for key %q: %q", shortKey, req.URL)

		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()

		// 6. Сохраняем в хранилище
		if err := store.Set(ctx, shortKey, sreq.URL); err != nil {
			log.Printf("Storage error in apiPostShorten: %v", err)
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		log.Printf("URL saved successfully for key %q", shortKey)

		// 7. Формируем и отправляем JSON-ответ
		response := ShortenResponse{Result: shortURL}
		res.Header().Set("Content-Type", "application/json")
		res.WriteHeader(http.StatusCreated)

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

		// Проверка на пустой URL
		if originalURL == "" {
			log.Printf("Empty URL found for key: %s", id)
			http.NotFound(res, req)
			return
		}

		// Дополнительная проверка корректности URL
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

		ctx, cancel := context.WithTimeout(req.Context(), 30)
		defer cancel()

		// Подготавливаем элементы для пакетного сохранения
		batchItems := make([]storage.BatchItem, 0, len(requests))
		responses := make([]BatchShortenResponse, 0, len(requests))

		for _, reqItem := range requests {
			// Валидация URL
			if reqItem.OriginalURL == "" {
				logger.Logger.Warnw("Empty URL in batch", "correlation_id", reqItem.CorrelationID)
				continue
			}

			if !isValidURL(reqItem.OriginalURL) {
				logger.Logger.Warnw("Invalid URL in batch",
					"correlation_id", reqItem.CorrelationID,
					"url", reqItem.OriginalURL)
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
			logger.Logger.Errorw("Failed to save batch", "error", err)
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Отправляем ответ
		res.Header().Set("Content-Type", "application/json")
		res.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(res).Encode(responses); err != nil {
			logger.Logger.Errorw("Failed to encode batch response", "error", err)
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		logger.Logger.Infow("Batch processed successfully",
			"total_requests", len(requests),
			"successful", len(batchItems))
	}
}
