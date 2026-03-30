package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_apiGet1(t *testing.T) {
	urlMap = make(map[string]string)

	testID := "/abc123"
	testURL := "https://example.com"
	urlMap[testID] = testURL

	type args struct {
		res http.ResponseWriter
		req *http.Request
	}
	tests := []struct {
		name           string
		args           args
		expectedStatus int
		expectedHeader string
	}{
		{
			name: "Успешное перенаправление: URL найден",
			args: args{
				res: httptest.NewRecorder(),
				req: httptest.NewRequest(http.MethodGet, testID, nil),
			},
			expectedStatus: http.StatusTemporaryRedirect,
			expectedHeader: testURL,
		},
		{
			name: "Ошибка 404: URL не найден",
			args: args{
				res: httptest.NewRecorder(),
				req: httptest.NewRequest(http.MethodGet, "/unknown", nil),
			},
			expectedStatus: http.StatusNotFound,
			expectedHeader: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiGet(tt.args.res, tt.args.req)
		})
	}
}

func Test_apiPost1(t *testing.T) {
	urlMap = make(map[string]string)

	type args struct {
		res http.ResponseWriter
		req *http.Request
	}
	tests := []struct {
		name           string
		args           args
		expectedStatus int
		expectedBody   string
		expectInMap    bool
	}{
		{
			name: "Успешное создание короткой ссылки",
			args: args{
				res: httptest.NewRecorder(),
				req: httptest.NewRequest(http.MethodPost, "/api/shorten", bytes.NewBufferString("https://example.com")),
			},
			expectedStatus: http.StatusCreated,
			expectedBody:   flagShortAddr + "/",
			expectInMap:    true,
		},
		{
			name: "Некорректный метод (GET)",
			args: args{
				res: httptest.NewRecorder(),
				req: httptest.NewRequest(http.MethodGet, "/api/shorten", nil),
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "400 Bad Request\n",
			expectInMap:    false,
		},
		{
			name: "Пустой body запроса",
			args: args{
				res: httptest.NewRecorder(),
				req: httptest.NewRequest(http.MethodPost, "/api/shorten", nil),
			},
			expectedStatus: http.StatusCreated,
			expectedBody:   flagShortAddr + "/",
			expectInMap:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiPost(tt.args.res, tt.args.req)

			recorder := tt.args.res.(*httptest.ResponseRecorder)

			// Проверяем статус ответа
			if recorder.Code != tt.expectedStatus {
				t.Errorf("apiPost() статус = %v, хотим %v", recorder.Code, tt.expectedStatus)
			}

			// Проверяем тело ответа (ищем подстроку, так как UUID случаен)
			if !bytes.Contains(recorder.Body.Bytes(), []byte(tt.expectedBody)) {
				t.Errorf("apiPost() тело = %v, ожидаем подстроку %v", recorder.Body.String(), tt.expectedBody)
			}

			// Проверяем, добавлен ли URL в карту (если ожидалось)
			if tt.expectInMap && len(urlMap) == 0 {
				t.Error("Ожидалось, что URL будет добавлен в urlMap, но карта пуста")
			}
		})
	}
}
