package storage

import (
	"context"
	"sync"
)

type MemoryStorage struct {
	mu           sync.RWMutex
	urls         map[string]string
	short        map[string]string
	correlations map[string][]string
}

func NewInMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		urls:         make(map[string]string),
		short:        make(map[string]string),
		correlations: make(map[string][]string),
	}
}

func (s *MemoryStorage) Save(ctx context.Context, shortURL, originalURL string) (string, error) {
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
	return shortURL, nil
}

func (s *MemoryStorage) Get(ctx context.Context, shortURL string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	originalURL, exists := s.urls[shortURL]
	if !exists {
		return "", ErrNotFound
	}
	return originalURL, nil
}

func (s *MemoryStorage) Set(ctx context.Context, key string, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.urls[key] = value
	s.short[value] = key
	return nil
}

func (s *MemoryStorage) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	originalURL, exists := s.urls[key]
	if !exists {
		return ErrNotFound
	}

	delete(s.urls, key)
	delete(s.short, originalURL)
	return nil
}

func (s *MemoryStorage) Ping(ctx context.Context) error {
	return nil
}

func (s *MemoryStorage) Close() error {
	return nil
}

func (s *MemoryStorage) SaveBatch(ctx context.Context, items []BatchItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, item := range items {
		// Сохраняем URL
		s.urls[item.ShortURL] = item.OriginalURL
		s.short[item.OriginalURL] = item.ShortURL

		// Сохраняем correlation_id
		if item.CorrelationID != "" {
			s.correlations[item.CorrelationID] = append(s.correlations[item.CorrelationID], item.ShortURL)
		}
	}

	return nil
}
