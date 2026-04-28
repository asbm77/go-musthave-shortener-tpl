package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/asbm77/go-musthave-shortener-tpl/internal/auth"
	"github.com/asbm77/go-musthave-shortener-tpl/internal/logger"
	"github.com/asbm77/go-musthave-shortener-tpl/internal/middleware"
	"github.com/asbm77/go-musthave-shortener-tpl/internal/storage"
	"github.com/go-chi/chi/v5"
)

// Инициализация для тестов
func init() {
	logger.Initialize("error")
	flagShortAddr = "http://localhost:8080"
}

// Тест для responseWriterWrapper
func TestResponseWriterWrapper(t *testing.T) {
	t.Run("WriteHeader sets status code", func(t *testing.T) {
		rec := httptest.NewRecorder()
		wrapper := &responseWriterWrapper{
			ResponseWriter: rec,
			statusCode:     0,
		}

		wrapper.WriteHeader(http.StatusCreated)

		if wrapper.statusCode != http.StatusCreated {
			t.Errorf("Expected status code %d, got %d", http.StatusCreated, wrapper.statusCode)
		}

		if rec.Code != http.StatusCreated {
			t.Errorf("Expected response writer status %d, got %d", http.StatusCreated, rec.Code)
		}
	})

	t.Run("Write accumulates body size", func(t *testing.T) {
		rec := httptest.NewRecorder()
		wrapper := &responseWriterWrapper{
			ResponseWriter: rec,
			bodySize:       0,
		}

		data := []byte("test data")
		n, err := wrapper.Write(data)

		if err != nil {
			t.Errorf("Write returned error: %v", err)
		}

		if n != len(data) {
			t.Errorf("Expected to write %d bytes, wrote %d", len(data), n)
		}

		if wrapper.bodySize != len(data) {
			t.Errorf("Expected body size %d, got %d", len(data), wrapper.bodySize)
		}
	})

	t.Run("Multiple writes accumulate size", func(t *testing.T) {
		rec := httptest.NewRecorder()
		wrapper := &responseWriterWrapper{
			ResponseWriter: rec,
			bodySize:       0,
		}

		wrapper.Write([]byte("first"))
		wrapper.Write([]byte("second"))

		if wrapper.bodySize != 11 {
			t.Errorf("Expected accumulated size 11, got %d", wrapper.bodySize)
		}
	})
}

// Тест для LoggingMiddleware
func TestLoggingMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		handler        http.HandlerFunc
		expectedStatus int
	}{
		{
			name:   "Successful request",
			method: http.MethodGet,
			path:   "/test",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Not found request",
			method: http.MethodGet,
			path:   "/notfound",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:   "Internal error",
			method: http.MethodPost,
			path:   "/error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := LoggingMiddleware(tt.handler)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rec.Code)
			}
		})
	}
}

// Тест для createStorage
func TestCreateStorage(t *testing.T) {
	originalConnDB := flagConnDB
	originalFileBD := flagFileBD
	defer func() {
		flagConnDB = originalConnDB
		flagFileBD = originalFileBD
	}()

	tests := []struct {
		name         string
		connDB       string
		fileBD       string
		expectError  bool
		expectedType string
	}{
		{
			name:         "File storage",
			connDB:       "",
			fileBD:       "/tmp/test-storage.json",
			expectError:  false,
			expectedType: "*storage.FileStorage",
		},
		{
			name:         "Memory storage",
			connDB:       "",
			fileBD:       "",
			expectError:  false,
			expectedType: "*storage.MemoryStorage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flagConnDB = tt.connDB
			flagFileBD = tt.fileBD

			if tt.fileBD != "" {
				os.Remove(tt.fileBD)
			}

			store, err := createStorage()

			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, got %v", err)
			}

			if !tt.expectError && store != nil {
				storeType := ""
				switch store.(type) {
				case *storage.FileStorage:
					storeType = "*storage.FileStorage"
				case *storage.MemoryStorage:
					storeType = "*storage.MemoryStorage"
				case *storage.PostgresStorage:
					storeType = "*storage.PostgresStorage"
				}

				if tt.expectedType != "" && storeType != tt.expectedType {
					t.Errorf("Expected storage type %s, got %s", tt.expectedType, storeType)
				}
				store.Close()
			}

			if tt.fileBD != "" {
				os.Remove(tt.fileBD)
			}
		})
	}
}

