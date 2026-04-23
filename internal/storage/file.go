package storage

import (
	"context"
	"encoding/json"
	"os"
	"sync"
)

type FileStorage struct {
	mu           sync.RWMutex
	urls         map[string]string
	short        map[string]string
	correlations map[string][]string
	filePath     string
}

type fileData struct {
	URLs         map[string]string   `json:"urls"`
	Correlations map[string][]string `json:"correlations"`
}

func NewFileStorage(filePath string) *FileStorage {
	return &FileStorage{
		urls:         make(map[string]string),
		short:        make(map[string]string),
		correlations: make(map[string][]string),
		filePath:     filePath,
	}
}

func (s *FileStorage) Save(ctx context.Context, shortURL, originalURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.urls[shortURL]; exists {
		return ErrExists
	}

	s.urls[shortURL] = originalURL
	s.short[originalURL] = shortURL

	return s.saveToFile()
}

func (s *FileStorage) Get(ctx context.Context, shortURL string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	originalURL, exists := s.urls[shortURL]
	if !exists {
		return "", ErrNotFound
	}
	return originalURL, nil
}

func (s *FileStorage) Set(ctx context.Context, key string, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.urls[key] = value
	s.short[value] = key

	return s.saveToFile()
}

func (s *FileStorage) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	originalURL, exists := s.urls[key]
	if !exists {
		return ErrNotFound
	}

	delete(s.urls, key)
	delete(s.short, originalURL)

	return s.saveToFile()
}

func (s *FileStorage) Ping(ctx context.Context) error {
	return nil
}

func (s *FileStorage) Close() error {
	return s.saveToFile()
}

func (s *FileStorage) saveToFile() error {
	file, err := os.Create(s.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(s.urls)
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

	decoder := json.NewDecoder(file)
	return decoder.Decode(&s.urls)
}

func (s *FileStorage) SaveBatch(ctx context.Context, items []BatchItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, item := range items {
		s.urls[item.ShortURL] = item.OriginalURL
		s.short[item.OriginalURL] = item.ShortURL

		if item.CorrelationID != "" {
			s.correlations[item.CorrelationID] = append(s.correlations[item.CorrelationID], item.ShortURL)
		}
	}

	return s.saveToFile()
}
