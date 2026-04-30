package storage

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync"
)

type FileStorage struct {
	mu           sync.RWMutex
	urls         map[string]string    // shortURL -> originalURL
	short        map[string]string    // originalURL -> shortURL
	deleted      map[string]bool      // shortURL -> isDeleted
	correlations map[string][]string  // correlationID -> []shortURL
	userURLs     map[string][]UserURL // userID -> []UserURL
	filePath     string
}

func NewFileStorage(filePath string) *FileStorage {
	return &FileStorage{
		urls:         make(map[string]string),
		short:        make(map[string]string),
		deleted:      make(map[string]bool),
		correlations: make(map[string][]string),
		userURLs:     make(map[string][]UserURL),
		filePath:     filePath,
	}
}

type fileData struct {
	URLs         map[string]string    `json:"urls"`
	Short        map[string]string    `json:"short"`
	Deleted      map[string]bool      `json:"deleted"`
	Correlations map[string][]string  `json:"correlations"`
	UserURLs     map[string][]UserURL `json:"user_urls"`
}

func (s *FileStorage) saveToFile() error {
	data := fileData{
		URLs:         s.urls,
		Short:        s.short,
		Deleted:      s.deleted,
		Correlations: s.correlations,
		UserURLs:     s.userURLs,
	}

	file, err := os.Create(s.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func (s *FileStorage) LoadFromFile() error {
	file, err := os.Open(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	var data fileData
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&data); err != nil {
		return err
	}

	// Восстанавливаем данные
	if data.URLs != nil {
		s.urls = data.URLs
	}
	if data.Short != nil {
		s.short = data.Short
	} else {
		// Обратная совместимость
		s.short = make(map[string]string)
		for shortURL, originalURL := range s.urls {
			s.short[originalURL] = shortURL
		}
	}
	if data.Deleted != nil {
		s.deleted = data.Deleted
	} else {
		s.deleted = make(map[string]bool)
	}
	if data.Correlations != nil {
		s.correlations = data.Correlations
	}
	if data.UserURLs != nil {
		s.userURLs = data.UserURLs
	}

	return nil
}

// Save сохраняет короткий URL и оригинальный URL
func (s *FileStorage) Save(ctx context.Context, shortURL, originalURL string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existingShort, exists := s.short[originalURL]; exists {
		return existingShort, ErrExists
	}

	if _, exists := s.urls[shortURL]; exists {
		return "", ErrExists
	}

	s.urls[shortURL] = originalURL
	s.short[originalURL] = shortURL

	if err := s.saveToFile(); err != nil {
		// Откат изменений при ошибке сохранения
		delete(s.urls, shortURL)
		delete(s.short, originalURL)
		return "", err
	}

	return shortURL, nil
}

func (s *FileStorage) Get(ctx context.Context, shortURL string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Проверяем, не удален ли URL
	if s.deleted[shortURL] {
		return "", ErrGone
	}

	originalURL, exists := s.urls[shortURL]
	if !exists {
		return "", ErrNotFound
	}
	return originalURL, nil
}

// Set устанавливает или обновляет пару ключ-значение
func (s *FileStorage) Set(ctx context.Context, key string, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Удаляем старую обратную ссылку
	if oldValue, exists := s.urls[key]; exists {
		delete(s.short, oldValue)
	}

	s.urls[key] = value
	s.short[value] = key

	return s.saveToFile()
}

// Delete удаляет URL по ключу
func (s *FileStorage) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	originalURL, exists := s.urls[key]
	if !exists {
		return ErrNotFound
	}

	delete(s.urls, key)
	delete(s.short, originalURL)

	// Удаляем из userURLs (нужно найти пользователя)
	for userID, urls := range s.userURLs {
		for i, userURL := range urls {
			if userURL.ShortURL == key {
				s.userURLs[userID] = append(urls[:i], urls[i+1:]...)
				break
			}
		}
	}

	return s.saveToFile()
}

// Ping проверяет доступность хранилища
func (s *FileStorage) Ping(ctx context.Context) error {
	// Для файлового хранилища проверяем, можем ли мы создать файл
	file, err := os.Create(s.filePath + ".tmp")
	if err != nil {
		return err
	}
	file.Close()
	os.Remove(s.filePath + ".tmp")
	return nil
}

// Close закрывает хранилище и сохраняет данные
func (s *FileStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveToFile()
}

// SaveBatch сохраняет пакет URL-ов
func (s *FileStorage) SaveBatch(ctx context.Context, items []BatchItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, item := range items {
		// Проверяем, не существует ли уже такой оригинальный URL
		if existingShort, exists := s.short[item.OriginalURL]; exists {
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

	return s.saveToFile()
}

// SaveUserURL сохраняет URL для конкретного пользователя
func (s *FileStorage) SaveUserURL(ctx context.Context, userID, shortURL, originalURL string) (string, error) {
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

	if err := s.saveToFile(); err != nil {
		// Откат изменений
		delete(s.urls, shortURL)
		delete(s.short, originalURL)
		// Удаляем из userURLs
		if urls, exists := s.userURLs[userID]; exists {
			for i, u := range urls {
				if u.ShortURL == shortURL {
					s.userURLs[userID] = append(urls[:i], urls[i+1:]...)
					break
				}
			}
		}
		return "", err
	}

	return shortURL, nil
}

// GetUserURLs возвращает все URL пользователя
func (s *FileStorage) GetUserURLs(ctx context.Context, userID string) ([]UserURL, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	urls, exists := s.userURLs[userID]
	if !exists || len(urls) == 0 {
		return []UserURL{}, nil
	}

	// Фильтруем удаленные URL
	var result []UserURL
	for _, url := range urls {
		if !s.deleted[url.ShortURL] {
			result = append(result, url)
		}
	}

	return result, nil
}

// Дополнительные методы:

// GetStats возвращает статистику хранилища
func (s *FileStorage) GetStats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]int{
		"total_urls":  len(s.urls),
		"total_short": len(s.short),
		"total_users": len(s.userURLs),
		"total_corr":  len(s.correlations),
	}
}

// Clear очищает все данные (полезно для тестов)
func (s *FileStorage) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.urls = make(map[string]string)
	s.short = make(map[string]string)
	s.correlations = make(map[string][]string)
	s.userURLs = make(map[string][]UserURL)

	return s.saveToFile()
}

func (s *FileStorage) DeleteUserURLs(ctx context.Context, userID string, shortURLs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(shortURLs) == 0 {
		return nil
	}

	// Проверяем, что URL принадлежат пользователю и помечаем как удаленные
	for _, shortURL := range shortURLs {
		// Ищем URL в urls мапе
		if _, exists := s.urls[shortURL]; exists {
			// Проверяем, принадлежит ли URL этому пользователю
			userURLs, exists := s.userURLs[userID]
			if !exists {
				continue
			}

			found := false
			for _, userURL := range userURLs {
				if userURL.ShortURL == shortURL {
					found = true
					break
				}
			}

			if found {
				// Помечаем как удаленный
				s.deleted[shortURL] = true
				log.Printf("[DEBUG] Marked URL as deleted: %s for user %s", shortURL, userID)
			}
		}
	}

	// Сохраняем изменения в файл
	return s.saveToFile()
}
