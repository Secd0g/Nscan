package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/yourname/nscan/internal/server/queue"
	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func newTestQueue(t *testing.T) (*queue.Queue, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return queue.New(rdb), mr
}

func subtask(stage string) *models.Subtask {
	return &models.Subtask{
		ID:         "st-" + stage + "-1",
		TaskID:     primitive.NewObjectID(),
		Stage:      stage,
		Capability: stage,
		Targets:    []string{"example.com"},
		Params:     map[string]string{},
		Status:     models.SubtaskPending,
		CreatedAt:  time.Now(),
	}
}

func TestEnqueueBLPop(t *testing.T) {
	q, _ := newTestQueue(t)
	ctx := context.Background()

	st := subtask("port")
	if err := q.Enqueue(ctx, st); err != nil {
		t.Fatal(err)
	}

	got, err := q.BLPop(ctx, "port", time.Second)
	if err != nil || got == nil {
		t.Fatalf("BLPop: got=%v err=%v", got, err)
	}
	if got.ID != st.ID {
		t.Errorf("id mismatch: %q != %q", got.ID, st.ID)
	}
}

func TestBLPopTimeout(t *testing.T) {
	q, _ := newTestQueue(t)
	got, err := q.BLPop(context.Background(), "empty", 50*time.Millisecond)
	if err != nil || got != nil {
		t.Fatalf("expected nil subtask on timeout, got=%v err=%v", got, err)
	}
}

func TestLeaseAndRenew(t *testing.T) {
	q, mr := newTestQueue(t)
	ctx := context.Background()

	st := subtask("http")
	ok, err := q.Lease(ctx, st.ID, "node-1", 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("Lease failed: %v %v", ok, err)
	}

	// same node can renew
	ok, err = q.Renew(ctx, st.ID, "node-1", 30*time.Second)
	if err != nil || !ok {
		t.Fatal("Renew failed")
	}

	// different node cannot steal
	ok, err = q.Lease(ctx, st.ID, "node-2", 30*time.Second)
	if err != nil || ok {
		t.Fatal("Lease should fail when already held")
	}

	// after expiry, another node can acquire
	mr.FastForward(31 * time.Second)
	ok, err = q.Lease(ctx, st.ID, "node-2", 30*time.Second)
	if err != nil || !ok {
		t.Fatal("Lease should succeed after expiry")
	}
}

func TestComplete(t *testing.T) {
	q, _ := newTestQueue(t)
	ctx := context.Background()

	st := subtask("vuln")
	taskID := st.TaskID.Hex()

	if err := q.Enqueue(ctx, st); err != nil {
		t.Fatal(err)
	}
	q.Lease(ctx, st.ID, "node-1", 30*time.Second) //nolint

	ok, err := q.Complete(ctx, taskID, st.ID, "node-1")
	if err != nil || !ok {
		t.Fatalf("Complete failed: %v %v", ok, err)
	}

	count, err := q.PendingCount(ctx, taskID)
	if err != nil || count != 0 {
		t.Fatalf("pending count should be 0, got %d", count)
	}

	// wrong node cannot complete
	if err := q.Enqueue(ctx, st); err != nil {
		t.Fatal(err)
	}
	q.Lease(ctx, st.ID, "node-1", 30*time.Second) //nolint
	ok, _ = q.Complete(ctx, taskID, st.ID, "node-2")
	if ok {
		t.Fatal("wrong node should not be able to complete")
	}
}

func TestReleaseKeepsPendingAndRefreshesMetadata(t *testing.T) {
	q, _ := newTestQueue(t)
	ctx := context.Background()
	st := subtask("http")
	if err := q.Enqueue(ctx, st); err != nil { t.Fatal(err) }
	if ok, err := q.Lease(ctx, st.ID, "node-1", 30*time.Second); err != nil || !ok { t.Fatal(err) }
	if ok, err := q.Release(ctx, st.ID, "node-1"); err != nil || !ok { t.Fatal(err) }
	if n, err := q.PendingCount(ctx, st.TaskID.Hex()); err != nil || n != 1 { t.Fatalf("release must keep pending, got %d %v", n, err) }
	if err := q.Requeue(ctx, st); err != nil { t.Fatal(err) }
	got, err := q.GetSubtask(ctx, st.ID)
	if err != nil || got.Attempt != 1 { t.Fatalf("metadata attempt not refreshed: %+v %v", got, err) }
}

