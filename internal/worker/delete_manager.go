package worker

import (
	"context"
	"sync"
	"time"

	"github.com/asbm77/go-musthave-shortener-tpl/internal/logger"
	"github.com/asbm77/go-musthave-shortener-tpl/internal/storage"
)

type DeleteRequest struct {
	UserID string
	URLs   []string
}

type DeleteManager struct {
	storage       storage.Storage
	requestChan   chan DeleteRequest
	bufferSize    int
	flushInterval time.Duration
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewDeleteManager(storage storage.Storage, bufferSize int, flushInterval time.Duration) *DeleteManager {
	return &DeleteManager{
		storage:       storage,
		requestChan:   make(chan DeleteRequest, bufferSize),
		bufferSize:    bufferSize,
		flushInterval: flushInterval,
	}
}

func (dm *DeleteManager) Start() {
	dm.ctx, dm.cancel = context.WithCancel(context.Background())
	dm.wg.Add(1)
	go dm.worker()
}

func (dm *DeleteManager) Stop() {
	if dm.cancel != nil {
		dm.cancel()
	}
	dm.wg.Wait()
	close(dm.requestChan)
}

// GetQueue возвращает канал для отправки запросов на удаление
func (dm *DeleteManager) GetQueue() chan<- DeleteRequest {
	return dm.requestChan
}

func (dm *DeleteManager) worker() {
	defer dm.wg.Done()

	buffer := make([]DeleteRequest, 0, dm.bufferSize)
	ticker := time.NewTicker(dm.flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(buffer) == 0 {
			return
		}

		// Группируем по userID
		userURLs := make(map[string][]string)
		for _, req := range buffer {
			userURLs[req.UserID] = append(userURLs[req.UserID], req.URLs...)
		}

		// Выполняем пакетное удаление для каждого пользователя
		for userID, urls := range userURLs {
			// Используем существующий метод DeleteUserURLs
			if err := dm.storage.DeleteUserURLs(dm.ctx, userID, urls); err != nil {
				logger.Logger.Errorw("Failed to delete URLs",
					"user_id", userID,
					"urls_count", len(urls),
					"error", err)
			} else {
				logger.Logger.Infow("Successfully deleted URLs",
					"user_id", userID,
					"urls_count", len(urls))
			}
		}

		// Очищаем буфер
		buffer = buffer[:0]
	}

	for {
		select {
		case req, ok := <-dm.requestChan:
			if !ok {
				flush()
				return
			}
			buffer = append(buffer, req)
			if len(buffer) >= dm.bufferSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-dm.ctx.Done():
			flush()
			return
		}
	}
}
