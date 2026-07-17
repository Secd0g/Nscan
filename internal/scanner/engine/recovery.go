package engine

import (
	"fmt"

	"github.com/cockroachdb/pebble"
	"github.com/yourname/nscan/pkg/proto/scanv1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// TaskRecovery persists in-progress task state to PebbleDB so that
// tasks can be resumed after a scanner crash/restart.
type TaskRecovery struct {
	db  *pebble.DB
	log *zap.Logger
}

// NewTaskRecovery opens (or creates) a PebbleDB at dataDir/recovery.
func NewTaskRecovery(dataDir string, log *zap.Logger) (*TaskRecovery, error) {
	db, err := pebble.Open(dataDir+"/recovery", &pebble.Options{})
	if err != nil {
		return nil, fmt.Errorf("open pebble db: %w", err)
	}
	return &TaskRecovery{db: db, log: log}, nil
}

func taskKey(taskID string) []byte {
	return []byte("task:" + taskID)
}

// SaveTask persists a task before execution starts.
func (r *TaskRecovery) SaveTask(task *scanv1.ScanTask) error {
	data, err := proto.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}
	if err := r.db.Set(taskKey(task.TaskId), data, pebble.Sync); err != nil {
		return fmt.Errorf("pebble set: %w", err)
	}
	r.log.Debug("task persisted for recovery", zap.String("task_id", task.TaskId))
	return nil
}

// RemoveTask deletes a task after it completes (success or failure).
func (r *TaskRecovery) RemoveTask(taskID string) error {
	if err := r.db.Delete(taskKey(taskID), pebble.Sync); err != nil {
		return fmt.Errorf("pebble delete: %w", err)
	}
	r.log.Debug("task removed from recovery store", zap.String("task_id", taskID))
	return nil
}

// RecoverTasks loads all persisted tasks on startup.
func (r *TaskRecovery) RecoverTasks() ([]*scanv1.ScanTask, error) {
	iter, err := r.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("task:"),
		UpperBound: []byte("task;"), // ';' is the byte after ':'
	})
	if err != nil {
		return nil, fmt.Errorf("pebble iter: %w", err)
	}
	defer iter.Close()

	var tasks []*scanv1.ScanTask
	for iter.First(); iter.Valid(); iter.Next() {
		val, err := iter.ValueAndErr()
		if err != nil {
			return nil, fmt.Errorf("pebble value: %w", err)
		}
		var task scanv1.ScanTask
		if err := proto.Unmarshal(val, &task); err != nil {
			r.log.Warn("skip corrupt recovery entry",
				zap.String("key", string(iter.Key())),
				zap.Error(err),
			)
			continue
		}
		tasks = append(tasks, &task)
	}
	return tasks, nil
}

// Close closes the underlying PebbleDB.
func (r *TaskRecovery) Close() error {
	return r.db.Close()
}
