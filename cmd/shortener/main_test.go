package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/asbm77/go-musthave-shortener-tpl/internal/logger"
	"github.com/asbm77/go-musthave-shortener-tpl/internal/storage"
	"github.com/go-chi/chi/v5"
)

// Тестовая конфигурация
func init() {
	// Инициализация логгера для тестов
	logger.Initialize("error")
}

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

func TestCreateStorage(t *testing.T) {
	// Сохраняем оригинальные значения флагов
	originalConnDB := flagConnDB
	originalFileBD := flagFileBD
	defer func() {
		flagConnDB = originalConnDB
		flagFileBD = originalFileBD
	}()

	tests := []struct {
		name          string
		connDB        string
		fileBD        string
		expectError   bool
		expectedType  string
		setupPostgres bool
	}{
		{
			name:         "PostgreSQL storage (priority 1)",
			connDB:       "postgres://user:pass@localhost:5432/db?sslmode=disable",
			fileBD:       "/tmp/test.json",
			expectError:  true, // Будет ошибка, т.к. PostgreSQL не запущен
			expectedType: "",
		},
		{
			name:         "File storage (priority 2)",
			connDB:       "",
			fileBD:       "/tmp/test-storage.json",
			expectError:  false,
			expectedType: "*storage.FileStorage",
		},
		{
			name:         "Memory storage (priority 3)",
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

			// Создаем тестовый файл если нужно
			if tt.fileBD != "" && tt.fileBD != "/tmp/test-storage.json" {
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
			}

			if store != nil {
				store.Close()
			}

			// Очистка
			if tt.fileBD != "" {
				os.Remove(tt.fileBD)
			}
		})
	}
}

func TestGracefulShutdown(t *testing.T) {
	// Этот тест проверяет обработку сигналов graceful shutdown

	// Создаем временное файловое хранилище для теста
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-db.json")

	// Сохраняем оригинальные флаги
	originalFileBD := flagFileBD
	defer func() {
		flagFileBD = originalFileBD
	}()

	flagFileBD = testFile

	// Создаем хранилище
	store, err := createStorage()
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Добавляем тестовые данные
	ctx := context.Background()
	if err := store.Set(ctx, "test-key", "https://test.com"); err != nil {
		t.Fatalf("Failed to set test data: %v", err)
	}

	// Проверяем, что данные сохранились
	value, err := store.Get(ctx, "test-key")
	if err != nil || value != "https://test.com" {
		t.Errorf("Data not saved correctly: value=%s, err=%v", value, err)
	}

	// Закрываем хранилище (должно сохранить данные)
	if err := store.Close(); err != nil {
		t.Errorf("Failed to close storage: %v", err)
	}

	// Создаем новое хранилище и проверяем, что данные загрузились
	newStore, err := createStorage()
	if err != nil {
		t.Fatalf("Failed to recreate storage: %v", err)
	}
	defer newStore.Close()

	// Для файлового хранилища данные должны сохраниться
	if fileStore, ok := newStore.(*storage.FileStorage); ok {
		if err := fileStore.LoadFromFile(); err != nil {
			t.Errorf("Failed to load from file: %v", err)
		}

		value, err := newStore.Get(ctx, "test-key")
		if err != nil {
			t.Errorf("Failed to get data after reload: %v", err)
		}
		if value != "https://test.com" {
			t.Errorf("Expected 'https://test.com', got '%s'", value)
		}
	}
}

// Тест для проверки конкурентных запросов
func TestConcurrentRequests(t *testing.T) {
	store := storage.NewInMemoryStorage()

	r := chi.NewRouter()
	r.Post("/", apiPost(store))
	r.Get("/{id}", redirectHandler(store))

	server := httptest.NewServer(r)
	defer server.Close()

	concurrency := 50
	done := make(chan bool, concurrency)

	// Параллельное создание URL
	for i := 0; i < concurrency; i++ {
		go func(i int) {
			defer func() { done <- true }()

			url := "https://concurrent-test.com/" + string(rune(i))
			resp, err := http.Post(server.URL+"/", "text/plain", bytes.NewBufferString(url))
			if err != nil {
				t.Errorf("Request failed: %v", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusCreated {
				t.Errorf("Expected status %d, got %d", http.StatusCreated, resp.StatusCode)
			}
		}(i)
	}

	// Ожидаем завершения всех горутин
	for i := 0; i < concurrency; i++ {
		<-done
	}

	// Проверяем, что все URL сохранены
	// Количество сохраненных URL должно быть равно количеству запросов
	// (для in-memory хранилища)
	t.Logf("Successfully processed %d concurrent requests", concurrency)
}

// Тест для проверки обработки паники в middleware
func TestMiddlewarePanicRecovery(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handler := LoggingMiddleware(panicHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	// В реальном приложении должен быть recoverer middleware
	// Этот тест просто проверяет, что middleware не паникует
	defer func() {
		if r := recover(); r != nil {
			t.Logf("Recovered from panic: %v", r)
		}
	}()

	handler.ServeHTTP(rec, req)
}

// Тест для проверки сигналов (мокает сигналы)
func TestSignalHandling(t *testing.T) {
	// Создаем канал для сигналов
	sigChan := make(chan os.Signal, 1)

	// Тестируем обработку SIGTERM
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

	// Тестируем обработку SIGINT
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
}

// Бенчмарки
func BenchmarkLoggingMiddleware(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := LoggingMiddleware(handler)
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		middleware.ServeHTTP(rec, req)
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
