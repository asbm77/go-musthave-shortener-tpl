package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// MockStorage — заглушка для тестирования.
type MockStorage struct {
	SetCalls []SetCall
	GetCalls []GetCall

	SetErrorToReturn error
	GetValueToReturn string
	GetErrorToReturn error
}

type SetCall struct {
	Key   string
	Value string
}
type GetCall struct {
	Key string
}

func (m *MockStorage) Set(key, value string) error {
	m.SetCalls = append(m.SetCalls, SetCall{Key: key, Value: value})
	return m.SetErrorToReturn
}
func (m *MockStorage) Get(key string) (string, error) {
	m.GetCalls = append(m.GetCalls, GetCall{Key: key})
	return m.GetValueToReturn, m.GetErrorToReturn
}
func (m *MockStorage) Delete(key string) error {
	return nil
}

//const flagShortAddr = "http://short.url"

// --- ТЕСТЫ ДЛЯ apiGet ---
func Test_apiGet(t *testing.T) {
	t.Run("Успешное перенаправление: URL найден", func(t *testing.T) {
		mockStore := &MockStorage{
			GetValueToReturn: "https://example.com",
			GetErrorToReturn: nil, // Ошибки нет
		}
		handler := apiGet(mockStore)

		req := httptest.NewRequest(http.MethodGet, "/abc123", nil)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		if res.Code != http.StatusTemporaryRedirect {
			t.Errorf("Ожидался статус %d, получен %d", http.StatusTemporaryRedirect, res.Code)
		}
	})

	t.Run("Ошибка 404: URL не найден", func(t *testing.T) {
		// ИСПРАВЛЕНИЕ ЗДЕСЬ:
		// Мы симулируем ситуацию, когда ключ не найден.
		// Для этого нужно вернуть специальную ошибку ErrNotFound.
		mockStore := &MockStorage{
			// GetValueToReturn можно оставить пустым или указать что угодно,
			// так как при ошибке значение обычно не проверяется.
			GetValueToReturn: "",
			GetErrorToReturn: ErrNotFound, // <-- Возвращаем ошибку "не найдено"
		}
		handler := apiGet(mockStore)

		req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		if res.Code != http.StatusNotFound {
			t.Errorf("Ожидался статус %d, получен %d", http.StatusNotFound, res.Code)
		}
	})
}

// --- ТЕСТЫ ДЛЯ apiPost ---
func Test_apiPost(t *testing.T) {
	t.Run("Успешное создание короткой ссылки", func(t *testing.T) {
		mockStore := &MockStorage{} // Ошибок нет по умолчанию

		handler := apiPost(mockStore)

		reqBody := bytes.NewBufferString("https://yandex.ru")
		req := httptest.NewRequest(http.MethodPost, "/api/shorten", reqBody)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		if res.Code != http.StatusCreated {
			t.Errorf("Ожидался статус %d, получен %d", http.StatusCreated, res.Code)
		}

		body := res.Body.String()
		if !bytes.Contains(res.Body.Bytes(), []byte(flagShortAddr+"/")) {
			t.Errorf("Ответ не содержит короткий URL. Тело: %s", body)
		}

		if len(mockStore.SetCalls) != 1 {
			t.Fatalf("Ожидался 1 вызов store.Set, было %d вызовов", len(mockStore.SetCalls))
		}

		call := mockStore.SetCalls[0]

		if call.Value != "https://yandex.ru" {
			t.Errorf("Ожидалось сохранение URL 'https://yandex.ru', сохранено '%s'", call.Value)
		}

		if call.Key == "" || len(call.Key) < 2 { // Проверяем, что ключ не пустой и имеет префикс "/"
			t.Errorf("Ключ для сохранения сгенерирован некорректно: %s", call.Key)
		}

	})

	t.Run("Некорректный метод (GET)", func(t *testing.T) {
		mockStore := &MockStorage{}

		handler := apiPost(mockStore)

		req := httptest.NewRequest(http.MethodGet, "/api/shorten", nil)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		if res.Code != http.StatusBadRequest {
			t.Errorf("Ожидался статус %d, получен %d", http.StatusBadRequest, res.Code)
		}

		if mockStore.SetCalls != nil && len(mockStore.SetCalls) > 0 {
			t.Error("Метод store.Set не должен был вызываться")
		}

	})

	t.Run("Ошибка хранилища возвращает 500", func(t *testing.T) {
		mockStore := &MockStorage{
			SetErrorToReturn: errors.New("база данных недоступна"),
		}

		handler := apiPost(mockStore)

		reqBody := bytes.NewBufferString("https://google.com")
		req := httptest.NewRequest(http.MethodPost, "/api/shorten", reqBody)
		res := httptest.NewRecorder()

		handler.ServeHTTP(res, req)

		if res.Code != http.StatusInternalServerError {
			t.Errorf("Ожидался статус %d, получен %d", http.StatusInternalServerError, res.Code)
		}

		if res.Body.String() != "Internal Server Error\n" {
			t.Errorf("Неожиданное тело ответа: %s", res.Body.String())
		}

		if len(mockStore.SetCalls) != 1 { // Хендлер попытался вызвать Set, но получил ошибку
			t.Error("Метод store.Set должен был быть вызван")
		}

	})
}
