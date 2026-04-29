package storage

import (
	"context"
	"sync"
)

type MemoryStorage struct {
	mu           sync.RWMutex
	urls         map[string]string    // shortURL -> originalURL
	short        map[string]string    // originalURL -> shortURL
	correlations map[string][]string  // correlationID -> []shortURL
	userURLs     map[string][]UserURL // userID -> []UserURL
}

func NewInMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		urls:         make(map[string]string),
		short:        make(map[string]string),
		correlations: make(map[string][]string),
		userURLs:     make(map[string][]UserURL),
	}
}

// Save сохраняет короткий URL и оригинальный URL
func (s *MemoryStorage) Save(ctx context.Context, shortURL, originalURL string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Проверяем, существует ли уже такой оригинальный URL
	if existingShort, exists := s.short[originalURL]; exists {
		return existingShort, ErrExists
	}

	// Проверяем, не занят ли короткий URL
	if _, exists := s.urls[shortURL]; exists {
		return "", ErrExists
	}

	s.urls[shortURL] = originalURL
	s.short[originalURL] = shortURL
	return shortURL, nil
}

// Get возвращает оригинальный URL по короткому URL
func (s *MemoryStorage) Get(ctx context.Context, shortURL string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	originalURL, exists := s.urls[shortURL]
	if !exists {
		return "", ErrNotFound
	}
	return originalURL, nil
}

// Set устанавливает или обновляет пару ключ-значение
func (s *MemoryStorage) Set(ctx context.Context, key string, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Удаляем старую обратную ссылку, если она существует
	if oldValue, exists := s.urls[key]; exists {
		delete(s.short, oldValue)
	}

	s.urls[key] = value
	s.short[value] = key
	return nil
}

// Delete удаляет URL по ключу
func (s *MemoryStorage) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	originalURL, exists := s.urls[key]
	if !exists {
		return ErrNotFound
	}

	delete(s.urls, key)
	delete(s.short, originalURL)

	// Примечание: удаление из userURLs требует дополнительной логики,
	// так как нужно найти пользователя, которому принадлежит этот URL
	// Для простоты оставляем как есть, но в реальном приложении
	// стоит добавить обратный индекс

	return nil
}

// Ping проверяет доступность хранилища (для in-memory всегда успешно)
func (s *MemoryStorage) Ping(ctx context.Context) error {
	return nil
}

// Close закрывает хранилище (для in-memory ничего не делает)
func (s *MemoryStorage) Close() error {
	return nil
}

// SaveBatch сохраняет пакет URL-ов
func (s *MemoryStorage) SaveBatch(ctx context.Context, items []BatchItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, item := range items {
		// Проверяем, не существует ли уже такой оригинальный URL
		if existingShort, exists := s.short[item.OriginalURL]; exists {
			// URL уже существует, используем существующий короткий URL
			item.ShortURL = existingShort
		}

		// Сохраняем URL
		s.urls[item.ShortURL] = item.OriginalURL
		s.short[item.OriginalURL] = item.ShortURL

		// Сохраняем correlation_id
		if item.CorrelationID != "" {
			s.correlations[item.CorrelationID] = append(s.correlations[item.CorrelationID], item.ShortURL)
		}

		// Сохраняем связь с пользователем
		userURL := UserURL{
			ShortURL:    item.ShortURL,
			OriginalURL: item.OriginalURL,
		}
		s.userURLs[item.UserID] = append(s.userURLs[item.UserID], userURL)
	}

	return nil
}

// SaveUserURL сохраняет URL для конкретного пользователя
func (s *MemoryStorage) SaveUserURL(ctx context.Context, userID, shortURL, originalURL string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Проверяем, существует ли уже такой оригинальный URL
	if existingShort, exists := s.short[originalURL]; exists {
		return existingShort, ErrExists
	}

	// Проверяем, не занят ли короткий URL
	if _, exists := s.urls[shortURL]; exists {
		return "", ErrExists
	}

	// Сохраняем URL
	s.urls[shortURL] = originalURL
	s.short[originalURL] = shortURL

	// Сохраняем связь с пользователем
	userURL := UserURL{
		ShortURL:    shortURL,
		OriginalURL: originalURL,
	}
	s.userURLs[userID] = append(s.userURLs[userID], userURL)

	return shortURL, nil
}

// GetUserURLs возвращает все URL пользователя
func (s *MemoryStorage) GetUserURLs(ctx context.Context, userID string) ([]UserURL, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	urls, exists := s.userURLs[userID]
	if !exists || len(urls) == 0 {
		return []UserURL{}, nil
	}

	// Возвращаем копию, чтобы избежать изменений извне
	result := make([]UserURL, len(urls))
	copy(result, urls)
	return result, nil
}

// Дополнительные методы для отладки и мониторинга (не входят в интерфейс):

// GetStats возвращает статистику хранилища
func (s *MemoryStorage) GetStats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]int{
		"total_urls":  len(s.urls),
		"total_short": len(s.short),
		"total_users": len(s.userURLs),
		"total_corr":  len(s.correlations),
	}
}

// GetUserShortURLs возвращает короткие URL пользователя (без дубликатов)
func (s *MemoryStorage) GetUserShortURLs(ctx context.Context, userID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	urls, exists := s.userURLs[userID]
	if !exists {
		return []string{}, nil
	}

	result := make([]string, 0, len(urls))
	seen := make(map[string]bool)

	for _, url := range urls {
		if !seen[url.ShortURL] {
			seen[url.ShortURL] = true
			result = append(result, url.ShortURL)
		}
	}

	return result, nil
}

// GetUserCount возвращает количество пользователей
func (s *MemoryStorage) GetUserCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.userURLs)
}

// Clear очищает все данные (полезно для тестов)
func (s *MemoryStorage) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.urls = make(map[string]string)
	s.short = make(map[string]string)
	s.correlations = make(map[string][]string)
	s.userURLs = make(map[string][]UserURL)
}
