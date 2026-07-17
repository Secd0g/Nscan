package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/allegro/bigcache/v3"
	"go.uber.org/zap"
)

// LocalDedup provides in-process deduplication using BigCache.
// It acts as a fast first-tier filter so duplicate results never reach
// the server/Redis layer.
type LocalDedup struct {
	cache *bigcache.BigCache
	mu    sync.Mutex // guards Reset
	log   *zap.Logger
}

// NewLocalDedup creates a LocalDedup with a 30-minute eviction window.
func NewLocalDedup(log *zap.Logger) (*LocalDedup, error) {
	cfg := bigcache.DefaultConfig(30 * time.Minute)
	cfg.CleanWindow = 5 * time.Minute
	cfg.Verbose = false

	cache, err := bigcache.New(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("bigcache init: %w", err)
	}
	return &LocalDedup{cache: cache, log: log}, nil
}

// IsSeen returns true if the (kind, key) pair was already recorded.
// On first encounter it marks the pair as seen and returns false.
func (d *LocalDedup) IsSeen(kind, key string) bool {
	composite := kind + ":" + key
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, err := d.cache.Get(composite); err == nil {
		return true // already seen
	}
	// Mark as seen; value doesn't matter.
	_ = d.cache.Set(composite, []byte{1})
	return false
}

// Reset clears all entries. Call when a task finishes.
func (d *LocalDedup) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.cache.Reset(); err != nil {
		d.log.Warn("local dedup reset failed", zap.Error(err))
	}
}

// ResultKey builds a dedup key from a ScanResult by hashing Type + Data.
func ResultKey(r *ScanResult) string {
	h := sha256.New()
	h.Write([]byte(r.Type))
	h.Write([]byte(":"))
	h.Write(r.Data)
	return hex.EncodeToString(h.Sum(nil))
}
