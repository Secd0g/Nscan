package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type PoolOptions struct {
	Concurrency    int
	PerItemTimeout time.Duration
}

// RunPool distributes items to a pool of workers with the specified concurrency and per-item timeout.
// It returns the number of items processed and a slice of errors encountered.
func RunPool[T any](
	ctx context.Context,
	items []T,
	opts PoolOptions,
	fn func(ctx context.Context, item T) error,
) (int, []error) {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 1
	}

	workCh := make(chan T, opts.Concurrency)
	var wg sync.WaitGroup

	var processedCount atomic.Int32
	var errs []error
	var errsMu sync.Mutex

	// Start workers
	for i := 0; i < opts.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range workCh {
				// Check parent context before processing
				if ctx.Err() != nil {
					continue
				}

				var err error
				if opts.PerItemTimeout > 0 {
					itemCtx, cancel := context.WithTimeout(ctx, opts.PerItemTimeout)
					err = fn(itemCtx, item)
					cancel()
				} else {
					err = fn(ctx, item)
				}

				if err != nil {
					errsMu.Lock()
					errs = append(errs, err)
					errsMu.Unlock()
				}
				processedCount.Add(1)
			}
		}()
	}

	// Distribute work
	go func() {
		defer close(workCh)
		for _, item := range items {
			select {
			case <-ctx.Done():
				return // Parent context cancelled, stop distributing
			case workCh <- item:
			}
		}
	}()

	wg.Wait()
	return int(processedCount.Load()), errs
}
