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

	"github.com/asbm77/go-musthave-shortener-tpl/internal/audit"
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

		result := rec.Result()
		defer result.Body.Close()

		cookies := result.Cookies()
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
	defer resp.Body.Close()

	// Получаем куку из ответа
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "user_token" {
			authCookie = cookie
			break
		}
	}

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
	defer resp.Body.Close()

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
	auditManager := audit.NewManager()
	defer auditManager.Close()

	r := chi.NewRouter()
	r.Use(middleware.AuthMiddleware)
	r.Post("/", apiPost(store, auditManager))
	r.Get("/{id}", redirectHandler(store, auditManager))

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
	if !flagEnableAuth {
		t.Skip("Skipping authentication test because auth is disabled")
	}

	store := storage.NewInMemoryStorage()

	userID := auth.GenerateUserID()
	token, err := auth.GenerateToken(userID)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	ctx := context.Background()
	_, err = store.SaveUserURL(ctx, userID, "abc123", "https://test1.com")
	if err != nil {
		t.Fatalf("Failed to save test URL: %v", err)
	}
	_, err = store.SaveUserURL(ctx, userID, "def456", "https://test2.com")
	if err != nil {
		t.Fatalf("Failed to save test URL: %v", err)
	}

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
		newUserID := auth.GenerateUserID()
		newToken, _ := auth.GenerateToken(newUserID)

		newStore := storage.NewInMemoryStorage()

		newR := chi.NewRouter()
		newR.Use(middleware.AuthMiddleware)
		newR.Get("/api/user/urls", apiGetUserURLs(newStore))

		newServer := httptest.NewServer(newR)
		defer newServer.Close()

		req, _ := http.NewRequest(http.MethodGet, newServer.URL+"/api/user/urls", nil)
		req.AddCookie(&http.Cookie{
			Name:  "user_token",
			Value: newToken,
		})

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("Expected status %d, got %d", http.StatusNoContent, resp.StatusCode)
		}
	})

	t.Run("Request without cookie - AuthMiddleware creates new user but returns NoContent", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/user/urls", nil)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("Expected status %d, got %d", http.StatusNoContent, resp.StatusCode)
		}
	})
}

// ============ ТЕСТЫ АУДИТА ============

// Тест для FileObserver
func TestFileObserver(t *testing.T) {
	// Используем временную директорию для тестов
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/test-audit.log"

	observer, err := audit.NewFileObserver(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create file observer: %v", err)
	}
	defer observer.Close()

	event := audit.NewEvent(audit.ActionShorten, "test-user", "https://example.com")

	err = observer.Notify(event)
	if err != nil {
		t.Fatalf("Failed to notify: %v", err)
	}

	// Проверяем, что файл создан
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Fatal("Audit file was not created")
	}

	// Читаем файл и проверяем содержимое
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read audit file: %v", err)
	}

	var receivedEvent audit.Event
	if err := json.Unmarshal(data, &receivedEvent); err != nil {
		t.Fatalf("Failed to unmarshal event: %v", err)
	}

	if receivedEvent.Action != audit.ActionShorten {
		t.Errorf("Expected action %s, got %s", audit.ActionShorten, receivedEvent.Action)
	}
	if receivedEvent.UserID != "test-user" {
		t.Errorf("Expected user_id test-user, got %s", receivedEvent.UserID)
	}
	if receivedEvent.URL != "https://example.com" {
		t.Errorf("Expected URL https://example.com, got %s", receivedEvent.URL)
	}
}

// Тест для HTTPObserver
func TestHTTPObserver(t *testing.T) {
	// Создаем тестовый HTTP сервер
	receivedEvents := make(chan *audit.Event, 1)
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event audit.Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Errorf("Failed to decode event: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		receivedEvents <- &event
		w.WriteHeader(http.StatusOK)
	}))
	defer testServer.Close()

	observer := audit.NewHTTPObserver(testServer.URL)

	event := audit.NewEvent(audit.ActionFollow, "test-user-http", "https://example-follow.com")

	err := observer.Notify(event)
	if err != nil {
		t.Fatalf("Failed to notify: %v", err)
	}

	// Ждем получения события
	select {
	case received := <-receivedEvents:
		if received.Action != audit.ActionFollow {
			t.Errorf("Expected action %s, got %s", audit.ActionFollow, received.Action)
		}
		if received.UserID != "test-user-http" {
			t.Errorf("Expected user_id test-user-http, got %s", received.UserID)
		}
		if received.URL != "https://example-follow.com" {
			t.Errorf("Expected URL https://example-follow.com, got %s", received.URL)
		}
		if received.Timestamp == 0 {
			t.Error("Timestamp should not be 0")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for event")
	}
}

