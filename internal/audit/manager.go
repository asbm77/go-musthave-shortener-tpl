package audit

import (
	"fmt"
	"sync"
)

type Manager struct {
	observers []Observer
	mu        sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		observers: make([]Observer, 0),
	}
}

func (m *Manager) Register(observer Observer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observers = append(m.observers, observer)
}

func (m *Manager) NotifyAll(event *Event) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, observer := range m.observers {
		go func(obs Observer) {
			if err := obs.Notify(event); err != nil {
				// Логируем ошибку, но не прерываем выполнение
				fmt.Printf("Audit notification error: %v\n", err)
			}
		}(observer)
	}
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for _, observer := range m.observers {
		if closer, ok := observer.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing observers: %v", errs)
	}
	return nil
}
