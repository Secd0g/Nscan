package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yourname/nscan/internal/server/ai"
	"github.com/yourname/nscan/internal/server/queue"
	"github.com/yourname/nscan/internal/server/repositories"
	"github.com/yourname/nscan/pkg/models"
	"github.com/yourname/nscan/pkg/proto/scanv1"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// OnSubtaskComplete is called by the gRPC handler when a SubtaskComplete
// message arrives from a scanner node. It:
//  1. Stores the output in the stage aggregate list
//  2. Completes the lease
//  3. Checks if all subtasks for the current stage are done
//  4. If so, triggers the next stage (or marks the task done)
func (s *Scheduler) OnSubtaskComplete(ctx context.Context, msg *scanv1.SubtaskComplete) {
	taskID := msg.TaskId
	subtaskID := msg.SubtaskId
	stage := msg.Stage
	nodeID := msg.NodeId
	// A completion can arrive after cancellation or deletion because it was
	// already in flight. Never let such a message advance or resurrect a task.
	taskOID, err := primitive.ObjectIDFromHex(taskID)
	if err != nil {
		return
	}
	var current models.Task
	if err := s.db.Collection("tasks").FindOne(ctx, bson.M{"_id": taskOID}).Decode(&current); err != nil {
		return
	}
	if current.Status == models.TaskStatusFailed || current.Status == models.TaskStatusDone {
		return
	}

	if !msg.Success {
		// Keep failed work in the pending set while it is retried. Removing it
		// here would make the aggregator treat a partial stage as complete.
		ok, err := s.q.Release(ctx, subtaskID, nodeID)
		if err != nil || !ok {
			s.log.Warn("release failed subtask lease failed", zap.String("subtask_id", subtaskID), zap.Error(err))
			return
		}
		st, err := s.q.GetSubtask(ctx, subtaskID)
		if err != nil {
			s.log.Error("load failed subtask metadata failed", zap.String("subtask_id", subtaskID), zap.Error(err))
			return
		}
		if st.Attempt+1 >= queue.MaxAttempts {
			reason := fmt.Sprintf("subtask failed after %d attempts: %s", st.Attempt+1, msg.ErrorMsg)
			if err := s.q.DeadLetter(ctx, taskID, st, reason); err != nil {
				return
			}
			s.markTaskFailed(ctx, taskID, reason)
			return
		}
		if err := s.q.Requeue(ctx, st); err != nil {
			s.log.Error("requeue failed subtask failed", zap.String("subtask_id", subtaskID), zap.Error(err))
		}
		return
	}

	// Only the node holding the lease may commit the result. This also makes
	// duplicate completion messages harmless after a lease is reclaimed.
	ok, err := s.q.Complete(ctx, taskID, subtaskID, nodeID)
	if err != nil {
		s.log.Error("complete subtask lease failed", zap.String("subtask_id", subtaskID), zap.Error(err))
		return
	}
	if !ok {
		s.log.Warn("complete subtask: lease not held by this node",
			zap.String("subtask_id", subtaskID),
			zap.String("node_id", nodeID),
		)
		return
	}

	if len(msg.Output) > 0 {
		if err := s.q.AppendOutput(ctx, taskID, stage, msg.Output); err != nil {
			s.log.Error("append subtask output failed",
				zap.String("subtask_id", subtaskID),
				zap.Error(err),
			)
			s.markTaskFailed(ctx, taskID, "failed to persist subtask output: "+err.Error())
			return
		}
	}

	// Check if all subtasks for this task+stage are done.
	pending, err := s.q.PendingMembers(ctx, taskID)
	if err != nil {
		s.log.Error("check pending subtasks failed", zap.String("task_id", taskID), zap.Error(err))
		return
	}

	// Filter pending to only those belonging to the current stage by checking
	// each subtask's stage prefix in its ID. We encode stage in the subtask ID
	// as UUID (no prefix), so we track per-stage completion via a stage-scoped
	// pending set key instead. For now, use the simpler approach: if ALL pending
	// subtasks are zero, the task's current stage is complete.
	//
	// NOTE: This works correctly because subtasks are enqueued stage-by-stage;
	// the Aggregator only enqueues the next stage after the current one fully drains.
	if len(pending) > 0 {
		s.log.Debug("stage not yet complete",
			zap.String("task_id", taskID),
			zap.String("stage", stage),
			zap.Int("remaining", len(pending)),
		)
		return
	}

	// All subtasks for this stage done → determine next action.
	s.log.Info("stage complete, advancing task",
		zap.String("task_id", taskID),
		zap.String("stage", stage),
	)
	s.advanceTask(ctx, taskID, stage)
}

