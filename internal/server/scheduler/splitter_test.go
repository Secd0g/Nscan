package scheduler

import (
	"testing"

	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func makeTask(stages []string, targets []string) *models.Task {
	return &models.Task{
		ID:        primitive.NewObjectID(),
		ProjectID: primitive.NewObjectID(),
		Config:    models.TaskConfig{Stages: stages, Params: map[string]string{}},
		Targets:   targets,
	}
}

func targets(n int) []string {
	ts := make([]string, n)
	for i := range ts {
		ts[i] = "t" + string(rune('0'+i%10))
	}
	return ts
}

func TestSplitFirstStage_basic(t *testing.T) {
	task := makeTask([]string{"port"}, targets(500))
	subs, err := SplitFirstStage(task)
	if err != nil {
		t.Fatal(err)
	}
	// chunk size for port = 200 → ceil(500/200) = 3
	if len(subs) != 3 {
		t.Errorf("expected 3 subtasks, got %d", len(subs))
	}
	total := 0
	for _, s := range subs {
		total += len(s.Targets)
		if s.Stage != "port" {
			t.Errorf("stage mismatch: %q", s.Stage)
		}
		if s.TaskID != task.ID {
			t.Error("task_id mismatch")
		}
	}
	if total != 500 {
		t.Errorf("total targets should be 500, got %d", total)
	}
}

func TestSplitFirstStage_noStages(t *testing.T) {
	task := makeTask([]string{}, targets(10))
	_, err := SplitFirstStage(task)
	if err == nil {
		t.Fatal("expected error for task with no stages")
	}
}

func TestSplitFirstStage_singleTarget(t *testing.T) {
	task := makeTask([]string{"bbot"}, []string{"example.com"})
	subs, _ := SplitFirstStage(task)
	if len(subs) != 1 {
		t.Errorf("expected 1 subtask, got %d", len(subs))
	}
}

func TestSplitStage_unknownStage(t *testing.T) {
	task := makeTask([]string{"custom"}, nil)
	subs := SplitStage(task, "custom", targets(300))
	// fallback chunk=100 → ceil(300/100)=3
	if len(subs) != 3 {
		t.Errorf("expected 3 subtasks for unknown stage, got %d", len(subs))
	}
}
