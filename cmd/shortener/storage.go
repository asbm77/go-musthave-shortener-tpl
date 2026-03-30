package main

import "errors"

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