// advanceTask is called when all subtasks for currentStage are done.
// It collects the stage's output, merges asset results into MongoDB, then
// either enqueues the next stage or marks the task done.
func (s *Scheduler) advanceTask(ctx context.Context, taskIDStr, completedStage string) {
	lockKey := fmt.Sprintf("task:%s:advance:%s", taskIDStr, completedStage)
	lockValue := fmt.Sprintf("%d", time.Now().UnixNano())
	locked, err := s.rdb.SetNX(ctx, lockKey, lockValue, 10*time.Minute).Result()
	if err != nil || !locked {
		return
	}
	defer func() {
		const release = `if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) else return 0 end`
		_, _ = s.rdb.Eval(context.Background(), release, []string{lockKey}, lockValue).Result()
	}()

	taskOID, err := primitive.ObjectIDFromHex(taskIDStr)
	if err != nil {
		s.log.Error("advanceTask: invalid task_id", zap.String("task_id", taskIDStr))
		return
	}

	// Load task to get full config.
	var task models.Task
	if err := s.db.Collection("tasks").FindOne(ctx, bson.M{"_id": taskOID}).Decode(&task); err != nil {
		s.log.Error("advanceTask: load task failed", zap.String("task_id", taskIDStr), zap.Error(err))
		return
	}

	// Collect and persist output from completed stage.
	chunks, err := s.q.CollectOutput(ctx, taskIDStr, completedStage)
	if err != nil {
		s.log.Error("advanceTask: collect output failed", zap.Error(err))
	}
	nextTargets := s.persistStageOutput(ctx, &task, completedStage, chunks)

	// Find the next stage index.
	nextStage, found := nextStageAfter(task.Config.Stages, completedStage)
	if !found {
		// Task fully complete.
		s.markTaskDone(ctx, taskOID)
		return
	}

	if len(nextTargets) == 0 {
		s.log.Info("no targets for next stage, task complete",
			zap.String("task_id", taskIDStr),
			zap.String("next_stage", nextStage),
		)
		s.markTaskDone(ctx, taskOID)
		return
	}

	// Deduplicate targets before handing to next stage.
	dedupedTargets := s.dedup.FilterNew(ctx, taskIDStr, nextStage, nextTargets)
	if len(dedupedTargets) == 0 {
		s.log.Info("all targets already seen, task complete",
			zap.String("task_id", taskIDStr),
			zap.String("next_stage", nextStage),
		)
		s.markTaskDone(ctx, taskOID)
		return
	}
	s.log.Info("dedup stage targets",
		zap.String("task_id", taskIDStr),
		zap.String("next_stage", nextStage),
		zap.Int("before", len(nextTargets)),
		zap.Int("after", len(dedupedTargets)),
	)

	// Enqueue subtasks for the next stage.
	subtasks := SplitStage(&task, nextStage, dedupedTargets)
	for _, st := range subtasks {
		if err := StoreSubtaskMeta(ctx, s.rdb, st); err != nil {
			s.log.Warn("store next-stage subtask meta failed", zap.String("subtask_id", st.ID), zap.Error(err))
		}
		if err := s.q.Enqueue(ctx, st); err != nil {
			s.log.Error("enqueue next-stage subtask failed",
				zap.String("task_id", taskIDStr),
				zap.String("stage", nextStage),
				zap.Error(err),
			)
		}
	}
	s.log.Info("next stage enqueued",
		zap.String("task_id", taskIDStr),
		zap.String("stage", nextStage),
		zap.Int("subtasks", len(subtasks)),
		zap.Int("targets", len(nextTargets)),
	)
}

// nextStageAfter returns the stage following current in the ordered list.
func nextStageAfter(stages []string, current string) (string, bool) {
	for i, s := range stages {
		if s == current && i+1 < len(stages) {
			return stages[i+1], true
		}
	}
	return "", false
}

