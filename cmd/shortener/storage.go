package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
)

var ErrNotFound = errors.New("storage: key not found")

type Storage interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

type InMemoryStorage struct {
	data map[string]string
}

func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		data: make(map[string]string),
	}
}

func (s *InMemoryStorage) Get(key string) (string, error) {
	value, ok := s.data[key]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}

func (s *InMemoryStorage) Set(key, value string) error {
	s.data[key] = value
	return nil
}

func (s *InMemoryStorage) Delete(key string) error {
	delete(s.data, key)
	return nil
}

func (s *InMemoryStorage) LoadFromFile(flagFileBD string) error {
	// Проверяем существование файла
	_, err := os.Stat(flagFileBD)
	if os.IsNotExist(err) {
		log.Printf("Файл %s не найден, создаётся новое хранилище", flagFileBD)
		return nil
	} else if err != nil {
		return err
	}

	// Читаем файл
	data, err := os.ReadFile(flagFileBD)
	if err != nil {
		return fmt.Errorf("ошибка чтения файла %s: %w", flagFileBD, err)
	}

	// Декодируем JSON в map
	err = json.Unmarshal(data, &s.data)
	if err != nil {
		return fmt.Errorf("ошибка декодирования JSON из %s: %w", flagFileBD, err)
	}
	log.Printf("Загружено %d записей из %s", len(s.data), flagFileBD)
	return nil
}

func (s *InMemoryStorage) SaveToFile(flagFileBD string) error {
	// Кодируем map в JSON
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка кодирования JSON: %w", err)
	}

	// Записываем в файл
	err = os.WriteFile(flagFileBD, data, 0644)
	if err != nil {
		return fmt.Errorf("ошибка записи в файл %s: %w", flagFileBD, err)
	}
	log.Printf("Сохранено %d записей в %s", len(s.data), flagFileBD)
	return nil
}
