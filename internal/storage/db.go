package storage

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

type PostgresStorage struct {
	db *sql.DB
}

func NewPostgresStorage(dsn string) (*PostgresStorage, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &PostgresStorage{db: db}, nil
}

func (s *PostgresStorage) Save(ctx context.Context, shortURL, originalURL string) (string, error) {
	query := `
		INSERT INTO save_url_table (shorturl, url)
		VALUES ($1, $2)
		ON CONFLICT (url) DO NOTHING
		RETURNING shorturl
	`

	var existingShortURL string
	err := s.db.QueryRowContext(ctx, query, shortURL, originalURL).Scan(&existingShortURL)
	if err != nil {
		if err == sql.ErrNoRows {
			// Конфликт - URL уже существует, получаем существующий shorturl
			getQuery := `SELECT shorturl FROM save_url_table WHERE url = $1`
			err = s.db.QueryRowContext(ctx, getQuery, originalURL).Scan(&existingShortURL)
			if err != nil {
				return "", fmt.Errorf("failed to get existing URL: %w", err)
			}
			return existingShortURL, ErrExists
		}
		return "", fmt.Errorf("failed to save URL: %w", err)
	}

	// Успешно создан новый URL
	return existingShortURL, nil
}

func (s *PostgresStorage) Get(ctx context.Context, shortURL string) (string, error) {
	query := `SELECT url FROM save_url_table WHERE shorturl = $1`

	var originalURL string
	err := s.db.QueryRowContext(ctx, query, shortURL).Scan(&originalURL)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("failed to get URL: %w", err)
	}

	return originalURL, nil
}

func (s *PostgresStorage) Set(ctx context.Context, key string, value string) error {
	query := `
		INSERT INTO save_url_table (shorturl, url)
		VALUES ($1, $2)
		
	`

	_, err := s.db.ExecContext(ctx, query, key, value)
	if err != nil {
		return fmt.Errorf("failed to set URL: %w", err)
	}

	return nil
}

func (s *PostgresStorage) Delete(ctx context.Context, key string) error {
	query := `DELETE FROM save_url_table WHERE shorturl = $1`

	result, err := s.db.ExecContext(ctx, query, key)
	if err != nil {
		return fmt.Errorf("failed to delete URL: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *PostgresStorage) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *PostgresStorage) Close() error {
	return s.db.Close()
}

func (s *PostgresStorage) RunMigrations() error {
	// Создание таблицы urls
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS save_url_table (
			id SERIAL PRIMARY KEY,
			shorturl VARCHAR(255) UNIQUE NOT NULL,
			url TEXT NOT NULL,
					);
		
		CREATE INDEX IF NOT EXISTS idx_shorturl ON save_url_table(shorturl);
		
	`

	_, err := s.db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	return nil
}

func (s *PostgresStorage) SaveBatch(ctx context.Context, items []BatchItem) error {
	// Начинаем транзакцию
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO save_url_table (short_url, original_url, correlation_id, created_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (short_url) DO UPDATE SET 
			original_url = EXCLUDED.original_url,
			correlation_id = EXCLUDED.correlation_id,
			updated_at = NOW()
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, item := range items {
		_, err = stmt.ExecContext(ctx, item.ShortURL, item.OriginalURL, item.CorrelationID)
		if err != nil {
			return fmt.Errorf("failed to insert batch item: %w", err)
		}
	}

	return tx.Commit()
}
