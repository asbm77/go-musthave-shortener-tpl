package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Observer - интерфейс наблюдателя для аудита
type Observer interface {
	Notify(event *Event) error
}

// ============ Файловый наблюдатель ============

// FileObserver - наблюдатель для записи аудита в файл
type FileObserver struct {
	filePath string
	file     *os.File
}

// NewFileObserver создает новый файловый наблюдатель
func NewFileObserver(filePath string) (*FileObserver, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path is empty")
	}

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit file: %w", err)
	}

	return &FileObserver{
		filePath: filePath,
		file:     file,
	}, nil
}

// Notify записывает событие аудита в файл
func (f *FileObserver) Notify(event *Event) error {
	if f.file == nil {
		return fmt.Errorf("audit file is not open")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Добавляем новую строку после каждого события
	if _, err := f.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write to audit file: %w", err)
	}

	return nil
}

// Close закрывает файл
func (f *FileObserver) Close() error {
	if f.file != nil {
		return f.file.Close()
	}
	return nil
}

// ============ HTTP наблюдатель ============

// HTTPObserver - наблюдатель для отправки аудита на удаленный сервер
type HTTPObserver struct {
	url        string
	httpClient *http.Client
}

// NewHTTPObserver создает новый HTTP наблюдатель
func NewHTTPObserver(url string) *HTTPObserver {
	return &HTTPObserver{
		url: url,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Notify отправляет событие аудита на удаленный сервер
func (h *HTTPObserver) Notify(event *Event) error {
	if h.url == "" {
		return fmt.Errorf("audit URL is empty")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send audit event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("audit server returned error status: %d", resp.StatusCode)
	}

	return nil
}

// Close закрывает HTTP клиент (ничего не делает, но реализует интерфейс для единообразия)
func (h *HTTPObserver) Close() error {
	// HTTP клиент не требует явного закрытия
	return nil
}

// ============ Мульти-наблюдатель (опционально) ============

// MultiObserver - наблюдатель, который отправляет события нескольким наблюдателям
type MultiObserver struct {
	observers []Observer
}

// NewMultiObserver создает новый мульти-наблюдатель
func NewMultiObserver(observers ...Observer) *MultiObserver {
	return &MultiObserver{
		observers: observers,
	}
}

// Notify отправляет событие всем наблюдателям
func (m *MultiObserver) Notify(event *Event) error {
	var lastErr error
	for _, observer := range m.observers {
		if err := observer.Notify(event); err != nil {
			lastErr = err
			// Продолжаем отправлять другим наблюдателям даже при ошибке
		}
	}
	return lastErr
}

// Close закрывает всех наблюдателей
func (m *MultiObserver) Close() error {
	var lastErr error
	for _, observer := range m.observers {
		if closer, ok := observer.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}
