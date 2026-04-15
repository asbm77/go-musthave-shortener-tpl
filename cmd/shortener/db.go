package main

import (
	"database/sql"
	_ "github.com/lib/pq" // Импорт драйвера PostgreSQL
	"log"
)

var DB *sql.DB

func InitDB() error {
	connStr := flagConnDB

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return err
	}

	// Проверяем подключение
	err = db.Ping()
	if err != nil {
		return err
	}

	DB = db
	log.Println("Успешное подключение к PostgreSQL")
	return nil
}

// CloseDB закрывает соединение с БД
func CloseDB() {
	if DB != nil {
		DB.Close()
	}
}
