package scheduler

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/yourname/nscan/pkg/models"
)

// chunkSize returns the number of targets per subtask for a given stage.
// These defaults balance parallelism with per-process overhead.
var defaultChunkSize = map[string]int{
	"search":     500,
	"subdomain":  50,
	"bbot":       10,
	"findomain":  50,
	"shuffledns": 100,
	"port":       200,
	"http":       500,
	"httpx":      500,
	"vuln":       200,
	"nuclei":     200,
	"brute":      50,
	"dir":        100,
	"crawler":    300,
	"sensitive":  300,
}

const defaultFallbackChunk = 100

func chunkSizeFor(stage string) int {
	if n, ok := defaultChunkSize[stage]; ok {
		return n
	}
	return defaultFallbackChunk
}

// SplitFirstStage splits the first stage of a task into subtasks using
// task.Targets. Subsequent stages are handled by the Aggregator after the
// first stage completes.
func SplitFirstStage(task *models.Task) ([]*models.Subtask, error) {
	if len(task.Config.Stages) == 0 {
		return nil, fmt.Errorf("task has no stages")
	}
	stage := task.Config.Stages[0]
	return splitTargets(task, stage, task.Targets), nil
}

// SplitStage splits an arbitrary stage using the provided targets (typically
// collected from the previous stage's output by the Aggregator).
func SplitStage(task *models.Task, stage string, targets []string) []*models.Subtask {
	return splitTargets(task, stage, targets)
}

func splitTargets(task *models.Task, stage string, targets []string) []*models.Subtask {
	size := chunkSizeFor(stage)
	now := time.Now()
	var subtasks []*models.Subtask

	for i := 0; i < len(targets); i += size {
		end := i + size
		if end > len(targets) {
			end = len(targets)
		}
		chunk := targets[i:end]

		st := &models.Subtask{
			ID:         uuid.NewString(),
			TaskID:     task.ID,
			RunID:      task.RunID,
			Stage:      stage,
			Capability: stage,
			Targets:    chunk,
			Params:     task.Config.Params,
			Blacklist:  task.Blacklist,
			Attempt:    0,
			Status:     models.SubtaskPending,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		subtasks = append(subtasks, st)
	}
	return subtasks
}
