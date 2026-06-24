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

	"github.com/asbm77/go-musthave-shortener-tpl/internal/audit"
	"github.com/asbm77/go-musthave-shortener-tpl/internal/logger"
	"github.com/asbm77/go-musthave-shortener-tpl/internal/middleware"
	"github.com/asbm77/go-musthave-shortener-tpl/internal/storage"
	"github.com/asbm77/go-musthave-shortener-tpl/internal/worker"
	"github.com/google/uuid"
)

// Функция для установки флага из main
func SetEnableAuth(enable bool) {
	flagEnableAuth = enable
}

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

func getUserIDFromContext(r *http.Request) (string, error) {
	userID := middleware.GetUserID(r.Context())

	// Для отладки
	log.Printf("[DEBUG] getUserIDFromContext - userID: '%s', auth enabled: %v", userID, flagEnableAuth)

	// Если аутентификация включена, проверяем наличие userID
	if flagEnableAuth {
		if userID == "" {
			return "", fmt.Errorf("unauthorized")
		}
		return userID, nil
	}

	// Если аутентификация выключена, используем anonymous для пустого userID
	if userID == "" {
		userID = "anonymous"
	}

	return userID, nil
}

func apiPost(store storage.Storage, auditManager *audit.Manager) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(res, "400 Bad Request", http.StatusBadRequest)
			return
		}

		userID, err := getUserIDFromContext(req)
		if err != nil {
			http.Error(res, "Unauthorized", http.StatusUnauthorized)
			return
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(res, "400 Bad Request", http.StatusBadRequest)
			return
		}
		defer req.Body.Close()

		originalURL := string(body)

		if originalURL == "" {
			http.Error(res, "Empty URL", http.StatusBadRequest)
			return
		}

		if !isValidURL(originalURL) {
			http.Error(res, "Invalid URL format", http.StatusBadRequest)
			return
		}

		shortKey := uuid.NewString()[:8]

		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()

		resultShortKey, err := store.SaveUserURL(ctx, userID, shortKey, originalURL)
		if err != nil {
			if err == storage.ErrExists {
				existingShortURL := flagShortAddr + "/" + resultShortKey
				res.Header().Set("Content-Type", "text/plain")
				res.WriteHeader(http.StatusConflict)
				fmt.Fprintf(res, "%s", existingShortURL)

				// Отправка аудит-события даже при конфликте (URL уже существует)
				auditManager.NotifyAll(audit.NewEvent(
					audit.ActionShorten,
					userID,
					originalURL,
				))
				return
			}
			log.Printf("[DEBUG] apiPost - SaveUserURL error: %v", err)
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		shortURL := flagShortAddr + "/" + resultShortKey
		res.Header().Set("Content-Type", "text/plain")
		res.WriteHeader(http.StatusCreated)
		fmt.Fprintf(res, "%s", shortURL)

		log.Printf("[DEBUG] apiPost - saved URL for user %s: %s -> %s", userID, originalURL, shortURL)

		// Отправка аудит-события после успешного создания URL
		auditManager.NotifyAll(audit.NewEvent(
			audit.ActionShorten,
			userID,
			originalURL,
		))
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

func apiPostShorten(store storage.Storage, auditManager *audit.Manager) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(res, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Получаем userID из контекста
		userID, err := getUserIDFromContext(req)
		if err != nil {
			http.Error(res, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var sreq ShortenRequest
		err = json.NewDecoder(req.Body).Decode(&sreq)
		if err != nil {
			http.Error(res, "Bad Request: Invalid JSON", http.StatusBadRequest)
			return
		}

		if sreq.URL == "" {
			http.Error(res, "'url' field is required", http.StatusBadRequest)
			return
		}

		if !isValidURL(sreq.URL) {
			http.Error(res, "URL must include protocol (http:// or https://)", http.StatusBadRequest)
			return
		}

		shortKey := uuid.NewString()[:8]

		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()

		resultShortKey, err := store.SaveUserURL(ctx, userID, shortKey, sreq.URL)
		if err != nil {
			if err == storage.ErrExists {
				existingShortURL := flagShortAddr + "/" + resultShortKey
				response := ShortenResponse{Result: existingShortURL}
				res.Header().Set("Content-Type", "application/json")
				res.WriteHeader(http.StatusConflict)
				json.NewEncoder(res).Encode(response)

				// Отправка аудит-события при конфликте (URL уже существует)
				auditManager.NotifyAll(audit.NewEvent(
					audit.ActionShorten,
					userID,
					sreq.URL,
				))
				return
			}
			log.Printf("[DEBUG] apiPostShorten - SaveUserURL error: %v", err)
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		shortURL := flagShortAddr + "/" + resultShortKey
		response := ShortenResponse{Result: shortURL}
		res.Header().Set("Content-Type", "application/json")
		res.WriteHeader(http.StatusCreated)
		json.NewEncoder(res).Encode(response)

		log.Printf("[DEBUG] apiPostShorten - saved URL for user %s: %s -> %s", userID, sreq.URL, shortURL)

		// Отправка аудит-события после успешного создания URL
		auditManager.NotifyAll(audit.NewEvent(
			audit.ActionShorten,
			userID,
			sreq.URL,
		))
	}
}

func apiGetUserURLs(store storage.Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			http.Error(res, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID, err := getUserIDFromContext(req)
		if err != nil {
			http.Error(res, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()

		// Получаем URL пользователя
		urls, err := store.GetUserURLs(ctx, userID)
		if err != nil {
			log.Printf("[DEBUG] GetUserURLs error: %v", err)
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Если URL нет, возвращаем 204 No Content
		if len(urls) == 0 {
			log.Printf("[DEBUG] apiGetUserURLs - no URLs for user %s, returning 204", userID)
			res.WriteHeader(http.StatusNoContent)
			return
		}

		// Формируем полные короткие URL
		response := make([]map[string]string, 0, len(urls))
		for _, url := range urls {
			response = append(response, map[string]string{
				"short_url":    flagShortAddr + "/" + url.ShortURL,
				"original_url": url.OriginalURL,
			})
		}

		res.Header().Set("Content-Type", "application/json")
		res.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(res).Encode(response); err != nil {
			log.Printf("[DEBUG] Error encoding response: %v", err)
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		log.Printf("[DEBUG] apiGetUserURLs - returning %d URLs for user %s", len(response), userID)
	}
}

func redirectHandler(store storage.Storage, auditManager *audit.Manager) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		id := strings.TrimPrefix(req.URL.Path, "/")

		if id == "" || id == "ping" || id == "api" {
			http.NotFound(res, req)
			return
		}

		// Получаем userID из контекста (если есть)
		userID, _ := getUserIDFromContext(req)

		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()

		originalURL, err := store.Get(ctx, id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				http.NotFound(res, req)
				return
			}
			if errors.Is(err, storage.ErrGone) {
				// URL был удален
				res.WriteHeader(http.StatusGone)
				return
			}
			log.Printf("Storage error in redirect: %v", err)
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if originalURL == "" {
			http.NotFound(res, req)
			return
		}

		// Отправка аудит-события после успешного получения URL (перед редиректом)
		auditManager.NotifyAll(audit.NewEvent(
			audit.ActionFollow,
			userID,
			originalURL,
		))

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

		userID, err := getUserIDFromContext(req)
		if err != nil {
			http.Error(res, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var requests []BatchShortenRequest
		err = json.NewDecoder(req.Body).Decode(&requests)
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
				logger.Logger.Warnw("Empty URL in batch",
					"correlation_id", reqItem.CorrelationID,
					"userID", userID)
				continue
			}

			if !isValidURL(reqItem.OriginalURL) {
				logger.Logger.Warnw("Invalid URL in batch",
					"correlation_id", reqItem.CorrelationID,
					"url", reqItem.OriginalURL,
					"userID", userID)
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
			logger.Logger.Errorw("Failed to save batch",
				"error", err,
				"userID", userID,
				"batch_size", len(batchItems))
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Отправляем ответ
		res.Header().Set("Content-Type", "application/json")
		res.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(res).Encode(responses); err != nil {
			logger.Logger.Errorw("Failed to encode batch response",
				"error", err,
				"userID", userID)
			http.Error(res, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		logger.Logger.Infow("Batch processed successfully",
			"total_requests", len(requests),
			"successful", len(batchItems))
	}
}

func apiDeleteUserURLs(deleteManager *worker.DeleteManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URLs []string `json:"urls"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		userID, err := getUserIDFromContext(r)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Асинхронная отправка в буфер
		select {
		case deleteManager.GetQueue() <- worker.DeleteRequest{
			UserID: userID,
			URLs:   req.URLs,
		}:
			w.WriteHeader(http.StatusAccepted) // 202 Accepted
			w.Write([]byte(`{"status":"accepted"}`))
		default:
			http.Error(w, "Server busy, please try again later", http.StatusServiceUnavailable)
		}
	}
}
