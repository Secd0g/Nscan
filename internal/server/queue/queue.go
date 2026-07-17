package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yourname/nscan/internal/server/metrics"
	"github.com/yourname/nscan/pkg/models"
)

const (
	DefaultLeaseTTL   = 60 * time.Second
	DefaultRenewEvery = 15 * time.Second
	MaxAttempts       = 3
)

// Queue encapsulates all Redis queue operations for subtask scheduling.
type Queue struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Queue {
	return &Queue{rdb: rdb}
}

// ── Key helpers ───────────────────────────────────────────────────────────────

func queueKey(capability string) string { return fmt.Sprintf("queue:%s", capability) }
func subtaskKey(id string) string       { return fmt.Sprintf("subtask:%s", id) }
func leaseKey(id string) string         { return fmt.Sprintf("subtask:%s:lease", id) }
func pendingKey(taskID string) string   { return fmt.Sprintf("task:%s:pending", taskID) }
func aggregateKey(taskID, stage string) string {
	return fmt.Sprintf("task:%s:aggregate:%s", taskID, stage)
}
func cancelKey(taskID string) string         { return fmt.Sprintf("task:%s:cancelled", taskID) }
func runKey(taskID string) string            { return fmt.Sprintf("task:%s:run", taskID) }
func deadLetterKey(capability string) string { return fmt.Sprintf("dead-letter:%s", capability) }
func nodeLeaseSetKey(nodeID string) string   { return fmt.Sprintf("node:%s:leases", nodeID) }

// ── Public API ────────────────────────────────────────────────────────────────

// Enqueue pushes a subtask onto the capability queue and registers it in the
// task's pending set. Also stores subtask data for lease recovery.
func (q *Queue) Enqueue(ctx context.Context, st *models.Subtask) error {
	data, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("queue: marshal subtask: %w", err)
	}
	pipe := q.rdb.Pipeline()
	pipe.RPush(ctx, queueKey(st.Capability), data)
	pipe.SAdd(ctx, pendingKey(st.TaskID.Hex()), st.ID)
	// Store subtask data so lease recovery can reconstruct it.
	pipe.Set(ctx, subtaskKey(st.ID), data, 24*time.Hour)
	pipe.Set(ctx, "subtask:"+st.ID+":meta", data, 7*24*time.Hour)
	_, err = pipe.Exec(ctx)
	return err
}

// BLPop blocks until a subtask is available on the given capability queue.
// Returns nil subtask (no error) on timeout so callers can loop.
func (q *Queue) BLPop(ctx context.Context, capability string, timeout time.Duration) (*models.Subtask, error) {
	res, err := q.rdb.BLPop(ctx, timeout, queueKey(capability)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("queue: blpop: %w", err)
	}
	var st models.Subtask
	if err := json.Unmarshal([]byte(res[1]), &st); err != nil {
		return nil, fmt.Errorf("queue: unmarshal subtask: %w", err)
	}
	return &st, nil
}

// Lease atomically acquires a lease for nodeID. Returns false if already held
// by another node. Also registers the subtaskID in the node's lease tracking set.
func (q *Queue) Lease(ctx context.Context, subtaskID, nodeID string, ttl time.Duration) (bool, error) {
	res, err := q.rdb.Eval(ctx, luaLease,
		[]string{leaseKey(subtaskID)},
		nodeID, int64(ttl.Seconds()),
	).Int64()
	if err == nil && res == 1 {
		// Track which subtasks this node holds so we can reclaim them on disconnect.
		q.rdb.SAdd(ctx, nodeLeaseSetKey(nodeID), subtaskID)
	}
	return res == 1, err
}

// RegisterLease adds subtaskID to the node's lease tracking set (idempotent).
func (q *Queue) RegisterLease(ctx context.Context, subtaskID, nodeID string) {
	q.rdb.SAdd(ctx, nodeLeaseSetKey(nodeID), subtaskID)
}

// UnregisterLease removes subtaskID from the node's lease tracking set.
func (q *Queue) UnregisterLease(ctx context.Context, subtaskID, nodeID string) {
	q.rdb.SRem(ctx, nodeLeaseSetKey(nodeID), subtaskID)
}

// Renew extends the lease TTL. Returns false if lease is no longer held by nodeID.
func (q *Queue) Renew(ctx context.Context, subtaskID, nodeID string, ttl time.Duration) (bool, error) {
	res, err := q.rdb.Eval(ctx, luaRenew,
		[]string{leaseKey(subtaskID)},
		nodeID, int64(ttl.Seconds()),
	).Int64()
	return res == 1, err
}

