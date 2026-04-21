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
	Save(ctx context.Context, shortURL, originalURL string) error
	Get(ctx context.Context, shortURL string) (string, error)
	Set(ctx context.Context, key string, value string) error // Добавлен
	Delete(ctx context.Context, key string) error            // Добавлен
	Ping(ctx context.Context) error
	Close() error
}