// Тест для аутентификации
func TestAuthMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r.Context())
		if userID == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(userID))
	})

	authHandler := middleware.AuthMiddleware(handler)

	t.Run("New user gets cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		authHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}

		cookies := rec.Result().Cookies()
		found := false
		for _, cookie := range cookies {
			if cookie.Name == "user_token" {
				found = true
				break
			}
		}

		if !found {
			t.Error("Expected user_token cookie to be set")
		}
	})

	t.Run("Existing user with valid token", func(t *testing.T) {
		userID := auth.GenerateUserID()
		token, _ := auth.GenerateToken(userID)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{
			Name:  "user_token",
			Value: token,
		})
		rec := httptest.NewRecorder()

		authHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}

		if rec.Body.String() != userID {
			t.Errorf("Expected userID %s, got %s", userID, rec.Body.String())
		}
	})
}

// Тест для пакетного создания URL
func TestBatchCreation(t *testing.T) {
	store := storage.NewInMemoryStorage()

	r := chi.NewRouter()
	r.Use(middleware.AuthMiddleware)
	r.Post("/api/shorten/batch", apiPostShortenBatch(store))

	server := httptest.NewServer(r)
	defer server.Close()

	client := &http.Client{Timeout: 10 * time.Second}

	// Получаем куку через POST запрос
	var authCookie *http.Cookie
	req, err := http.NewRequest(http.MethodPost, server.URL+"/", bytes.NewBufferString("https://init.com"))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to get cookie: %v", err)
	}

	// Получаем куку из ответа
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "user_token" {
			authCookie = cookie
			break
		}
	}
	// Закрываем тело ответа
	resp.Body.Close()

	if authCookie == nil {
		t.Fatal("Failed to get auth cookie")
	}

	batchRequests := []BatchShortenRequest{
		{CorrelationID: "req1", OriginalURL: "https://batch1.com"},
		{CorrelationID: "req2", OriginalURL: "https://batch2.com"},
		{CorrelationID: "req3", OriginalURL: "https://batch3.com"},
	}

	jsonBody, _ := json.Marshal(batchRequests)

	req, err = http.NewRequest(http.MethodPost, server.URL+"/api/shorten/batch", bytes.NewBuffer(jsonBody))
	if err != nil {
		t.Fatalf("Failed to create batch request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(authCookie)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to create batch: %v", err)
	}
	defer resp.Body.Close() // Закрываем тело ответа

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status %d, got %d", http.StatusCreated, resp.StatusCode)
	}

	var responses []BatchShortenResponse
	if err := json.NewDecoder(resp.Body).Decode(&responses); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(responses) != 3 {
		t.Errorf("Expected 3 responses, got %d", len(responses))
	}
}

// Тест для graceful shutdown
func TestGracefulShutdown(t *testing.T) {
	t.Skip("Skipping graceful shutdown test - requires additional setup")
}

// Тест для сигналов
func TestSignalHandling(t *testing.T) {
	sigChan := make(chan os.Signal, 1)

	t.Run("SIGTERM handling", func(t *testing.T) {
		go func() {
			sigChan <- syscall.SIGTERM
		}()

		select {
		case sig := <-sigChan:
			if sig != syscall.SIGTERM {
				t.Errorf("Expected SIGTERM, got %v", sig)
			}
		case <-time.After(1 * time.Second):
			t.Error("Timeout waiting for signal")
		}
	})

	t.Run("SIGINT handling", func(t *testing.T) {
		go func() {
			sigChan <- os.Interrupt
		}()

		select {
		case sig := <-sigChan:
			if sig != os.Interrupt {
				t.Errorf("Expected Interrupt, got %v", sig)
			}
		case <-time.After(1 * time.Second):
			t.Error("Timeout waiting for signal")
		}
	})
}