// Complete atomically releases the lease and removes subtaskID from the task's
// pending set. Returns false if lease not held by nodeID.
func (q *Queue) Complete(ctx context.Context, taskID, subtaskID, nodeID string) (bool, error) {
	res, err := q.rdb.Eval(ctx, luaComplete,
		[]string{leaseKey(subtaskID), pendingKey(taskID)},
		nodeID, subtaskID,
	).Int64()
	if err == nil {
		q.rdb.SRem(ctx, nodeLeaseSetKey(nodeID), subtaskID)
	}
	return res == 1, err
}

// Release drops a lease while keeping the subtask pending for retry.
func (q *Queue) Release(ctx context.Context, subtaskID, nodeID string) (bool, error) {
	res, err := q.rdb.Eval(ctx, luaRelease, []string{leaseKey(subtaskID)}, nodeID).Int64()
	if err == nil && res == 1 {
		q.rdb.SRem(ctx, nodeLeaseSetKey(nodeID), subtaskID)
	}
	return res == 1, err
}

// GetSubtask retrieves the durable metadata used to retry a failed subtask.
func (q *Queue) GetSubtask(ctx context.Context, subtaskID string) (*models.Subtask, error) {
	data, err := q.rdb.Get(ctx, subtaskKey(subtaskID)).Result()
	if err != nil {
		return nil, err
	}
	var st models.Subtask
	if err := json.Unmarshal([]byte(data), &st); err != nil {
		return nil, fmt.Errorf("queue: unmarshal subtask metadata: %w", err)
	}
	return &st, nil
}

// CancelTask marks a task cancelled. Workers check this marker before taking
// work so already queued messages cannot execute after cancellation.
func (q *Queue) CancelTask(ctx context.Context, taskID string) error {
	return q.rdb.Set(ctx, cancelKey(taskID), "1", 24*time.Hour).Err()
}

func (q *Queue) IsTaskCancelled(ctx context.Context, taskID string) (bool, error) {
	v, err := q.rdb.Exists(ctx, cancelKey(taskID)).Result()
	return v > 0, err
}

func (q *Queue) SetTaskRunID(ctx context.Context, taskID, runID string) error {
	return q.rdb.Set(ctx, runKey(taskID), runID, 7*24*time.Hour).Err()
}

