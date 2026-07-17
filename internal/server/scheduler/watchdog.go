package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yourname/nscan/internal/server/metrics"
	"github.com/yourname/nscan/internal/server/queue"
	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

const (
	watchdogInterval = 30 * time.Second
)

// Watchdog periodically scans for subtasks whose leases have expired (the
// holding node crashed or stalled) and re-enqueues them or sends them to the
// dead-letter queue after MaxAttempts.
type Watchdog struct {
	q            *queue.Queue
	rdb          *redis.Client
	log          *zap.Logger
	onDeadLetter func(context.Context, string, string)
}

// NewWatchdog creates a Watchdog. Call Run in a goroutine.
func NewWatchdog(q *queue.Queue, rdb *redis.Client, log *zap.Logger, onDeadLetter func(context.Context, string, string)) *Watchdog {
	return &Watchdog{q: q, rdb: rdb, log: log, onDeadLetter: onDeadLetter}
}

// Run loops every watchdogInterval until ctx is cancelled.
func (w *Watchdog) Run(ctx context.Context) {
	ticker := time.NewTicker(watchdogInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.scan(ctx)
		}
	}
}

// scan checks all tracked pending subtask sets and reclaims orphaned ones.
// Strategy: we scan keys matching "task:*:pending" and for each subtask ID in
// the set check whether its lease key exists. If the lease is gone but the
// subtask ID is still in pending, the holder crashed → requeue.
func (w *Watchdog) scan(ctx context.Context) {
	var cursor uint64
	for {
		keys, next, err := w.rdb.Scan(ctx, cursor, "task:*:pending", 100).Result()
		if err != nil {
			w.log.Error("watchdog scan failed", zap.Error(err))
			return
		}
		for _, key := range keys {
			taskID := pendingKeyToTaskID(key)
			w.checkPendingSet(ctx, taskID)
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
}

func pendingKeyToTaskID(key string) string {
	// key = "task:<taskID>:pending"
	if len(key) > len("task::pending") {
		return key[len("task:") : len(key)-len(":pending")]
	}
	return ""
}

func (w *Watchdog) checkPendingSet(ctx context.Context, taskID string) {
	if taskID == "" {
		return
	}
	subtaskIDs, err := w.q.PendingMembers(ctx, taskID)
	if err != nil || len(subtaskIDs) == 0 {
		return
	}

	for _, stID := range subtaskIDs {
		owner, err := w.q.LeaseOwner(ctx, stID)
		if err != nil {
			continue
		}
		if owner != "" {
			// Lease still alive; holder is still working.
			continue
		}

		// Lease expired; try to recover the subtask from the subtask hash.
		st, err := w.loadSubtask(ctx, stID, taskID)
		if err != nil {
			w.log.Warn("watchdog: cannot load orphaned subtask",
				zap.String("subtask_id", stID),
				zap.Error(err),
			)
			continue
		}

		metrics.LeaseExpiredTotal.Inc()
		if st.Attempt >= queue.MaxAttempts {
			reason := fmt.Sprintf("max attempts (%d) reached", queue.MaxAttempts)
			w.log.Warn("watchdog: dead-lettering subtask",
				zap.String("subtask_id", stID),
				zap.String("task_id", taskID),
			)
			if err := w.q.DeadLetter(ctx, taskID, st, reason); err == nil && w.onDeadLetter != nil {
				w.onDeadLetter(ctx, taskID, reason)
			}
		} else {
			w.log.Info("watchdog: re-queuing orphaned subtask",
				zap.String("subtask_id", stID),
				zap.String("task_id", taskID),
				zap.Int("attempt", st.Attempt),
			)
			_ = w.q.Requeue(ctx, st)
		}
	}
}

// loadSubtask retrieves a Subtask from the "subtask:<id>" Redis hash.
// If not found, it constructs a minimal recovery stub so we can at least
// re-enqueue with the right capability.
func (w *Watchdog) loadSubtask(ctx context.Context, stID, taskID string) (*models.Subtask, error) {
	data, err := w.rdb.Get(ctx, "subtask:"+stID+":meta").Bytes()
	if err == nil {
		var st models.Subtask
		if jsonErr := json.Unmarshal(data, &st); jsonErr == nil {
			return &st, nil
		}
	}
	// Stub: we don't have the full subtask; return a minimal one so the
	// watchdog can remove it from pending and dead-letter it gracefully.
	return &models.Subtask{
		ID:         stID,
		Capability: "unknown",
		Attempt:    queue.MaxAttempts, // force dead-letter
		Status:     models.SubtaskLeased,
		UpdatedAt:  time.Now(),
	}, nil
}

// StoreSubtaskMeta persists a subtask as JSON under "subtask:<id>:meta" with
// a TTL so the watchdog can recover it after a node crash.
// Call this from Enqueue's caller (Scheduler) just before pushing to the queue.
func StoreSubtaskMeta(ctx context.Context, rdb *redis.Client, st *models.Subtask) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	// TTL = 7 days; after that we no longer need recovery metadata.
	return rdb.Set(ctx, "subtask:"+st.ID+":meta", data, 7*24*time.Hour).Err()
}
