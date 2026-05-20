package worker

import (
	"context"
	"sync"
)

// FanIn объединяет несколько каналов в один
func FanIn(ctx context.Context, inputs ...<-chan DeleteRequest) <-chan DeleteRequest {
	out := make(chan DeleteRequest, 100) // Буферизированный выходной канал

	var wg sync.WaitGroup

	for _, input := range inputs {
		wg.Add(1)
		go func(ch <-chan DeleteRequest) {
			defer wg.Done()
			for {
				select {
				case req, ok := <-ch:
					if !ok {
						return
					}
					select {
					case out <- req:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}(input)
	}

	// Закрываем выходной канал, когда все входные закрыты
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
