package storage

import (
	"context"
	"errors"
)

var (
	ErrNotFound = errors.New("URL not found")
	ErrExists   = errors.New("URL already exists")
)

type Storage interface {
	Save(ctx context.Context, shortURL, originalURL string) (string, error)
	Get(ctx context.Context, shortURL string) (string, error)
	Set(ctx context.Context, key string, value string) error
	Delete(ctx context.Context, key string) error
	Ping(ctx context.Context) error
	Close() error

	SaveBatch(ctx context.Context, items []BatchItem) error

	SaveUserURL(ctx context.Context, userID, shortURL, originalURL string) (string, error)
	GetUserURLs(ctx context.Context, userID string) ([]UserURL, error)

	DeleteUserURLs(ctx context.Context, userID string, shortURLs []string) error
}

type BatchItem struct {
	CorrelationID string
	ShortURL      string
	OriginalURL   string
	UserID        string `json:"user_id"`
}

type URLRecord struct {
	ID            int64
	ShortURL      string
	OriginalURL   string
	UserID        string
	CorrelationID string
	CreatedAt     string
}

type UserURL struct {
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
}
