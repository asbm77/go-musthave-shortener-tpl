package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"

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

	createTableSQL := `
		CREATE TABLE IF NOT EXISTS save_url_table (
			id SERIAL PRIMARY KEY,
			shorturl VARCHAR(255) UNIQUE NOT NULL,
			url TEXT NOT NULL UNIQUE,
		    correlation_id VARCHAR(255),
		    user_id VARCHAR(255)
					);
		
		CREATE INDEX IF NOT EXISTS idx_shorturl ON save_url_table(shorturl);
		CREATE INDEX IF NOT EXISTS idx_user_id ON save_url_table(user_id);
		
	`

	_, err := s.db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	return nil
}

func (s *PostgresStorage) SaveBatch(ctx context.Context, items []BatchItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO save_url_table (shorturl, url, correlation_id, user_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (url) DO UPDATE 
		SET correlation_id = EXCLUDED.correlation_id,
		    user_id = EXCLUDED.user_id
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, item := range items {
		_, err := stmt.ExecContext(ctx, item.ShortURL, item.OriginalURL, item.CorrelationID, item.UserID)
		if err != nil {
			return fmt.Errorf("failed to insert batch item: %w", err)
		}
	}

	return tx.Commit()
}

func (s *PostgresStorage) SaveUserURL(ctx context.Context, userID, shortURL, originalURL string) (string, error) {
	// Если userID пустой, используем значение по умолчанию
	if userID == "" {
		userID = "anonymous"
	}

	// Сначала проверяем, существует ли уже такой URL
	var existingShortURL string
	var existingUserID sql.NullString
	checkQuery := `SELECT shorturl, user_id FROM save_url_table WHERE url = $1`
	err := s.db.QueryRowContext(ctx, checkQuery, originalURL).Scan(&existingShortURL, &existingUserID)
	if err == nil {
		// Если URL существует и user_id не установлен, обновляем его
		if !existingUserID.Valid || existingUserID.String == "" {
			updateQuery := `UPDATE save_url_table SET user_id = $1 WHERE url = $2`
			_, updateErr := s.db.ExecContext(ctx, updateQuery, userID, originalURL)
			if updateErr != nil {
				log.Printf("Failed to update user_id: %v", updateErr)
			}
		}
		return existingShortURL, ErrExists
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("failed to check existing URL: %w", err)
	}

	// URL не существует, вставляем новый
	query := `
		INSERT INTO save_url_table (shorturl, url, user_id)
		VALUES ($1, $2, $3)
		RETURNING shorturl
	`

	var resultShortURL string
	err = s.db.QueryRowContext(ctx, query, shortURL, originalURL, userID).Scan(&resultShortURL)
	if err != nil {
		return "", fmt.Errorf("failed to save user URL: %w", err)
	}

	return resultShortURL, nil
}

func (s *PostgresStorage) GetUserURLs(ctx context.Context, userID string) ([]UserURL, error) {
	log.Printf("GetUserURLs called for userID: %s", userID)

	query := `
		SELECT shorturl, url 
		FROM save_url_table 
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user URLs: %w", err)
	}
	defer rows.Close()

	var urls []UserURL
	for rows.Next() {
		var url UserURL
		if err := rows.Scan(&url.ShortURL, &url.OriginalURL); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		urls = append(urls, url)
		log.Printf("Found URL for user: short=%s, original=%s", url.ShortURL, url.OriginalURL)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	log.Printf("Returning %d URLs for user %s", len(urls), userID)
	return urls, nil
}
