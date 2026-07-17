package engine

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunPool(t *testing.T) {
	t.Run("concurrency and processing", func(t *testing.T) {
		items := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
		var processed atomic.Int32
		
		opts := PoolOptions{
			Concurrency: 3,
		}
		
		count, errs := RunPool(context.Background(), items, opts, func(ctx context.Context, item int) error {
			time.Sleep(10 * time.Millisecond)
			processed.Add(1)
			if item%2 == 0 {
				return errors.New("even error")
			}
			return nil
		})
		
		if count != 10 {
			t.Errorf("expected 10 processed items, got %d", count)
		}
		if processed.Load() != 10 {
			t.Errorf("expected 10 processed items from atomic, got %d", processed.Load())
		}
		if len(errs) != 5 {
			t.Errorf("expected 5 errors, got %d", len(errs))
		}
	})

	t.Run("per-item timeout", func(t *testing.T) {
		items := []int{1, 2}
		
		opts := PoolOptions{
			Concurrency:    2,
			PerItemTimeout: 50 * time.Millisecond,
		}
		
		count, errs := RunPool(context.Background(), items, opts, func(ctx context.Context, item int) error {
			if item == 1 {
				time.Sleep(100 * time.Millisecond) // Will timeout
			} else {
				time.Sleep(10 * time.Millisecond) // Will succeed
			}
			return ctx.Err()
		})
		
		if count != 2 {
			t.Errorf("expected 2 processed items, got %d", count)
		}
		
		// One should have context.DeadlineExceeded
		hasTimeout := false
		for _, err := range errs {
			if err == context.DeadlineExceeded {
				hasTimeout = true
			}
		}
		if !hasTimeout {
			t.Errorf("expected context deadline exceeded error")
		}
	})

	t.Run("parent cancellation", func(t *testing.T) {
		items := []int{1, 2, 3, 4, 5}
		
		opts := PoolOptions{
			Concurrency: 1, // Ensure sequential processing to test cancellation
		}
		
		ctx, cancel := context.WithCancel(context.Background())
		
		count, errs := RunPool(ctx, items, opts, func(tctx context.Context, item int) error {
			if item == 2 {
				cancel()
			}
			time.Sleep(50 * time.Millisecond)
			return nil
		})
		
		if count > 3 {
			t.Errorf("expected at most 3 items processed, got %d", count)
		}
		
		// No errors from fn, cancellation skips processing
		if len(errs) != 0 {
			t.Errorf("expected 0 errors, got %d", len(errs))
		}
	})
}
