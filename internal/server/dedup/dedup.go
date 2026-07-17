package dedup

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yourname/nscan/internal/server/metrics"
)

const defaultTTL = 48 * time.Hour

// Dedup provides a per-task, per-kind distributed seen-set backed by Redis.
// kind is a short label such as "subdomain", "host", "url".
type Dedup struct {
	rdb *redis.Client
	ttl time.Duration
}

func New(rdb *redis.Client) *Dedup {
	return &Dedup{rdb: rdb, ttl: defaultTTL}
}

func seenKey(taskID, kind string) string {
	return fmt.Sprintf("seen:%s:%s", taskID, kind)
}

// FilterNew returns only the items not yet seen for (taskID, kind), and marks
// them as seen in Redis. Uses a pipeline to minimise round-trips.
func (d *Dedup) FilterNew(ctx context.Context, taskID, kind string, items []string) []string {
	if len(items) == 0 {
		return nil
	}
	key := seenKey(taskID, kind)

	// SADD each item individually in a pipeline; the return value tells us
	// how many members were actually added (1 = new, 0 = already seen).
	pipe := d.rdb.Pipeline()
	cmds := make([]*redis.IntCmd, len(items))
	for i, item := range items {
		cmds[i] = pipe.SAdd(ctx, key, item)
	}
	pipe.Expire(ctx, key, d.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		// On Redis error fall through — return all items so the pipeline doesn't stall.
		return items
	}

	out := make([]string, 0, len(items))
	for i, cmd := range cmds {
		if cmd.Val() > 0 {
			out = append(out, items[i])
		}
	}
	hits := len(items) - len(out)
	if hits > 0 {
		metrics.DedupHits.WithLabelValues(kind).Add(float64(hits))
	}
	if len(out) > 0 {
		metrics.DedupNew.WithLabelValues(kind).Add(float64(len(out)))
	}
	return out
}

// Clear removes the seen-sets for a task (called when a task finishes).
func (d *Dedup) Clear(ctx context.Context, taskID string) {
	var cursor uint64
	for {
		keys, next, err := d.rdb.Scan(ctx, cursor, fmt.Sprintf("seen:%s:*", taskID), 100).Result()
		if err != nil {
			break
		}
		if len(keys) > 0 {
			d.rdb.Del(ctx, keys...)
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
}
