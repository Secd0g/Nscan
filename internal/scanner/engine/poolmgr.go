package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/panjf2000/ants/v2"
	"go.uber.org/zap"
)

// defaultPoolSizes defines reasonable default concurrency for each scan module.
var defaultPoolSizes = map[string]int{
	"subdomain": 10,
	"port":      20,
	"http":      20,
	"nuclei":    5,
	"dir":       10,
	"brute":     5,
	"crawler":   20,
	"sensitive": 10,
	"search":    3,
}

// PoolStats holds runtime statistics for a single pool.
type PoolStats struct {
	Running int `json:"running"`
	Free    int `json:"free"`
	Cap     int `json:"cap"`
}

// PoolManager manages named ants goroutine pools, one per scan module.
type PoolManager struct {
	mu    sync.RWMutex
	pools map[string]*ants.Pool
	log   *zap.Logger
}

// NewPoolManager creates a new PoolManager.
func NewPoolManager(log *zap.Logger) *PoolManager {
	return &PoolManager{
		pools: make(map[string]*ants.Pool),
		log:   log,
	}
}

// GetOrCreate returns the pool for the given name, creating it with the
// specified size if it does not exist. If size <= 0 and a default exists in
// defaultPoolSizes, the default is used; otherwise it falls back to 10.
func (pm *PoolManager) GetOrCreate(name string, size int) *ants.Pool {
	pm.mu.RLock()
	if p, ok := pm.pools[name]; ok {
		pm.mu.RUnlock()
		return p
	}
	pm.mu.RUnlock()

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Double-check after acquiring write lock.
	if p, ok := pm.pools[name]; ok {
		return p
	}

	if size <= 0 {
		if d, ok := defaultPoolSizes[name]; ok {
			size = d
		} else {
			size = 10
		}
	}

	p, err := ants.NewPool(size, ants.WithPreAlloc(false), ants.WithNonblocking(false))
	if err != nil {
		pm.log.Error("failed to create pool, falling back to size 1",
			zap.String("pool", name), zap.Error(err))
		p, _ = ants.NewPool(1)
	}

	pm.log.Info("pool created", zap.String("name", name), zap.Int("size", size))
	pm.pools[name] = p
	return p
}

// Tune resizes an existing pool at runtime. Returns an error if the pool
// does not exist.
func (pm *PoolManager) Tune(name string, size int) error {
	pm.mu.RLock()
	p, ok := pm.pools[name]
	pm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("pool %q not found", name)
	}
	p.Tune(size)
	pm.log.Info("pool resized", zap.String("name", name), zap.Int("new_size", size))
	return nil
}

// Stats returns per-pool statistics.
func (pm *PoolManager) Stats() map[string]PoolStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	out := make(map[string]PoolStats, len(pm.pools))
	for name, p := range pm.pools {
		out[name] = PoolStats{
			Running: p.Running(),
			Free:    p.Free(),
			Cap:     p.Cap(),
		}
	}
	return out
}

// Release shuts down all pools, waiting for workers to finish.
func (pm *PoolManager) Release() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for name, p := range pm.pools {
		p.Release()
		pm.log.Info("pool released", zap.String("name", name))
	}
	pm.pools = make(map[string]*ants.Pool)
}

// RunPoolManaged distributes items across a named ants pool managed by pm.
// It behaves like RunPool but uses a bounded goroutine pool instead of raw
// goroutines, providing global concurrency control.
func RunPoolManaged[T any](
	ctx context.Context,
	pm *PoolManager,
	poolName string,
	items []T,
	opts PoolOptions,
	fn func(ctx context.Context, item T) error,
) (int, []error) {
	pool := pm.GetOrCreate(poolName, opts.Concurrency)

	var processedCount atomic.Int32
	var errs []error
	var errsMu sync.Mutex
	var wg sync.WaitGroup

	for _, item := range items {
		if ctx.Err() != nil {
			break
		}

		item := item // capture loop variable
		wg.Add(1)
		err := pool.Submit(func() {
			defer wg.Done()

			if ctx.Err() != nil {
				return
			}

			var execErr error
			if opts.PerItemTimeout > 0 {
				itemCtx, cancel := context.WithTimeout(ctx, opts.PerItemTimeout)
				execErr = fn(itemCtx, item)
				cancel()
			} else {
				execErr = fn(ctx, item)
			}

			if execErr != nil {
				errsMu.Lock()
				errs = append(errs, execErr)
				errsMu.Unlock()
			}
			processedCount.Add(1)
		})
		if err != nil {
			// Submit failed (shouldn't happen with blocking mode, but be safe).
			wg.Done()
			errsMu.Lock()
			errs = append(errs, fmt.Errorf("pool submit for %s: %w", poolName, err))
			errsMu.Unlock()
		}
	}

	wg.Wait()
	return int(processedCount.Load()), errs
}
