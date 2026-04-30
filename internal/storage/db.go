package storage

import (
	"context"
	"database/sql"
	"errors"
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

	return existingShortURL, nil
}

func (s *PostgresStorage) Get(ctx context.Context, shortURL string) (string, error) {
	query := `SELECT url, is_deleted FROM save_url_table WHERE shorturl = $1`

	var originalURL string
	var isDeleted bool
	err := s.db.QueryRowContext(ctx, query, shortURL).Scan(&originalURL, &isDeleted)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("failed to get URL: %w", err)
	}

	// Если URL помечен как удаленный, возвращаем специальную ошибку
	if isDeleted {
		return "", ErrGone
	}

	return originalURL, nil
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
			user_id VARCHAR(255),
			is_deleted BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		
		CREATE INDEX IF NOT EXISTS idx_shorturl ON save_url_table(shorturl);
		CREATE INDEX IF NOT EXISTS idx_url ON save_url_table(url);
		CREATE INDEX IF NOT EXISTS idx_user_id ON save_url_table(user_id);
		CREATE INDEX IF NOT EXISTS idx_is_deleted ON save_url_table(is_deleted);
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
		INSERT INTO save_url_table (shorturl, url, correlation_id, user_id, created_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
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
	log.Printf("SaveUserURL called: userID=%s, shortURL=%s, originalURL=%s", userID, shortURL, originalURL)

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
		log.Printf("URL already exists: shorturl=%s, existing userID=%v", existingShortURL, existingUserID)

		// Если URL существует, но user_id не установлен, обновляем его
		if !existingUserID.Valid || existingUserID.String == "" {
			updateQuery := `UPDATE save_url_table SET user_id = $1 WHERE url = $2`
			_, updateErr := s.db.ExecContext(ctx, updateQuery, userID, originalURL)
			if updateErr != nil {
				log.Printf("Failed to update user_id: %v", updateErr)
			} else {
				log.Printf("Updated user_id to %s for existing URL", userID)
			}
		}
		return existingShortURL, ErrExists
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("failed to check existing URL: %w", err)
	}

	// URL не существует, вставляем новый
	query := `
		INSERT INTO save_url_table (shorturl, url, user_id, created_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		RETURNING shorturl
	`

	var resultShortURL string
	err = s.db.QueryRowContext(ctx, query, shortURL, originalURL, userID).Scan(&resultShortURL)
	if err != nil {
		log.Printf("Failed to insert new URL: %v", err)
		return "", fmt.Errorf("failed to save user URL: %w", err)
	}

	log.Printf("Successfully saved new URL: %s -> %s for user %s", resultShortURL, originalURL, userID)
	return resultShortURL, nil
}

func (s *PostgresStorage) GetUserURLs(ctx context.Context, userID string) ([]UserURL, error) {
	log.Printf("GetUserURLs called for userID: %s", userID)

	query := `
		SELECT shorturl, url 
		FROM save_url_table 
		WHERE user_id = $1 AND is_deleted = FALSE
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
	}

	return urls, nil
}

func (s *PostgresStorage) DeleteUserURLs(ctx context.Context, userID string, shortURLs []string) error {
	if len(shortURLs) == 0 {
		return nil
	}

	// Используем множественное обновление (batch update)
	query := `
		UPDATE save_url_table 
		SET is_deleted = TRUE 
		WHERE shorturl = ANY($1) AND user_id = $2
	`

	result, err := s.db.ExecContext(ctx, query, shortURLs, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user URLs: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	log.Printf("[DEBUG] Deleted %d URLs for user %s", rowsAffected, userID)
	return nil
}
