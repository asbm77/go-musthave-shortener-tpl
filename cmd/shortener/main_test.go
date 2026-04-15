package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Мок хранилища ---
// Это заглушка, которая позволяет нам симулировать поведение реального хранилища.
type StorageMock struct {
	SetFn    func(key, url string) error
	GetFn    func(key string) (string, error)
	DeleteFn func(key string) error
}

func (m *StorageMock) Set(key, url string) error {
	return m.SetFn(key, url)
}

func (m *StorageMock) Get(key string) (string, error) {
	return m.GetFn(key)
}

func (m *StorageMock) Delete(key string) error {
	// Реализация может быть пустой или возвращать ошибку,
	// если вы хотите проверить сценарии с ошибкой удаления.
	if m.DeleteFn != nil {
		return m.DeleteFn(key)
	}
	return nil
}

// --- Вспомогательная функция для тестов ---
// Упрощает создание и выполнение HTTP-запросов.
func performRequest(handler http.HandlerFunc, method, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "text/plain") // Для apiPost
	if method == http.MethodPost && strings.Contains(path, "shorten") {
		req.Header.Set("Content-Type", "application/json") // Для apiPostShorten
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// TestApiGetPing_Success — тест успешного подключения к БД и ответа 200
func TestApiGetPing_Success(t *testing.T) {
	// Сохраняем оригинал и устанавливаем мок
	original := initDBFunc
	initDBFunc = func() error { return nil }
	defer func() { initDBFunc = original }() // Восстанавливаем после теста

	store := &StorageMock{}
	handler := apiGetPing(store)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	// Проверяем статус-код
	if w.Code != http.StatusOK {
		t.Errorf("Ожидаемый статус %d, получен %d", http.StatusOK, w.Code)
	}

	// Проверяем, что тело ответа пустое
	if w.Body.String() != "" {
		t.Errorf("Ожидалось пустое тело ответа, получено: %q", w.Body.String())
	}
}

// TestApiGetPing_DBError — тест ошибки подключения к БД (500)
func TestApiGetPing_DBError(t *testing.T) {
	// Сохраняем оригинал и устанавливаем мок с ошибкой
	original := initDBFunc
	initDBFunc = func() error { return fmt.Errorf("ошибка подключения к БД") }
	defer func() { initDBFunc = original }()

	store := &StorageMock{}
	handler := apiGetPing(store)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	// Проверяем статус-код
	if w.Code != http.StatusInternalServerError {
		t.Errorf("Ожидаемый статус %d, получен %d", http.StatusInternalServerError, w.Code)
	}

	// Проверяем сообщение об ошибке
	expectedBody := "Internal Server Error\n"
	if w.Body.String() != expectedBody {
		t.Errorf("Ожидаемое тело ответа %q, получено %q", expectedBody, w.Body.String())
	}
}

// --- Тесты для apiPost (Plain Text) ---
func TestApiPost_Success(t *testing.T) {
	mockStore := &StorageMock{
		SetFn: func(key, url string) error {
			assert.Contains(t, url, "example.com")
			assert.Len(t, key, 8)
			return nil
		},
	}
	handler := apiPost(mockStore)

	rr := performRequest(handler, http.MethodPost, "/", []byte("https://example.com"))

	assert.Equal(t, http.StatusCreated, rr.Code)
	assert.NotEmpty(t, rr.Body.String())
	assert.True(t, strings.HasPrefix(rr.Body.String(), flagShortAddr+"/"))
}

func TestApiPost_StorageError(t *testing.T) {
	mockStore := &StorageMock{
		SetFn: func(key, url string) error {
			return errors.New("db connection failed")
		},
	}
	handler := apiPost(mockStore)

	rr := performRequest(handler, http.MethodPost, "/", []byte("https://example.com"))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "Internal Server Error")
}

// --- Тесты для apiPostShorten (JSON API) ---
func TestApiPostShorten_Success(t *testing.T) {
	mockStore := &StorageMock{
		SetFn: func(key, url string) error {
			assert.Equal(t, "https://example.com", url)
			return nil
		},
	}
	handler := apiPostShorten(mockStore)
	body, _ := json.Marshal(ShortenRequest{URL: "https://example.com"})

	rr := performRequest(handler, http.MethodPost, "/api/shorten", body)

	assert.Equal(t, http.StatusCreated, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var resp ShortenResponse
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(resp.Result, flagShortAddr+"/"))
}

func TestApiPostShorten_InvalidJSON(t *testing.T) {
	mockStore := &StorageMock{}
	handler := apiPostShorten(mockStore)
	body := []byte("{invalid json}")

	rr := performRequest(handler, http.MethodPost, "/api/shorten", body)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "Bad Request: Invalid JSON")
}

func TestApiPostShorten_InvalidURLFormat(t *testing.T) {
	mockStore := &StorageMock{}
	handler := apiPostShorten(mockStore)
	body, _ := json.Marshal(ShortenRequest{URL: "invalid_url"})

	rr := performRequest(handler, http.MethodPost, "/api/shorten", body)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "URL must include protocol")
}

// --- Тесты для redirectHandler ---
func TestRedirectHandler_Success(t *testing.T) {
	mockStore := &StorageMock{
		GetFn: func(key string) (string, error) {
			return "https://example.com", nil
		},
	}
	handler := redirectHandler(mockStore)
	req := httptest.NewRequest(http.MethodGet, "/abc123", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusTemporaryRedirect, rr.Code)
	assert.Equal(t, "https://example.com", rr.Header().Get("Location"))
}

func TestRedirectHandler_NotFound(t *testing.T) {
	mockStore := &StorageMock{
		GetFn: func(key string) (string, error) {
			return "", ErrNotFound
		},
	}
	handler := redirectHandler(mockStore)
	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}