func (q *Queue) TaskRunID(ctx context.Context, taskID string) (string, error) {
	v, err := q.rdb.Get(ctx, runKey(taskID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return v, err
}

// DropPending removes a queued or abandoned subtask from task bookkeeping.
func (q *Queue) DropPending(ctx context.Context, taskID, subtaskID string) error {
	return q.rdb.SRem(ctx, pendingKey(taskID), subtaskID).Err()
}

// AppendOutput appends serialised output bytes to the stage aggregate list.
func (q *Queue) AppendOutput(ctx context.Context, taskID, stage string, output []byte) error {
	return q.rdb.RPush(ctx, aggregateKey(taskID, stage), output).Err()
}

// CollectOutput returns all output chunks for a given task stage and deletes
// the aggregate list atomically.
func (q *Queue) CollectOutput(ctx context.Context, taskID, stage string) ([][]byte, error) {
	key := aggregateKey(taskID, stage)
	pipe := q.rdb.Pipeline()
	lrange := pipe.LRange(ctx, key, 0, -1)
	pipe.Del(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	strs := lrange.Val()
	out := make([][]byte, len(strs))
	for i, s := range strs {
		out[i] = []byte(s)
	}
	return out, nil
}

// PendingCount returns how many subtasks are still pending for a task+stage.
// If stage is empty, it returns the total pending count across all stages.
func (q *Queue) PendingCount(ctx context.Context, taskID string) (int64, error) {
	return q.rdb.SCard(ctx, pendingKey(taskID)).Result()
}

// PendingMembers returns all pending subtask IDs for a task.
func (q *Queue) PendingMembers(ctx context.Context, taskID string) ([]string, error) {
	return q.rdb.SMembers(ctx, pendingKey(taskID)).Result()
}

// Requeue re-enqueues a subtask (Attempt++) after a failed or expired lease.
func (q *Queue) Requeue(ctx context.Context, st *models.Subtask) error {
	st.Attempt++
	st.LeasedBy = ""
	st.Status = models.SubtaskPending
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	pipe := q.rdb.Pipeline()
	// LPUSH to front so retried subtasks are processed before new ones.
	pipe.LPush(ctx, queueKey(st.Capability), data)
	pipe.Set(ctx, subtaskKey(st.ID), data, 24*time.Hour)
	pipe.Set(ctx, "subtask:"+st.ID+":meta", data, 7*24*time.Hour)
	_, err = pipe.Exec(ctx)
	return err
}

// DeadLetter moves a subtask to the dead-letter queue.
func (q *Queue) DeadLetter(ctx context.Context, taskID string, st *models.Subtask, reason string) error {
	metrics.DeadLetterTotal.WithLabelValues(st.Capability).Inc()
	st.Status = models.SubtaskDeadLetter
	st.ErrorMsg = reason
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	pipe := q.rdb.Pipeline()
	pipe.RPush(ctx, deadLetterKey(st.Capability), data)
	// Remove from pending so the task can still complete.
	pipe.SRem(ctx, pendingKey(taskID), st.ID)
	_, err = pipe.Exec(ctx)
	return err
}

// LeaseOwner returns the nodeID currently holding the lease, or "" if none.
func (q *Queue) LeaseOwner(ctx context.Context, subtaskID string) (string, error) {
	v, err := q.rdb.Get(ctx, leaseKey(subtaskID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return v, err
}

// ReleaseNodeLeases reclaims all subtasks leased by nodeID (called on disconnect).
// Each subtask is re-queued (Attempt++) or moved to dead-letter if MaxAttempts exceeded.
func (q *Queue) ReleaseNodeLeases(ctx context.Context, nodeID string) (released int, err error) {
	key := nodeLeaseSetKey(nodeID)
	members, err := q.rdb.SMembers(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("queue: release node leases: %w", err)
	}
	for _, subtaskID := range members {
		data, err := q.rdb.Get(ctx, subtaskKey(subtaskID)).Result()
		if err == redis.Nil {
			// Subtask data expired or never stored; just remove from pending tracking.
			q.rdb.SRem(ctx, key, subtaskID)
			continue
		}
		if err != nil {
			continue
		}
		var st models.Subtask
		if json.Unmarshal([]byte(data), &st) != nil {
			continue
		}
		q.rdb.Del(ctx, leaseKey(subtaskID))
		q.rdb.SRem(ctx, key, subtaskID)
		metrics.LeaseExpiredTotal.Inc()

		if st.Attempt >= MaxAttempts {
			_ = q.DeadLetter(ctx, st.TaskID.Hex(), &st, "node offline: max attempts reached")
		} else {
			_ = q.Requeue(ctx, &st)
		}
		released++
	}
	q.rdb.Del(ctx, key)
	return released, nil
}

// ListDeadLetterByTask returns all dead-lettered subtasks for a given taskID.
// It scans all dead-letter queues and filters by TaskID.
func (q *Queue) ListDeadLetterByTask(ctx context.Context, taskID string) ([]*models.Subtask, error) {
	var cursor uint64
	var results []*models.Subtask
	for {
		keys, next, err := q.rdb.Scan(ctx, cursor, "dead-letter:*", 100).Result()
		if err != nil {
			return nil, fmt.Errorf("queue: scan dead-letter: %w", err)
		}
		for _, key := range keys {
			items, err := q.rdb.LRange(ctx, key, 0, -1).Result()
			if err != nil {
				continue
			}
			for _, item := range items {
				var st models.Subtask
				if json.Unmarshal([]byte(item), &st) != nil {
					continue
				}
				if st.TaskID.Hex() == taskID {
					results = append(results, &st)
				}
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return results, nil
}

// RetryDeadLetter removes a specific subtask from the dead-letter queue and
// re-enqueues it with Attempt reset to 0.
func (q *Queue) RetryDeadLetter(ctx context.Context, subtaskID string) error {
	var cursor uint64
	for {
		keys, next, err := q.rdb.Scan(ctx, cursor, "dead-letter:*", 100).Result()
		if err != nil {
			return fmt.Errorf("queue: scan dead-letter: %w", err)
		}
		for _, key := range keys {
			items, err := q.rdb.LRange(ctx, key, 0, -1).Result()
			if err != nil {
				continue
			}
			for _, item := range items {
				var st models.Subtask
				if json.Unmarshal([]byte(item), &st) != nil {
					continue
				}
				if st.ID != subtaskID {
					continue
				}
				// Remove from dead-letter list (LREM count=1 removes first match).
				q.rdb.LRem(ctx, key, 1, item)
				st.Attempt = 0
				st.Status = models.SubtaskPending
				st.ErrorMsg = ""
				return q.Enqueue(ctx, &st)
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return fmt.Errorf("queue: subtask %s not found in dead-letter", subtaskID)
}

// ClearTask removes all Redis state for a completed/cancelled/rescanned task:
// pending set + all stage aggregate lists.
func (q *Queue) ClearTask(ctx context.Context, taskID string) error {
	keys := []string{pendingKey(taskID)}
	// Scan for aggregate keys (task:{id}:aggregate:*)
	var cursor uint64
	for {
		batch, next, err := q.rdb.Scan(ctx, cursor, fmt.Sprintf("task:%s:aggregate:*", taskID), 100).Result()
		if err != nil {
			break
		}
		keys = append(keys, batch...)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return q.rdb.Del(ctx, keys...).Err()
}

func (q *Queue) ClearCancellation(ctx context.Context, taskID string) error {
	return q.rdb.Del(ctx, cancelKey(taskID)).Err()
}