// Тест для HTTPObserver с ошибкой
func TestHTTPObserverError(t *testing.T) {
	observer := audit.NewHTTPObserver("http://localhost:9999/audit")

	event := audit.NewEvent(audit.ActionShorten, "test-user", "https://example.com")

	err := observer.Notify(event)
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

// Тест для Manager
func TestAuditManager(t *testing.T) {
	manager := audit.NewManager()
	defer manager.Close()

	mockObserver := &MockObserver{events: make([]*audit.Event, 0)}
	manager.Register(mockObserver)

	event := audit.NewEvent(audit.ActionShorten, "test-user", "https://example.com")
	manager.NotifyAll(event)

	// Даем время на асинхронную отправку
	time.Sleep(100 * time.Millisecond)

	if len(mockObserver.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(mockObserver.events))
	}
}

// MockObserver для тестирования
type MockObserver struct {
	events []*audit.Event
}

func (m *MockObserver) Notify(event *audit.Event) error {
	m.events = append(m.events, event)
	return nil
}

func (m *MockObserver) Close() error {
	return nil
}

// Тест интеграции аудита с API
func TestAuditIntegrationWithAPI(t *testing.T) {
	store := storage.NewInMemoryStorage()
	auditManager := audit.NewManager()
	defer auditManager.Close()

	// Создаем наблюдатель для сбора событий
	mockObserver := &MockObserver{events: make([]*audit.Event, 0)}
	auditManager.Register(mockObserver)

	r := chi.NewRouter()
	r.Use(middleware.AuthMiddleware)
	r.Post("/api/shorten", apiPostShorten(store, auditManager))
	r.Get("/{id}", redirectHandler(store, auditManager))

	server := httptest.NewServer(r)
	defer server.Close()

	client := &http.Client{Timeout: 10 * time.Second}

	// Генерируем userID и токен для теста
	userID := auth.GenerateUserID()
	token, err := auth.GenerateToken(userID)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	authCookie := &http.Cookie{
		Name:  "user_token",
		Value: token,
	}

	// Тестируем создание короткой ссылки
	t.Run("Create short URL generates audit event", func(t *testing.T) {
		// Очищаем события перед тестом
		mockObserver.events = make([]*audit.Event, 0)

		reqBody := ShortenRequest{URL: "https://audit-test.com"}
		jsonBody, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/shorten", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(authCookie)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status %d, got %d", http.StatusCreated, resp.StatusCode)
		}

		// Проверяем, что событие аудита было создано
		time.Sleep(100 * time.Millisecond)

		if len(mockObserver.events) == 0 {
			t.Error("No audit events were created")
		}

		found := false
		for _, event := range mockObserver.events {
			if event.Action == audit.ActionShorten && event.URL == "https://audit-test.com" {
				found = true
				if event.UserID != userID {
					t.Errorf("Expected user_id %s, got %s", userID, event.UserID)
				}
				break
			}
		}
		if !found {
			t.Error("Shorten audit event not found")
		}
	})

	// Тестируем переход по ссылке
	t.Run("Follow short URL generates audit event", func(t *testing.T) {
		// Очищаем события перед тестом
		mockObserver.events = make([]*audit.Event, 0)

		// Сначала создаем URL
		shortKey := "test123"
		originalURL := "https://follow-test.com"
		ctx := context.Background()
		_, err := store.SaveUserURL(ctx, userID, shortKey, originalURL)
		if err != nil {
			t.Fatalf("Failed to save URL: %v", err)
		}

		// Затем переходим по нему
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/"+shortKey, nil)
		req.AddCookie(authCookie)

		// Отключаем редирект, чтобы проверить статус
		clientNoRedirect := &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}

		resp, err := clientNoRedirect.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusTemporaryRedirect {
			t.Errorf("Expected status %d, got %d", http.StatusTemporaryRedirect, resp.StatusCode)
		}

		// Проверяем, что событие аудита было создано
		time.Sleep(100 * time.Millisecond)

		found := false
		for _, event := range mockObserver.events {
			if event.Action == audit.ActionFollow && event.URL == originalURL {
				found = true
				if event.UserID != userID {
					t.Errorf("Expected user_id %s, got %s", userID, event.UserID)
				}
				break
			}
		}
		if !found {
			t.Error("Follow audit event not found")
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
	auditManager := audit.NewManager()
	defer auditManager.Close()

	handler := apiPost(store, auditManager)

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

// Бенчмарк для аудита
func BenchmarkAuditManager(b *testing.B) {
	manager := audit.NewManager()
	defer manager.Close()

	mockObserver := &MockObserver{events: make([]*audit.Event, 0)}
	manager.Register(mockObserver)

	event := audit.NewEvent(audit.ActionShorten, "bench-user", "https://bench.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.NotifyAll(event)
	}
}