func (s *Scheduler) markTaskDone(ctx context.Context, taskID primitive.ObjectID) {
	now := time.Now()
	res, err := s.db.Collection("tasks").UpdateOne(ctx, bson.M{
		"_id": taskID,
		"status": bson.M{"$in": []models.TaskStatus{
			models.TaskStatusDispatched,
			models.TaskStatusRunning,
		}},
	}, bson.M{"$set": bson.M{
		"status":     models.TaskStatusDone,
		"done_at":    now,
		"updated_at": now,
	}})
	if err != nil || res.ModifiedCount == 0 {
		return
	}
	s.q.ClearTask(ctx, taskID.Hex()) //nolint
	s.dedup.Clear(ctx, taskID.Hex())
	s.notifyTaskEvent(taskID, "done", "")
	s.log.Info("task done (queue mode)", zap.String("task_id", taskID.Hex()))
	s.maybeAnalyzeTask(taskID)
}

func (s *Scheduler) maybeAnalyzeTask(taskID primitive.ObjectID) {
	var task models.Task
	if err := s.db.Collection("tasks").FindOne(context.Background(), bson.M{"_id": taskID}).Decode(&task); err != nil || !task.AIAnalysisEnabled || s.settings == nil {
		return
	}
	_, _ = s.db.Collection("tasks").UpdateByID(context.Background(), taskID, bson.M{"$set": bson.M{"ai_analysis_status": "running"}})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		var cfg ai.Config
		raw, err := s.settings.GetValue(ctx, "ai")
		if err == nil {
			err = json.Unmarshal([]byte(raw), &cfg)
		}
		result := ""
		if err == nil {
			result, err = ai.Analyze(ctx, cfg, &task, repositories.NewAssetRepo(s.db))
		}
		update := bson.M{"ai_analysis_status": "done", "ai_analysis": result, "ai_analysis_error": ""}
		if err != nil {
			update = bson.M{"ai_analysis_status": "failed", "ai_analysis_error": err.Error()}
		}
		_, _ = s.db.Collection("tasks").UpdateByID(context.Background(), taskID, bson.M{"$set": update})
	}()
}

// persistStageOutput decodes each JSON chunk (list of asset objects), upserts
// them into MongoDB, and returns the targets (hostnames/IPs) for the next stage.
func (s *Scheduler) persistStageOutput(ctx context.Context, task *models.Task, stage string, chunks [][]byte) []string {
	if len(chunks) == 0 {
		return nil
	}

	// Each chunk is a JSON array of {type, data} envelopes produced by the scanner.
	// Fall back to using the stage name as ResultType for legacy plain-data entries.
	targetSet := map[string]struct{}{}
	for _, chunk := range chunks {
		var items []json.RawMessage
		if err := json.Unmarshal(chunk, &items); err != nil {
			items = []json.RawMessage{chunk}
		}
		for _, item := range items {
			// Try to unwrap the typed envelope written by subtask_worker.
			var envelope struct {
				Type string          `json:"type"`
				Data json.RawMessage `json:"data"`
			}
			resultType := stage
			resultData := item
			if err := json.Unmarshal(item, &envelope); err == nil && envelope.Type != "" && len(envelope.Data) > 0 {
				resultType = envelope.Type
				resultData = envelope.Data
			}
			result := &scanv1.TaskResult{
				TaskId:     task.ID.Hex(),
				NodeId:     "aggregator",
				ResultType: resultType,
				Data:       resultData,
			}
			s.OnResult(result)

			// Extract targets for next stage from subdomain/http results.
			var obj map[string]interface{}
			if err := json.Unmarshal(resultData, &obj); err == nil {
				if u, ok := obj["url"].(string); ok && u != "" {
					targetSet[u] = struct{}{}
				}
				if h, ok := obj["host"].(string); ok && h != "" {
					targetSet[h] = struct{}{}
				}
				if d, ok := obj["domain"].(string); ok && d != "" {
					targetSet[d] = struct{}{}
				}
				if ip, ok := obj["ip"].(string); ok && ip != "" {
					targetSet[ip] = struct{}{}
				}
			}
		}
	}

	targets := make([]string, 0, len(targetSet))
	for t := range targetSet {
		targets = append(targets, t)
	}

	if len(targets) == 0 {
		// Fallback: use original task targets so the pipeline doesn't stall.
		s.log.Warn("persistStageOutput: no targets extracted, falling back to task targets",
			zap.String("stage", fmt.Sprintf("%s→next", stage)),
		)
		return task.Targets
	}
	return targets
}