func TestCancelTaskAndDropPending(t *testing.T) {
	q, _ := newTestQueue(t)
	ctx := context.Background()
	st := subtask("dir")
	if err := q.Enqueue(ctx, st); err != nil {
		t.Fatal(err)
	}
	if err := q.CancelTask(ctx, st.TaskID.Hex()); err != nil {
		t.Fatal(err)
	}
	cancelled, err := q.IsTaskCancelled(ctx, st.TaskID.Hex())
	if err != nil || !cancelled {
		t.Fatalf("expected task to be cancelled, got %v %v", cancelled, err)
	}
	if err := q.DropPending(ctx, st.TaskID.Hex(), st.ID); err != nil {
		t.Fatal(err)
	}
	count, err := q.PendingCount(ctx, st.TaskID.Hex())
	if err != nil || count != 0 {
		t.Fatalf("expected no pending subtasks, got %d %v", count, err)
	}
	if err := q.ClearCancellation(ctx, st.TaskID.Hex()); err != nil {
		t.Fatal(err)
	}
	cancelled, err = q.IsTaskCancelled(ctx, st.TaskID.Hex())
	if err != nil || cancelled {
		t.Fatalf("expected cancellation marker cleared, got %v %v", cancelled, err)
	}
}

func TestTaskRunID(t *testing.T) {
	q, _ := newTestQueue(t)
	ctx := context.Background()
	taskID := primitive.NewObjectID().Hex()
	if err := q.SetTaskRunID(ctx, taskID, "run-2"); err != nil {
		t.Fatal(err)
	}
	got, err := q.TaskRunID(ctx, taskID)
	if err != nil || got != "run-2" {
		t.Fatalf("unexpected run id %q %v", got, err)
	}
}

func TestRequeue(t *testing.T) {
	q, _ := newTestQueue(t)
	ctx := context.Background()

	st := subtask("subdomain")
	if err := q.Enqueue(ctx, st); err != nil {
		t.Fatal(err)
	}

	if err := q.Requeue(ctx, st); err != nil {
		t.Fatal(err)
	}
	if st.Attempt != 1 {
		t.Errorf("Attempt should be 1, got %d", st.Attempt)
	}
}

func TestDeadLetter(t *testing.T) {
	q, _ := newTestQueue(t)
	ctx := context.Background()

	st := subtask("bbot")
	taskID := st.TaskID.Hex()
	q.Enqueue(ctx, st) //nolint

	if err := q.DeadLetter(ctx, taskID, st, "max attempts"); err != nil {
		t.Fatal(err)
	}

	// should be removed from pending
	count, _ := q.PendingCount(ctx, taskID)
	if count != 0 {
		t.Fatalf("pending count should be 0 after dead-letter, got %d", count)
	}
}

func TestAggregateOutput(t *testing.T) {
	q, _ := newTestQueue(t)
	ctx := context.Background()

	taskID := primitive.NewObjectID().Hex()
	q.AppendOutput(ctx, taskID, "port", []byte(`{"host":"a"}`)) //nolint
	q.AppendOutput(ctx, taskID, "port", []byte(`{"host":"b"}`)) //nolint

	chunks, err := q.CollectOutput(ctx, taskID, "port")
	if err != nil || len(chunks) != 2 {
		t.Fatalf("CollectOutput: len=%d err=%v", len(chunks), err)
	}

	// second call should be empty (list deleted)
	chunks2, _ := q.CollectOutput(ctx, taskID, "port")
	if len(chunks2) != 0 {
		t.Fatal("aggregate list should be deleted after collect")
	}
}