// Тест для конкурентных запросов
func TestConcurrentRequests(t *testing.T) {
	store := storage.NewInMemoryStorage()

	r := chi.NewRouter()
	r.Use(middleware.AuthMiddleware)
	r.Post("/", apiPost(store))
	r.Get("/{id}", redirectHandler(store))

	server := httptest.NewServer(r)
	defer server.Close()

	concurrency := 20
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(i int) {
			defer func() { done <- true }()

			url := "https://concurrent-test.com/" + string(rune(i+65))

			req, _ := http.NewRequest(http.MethodPost, server.URL+"/", bytes.NewBufferString(url))
			req.Header.Set("Content-Type", "text/plain")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("Request failed: %v", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
				t.Errorf("Expected status %d or %d, got %d",
					http.StatusCreated, http.StatusConflict, resp.StatusCode)
			}
		}(i)
	}

	for i := 0; i < concurrency; i++ {
		<-done
	}

	t.Logf("Successfully processed %d concurrent requests", concurrency)
}

// Тест для API получения URL пользователя
func TestAPIGetUserURLs(t *testing.T) {
	store := storage.NewInMemoryStorage()

	// Создаем тестового пользователя и токен
	userID := auth.GenerateUserID()
	token, err := auth.GenerateToken(userID)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Сохраняем тестовые URL
	ctx := context.Background()
	_, err = store.SaveUserURL(ctx, userID, "abc123", "https://test1.com")
	if err != nil {
		t.Fatalf("Failed to save test URL: %v", err)
	}
	_, err = store.SaveUserURL(ctx, userID, "def456", "https://test2.com")
	if err != nil {
		t.Fatalf("Failed to save test URL: %v", err)
	}

	// Создаем роутер с AuthMiddleware (создает пользователя если нет куки)
	r := chi.NewRouter()
	r.Use(middleware.AuthMiddleware)
	r.Get("/api/user/urls", apiGetUserURLs(store))

	server := httptest.NewServer(r)
	defer server.Close()

	client := &http.Client{Timeout: 10 * time.Second}

	t.Run("Get existing user URLs", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/user/urls", nil)
		req.AddCookie(&http.Cookie{
			Name:  "user_token",
			Value: token,
		})

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			t.Logf("Response body: %s", string(body))
			return
		}

		var urls []map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&urls); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(urls) != 2 {
			t.Errorf("Expected 2 URLs, got %d", len(urls))
		}
	})

	t.Run("User with no URLs returns 204", func(t *testing.T) {
		// Создаем нового пользователя
		newUserID := auth.GenerateUserID()
		newToken, _ := auth.GenerateToken(newUserID)

		req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/user/urls", nil)
		req.AddCookie(&http.Cookie{
			Name:  "user_token",
			Value: newToken,
		})

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		// AuthMiddleware создаст пользователя, но у него нет URL, поэтому должно быть 204
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("Expected status %d, got %d", http.StatusNoContent, resp.StatusCode)
		}
	})

	t.Run("Request without cookie - AuthMiddleware creates new user", func(t *testing.T) {
		// Запрос без куки - AuthMiddleware создаст нового пользователя
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/user/urls", nil)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		// AuthMiddleware создает пользователя, но у него нет URL, поэтому 204
		// (не 401, так как AuthMiddleware автоматически создает пользователя)
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("Expected status %d, got %d", http.StatusNoContent, resp.StatusCode)
		}
	})
}

// Бенчмарки
func BenchmarkLoggingMiddleware(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middlewareLog := LoggingMiddleware(handler)
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		middlewareLog.ServeHTTP(rec, req)
	}
}

func BenchmarkResponseWriterWrapper(b *testing.B) {
	rec := httptest.NewRecorder()
	wrapper := &responseWriterWrapper{
		ResponseWriter: rec,
		statusCode:     0,
		bodySize:       0,
	}

	data := []byte("benchmark test data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wrapper.WriteHeader(http.StatusOK)
		wrapper.Write(data)
	}
}

func BenchmarkAPIEndpoint(b *testing.B) {
	store := storage.NewInMemoryStorage()
	handler := apiPost(store)

	userID := auth.GenerateUserID()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("https://benchmark.com"))
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}
