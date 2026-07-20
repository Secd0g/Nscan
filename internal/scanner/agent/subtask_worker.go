package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yourname/nscan/internal/scanner/engine"
	"github.com/yourname/nscan/internal/server/queue"
	"github.com/yourname/nscan/pkg/models"
	"github.com/yourname/nscan/pkg/proto/scanv1"
	"go.uber.org/zap"
)

const (
	subtaskLeaseTTL = 60 * time.Second
	renewInterval   = 15 * time.Second
	blpopTimeout    = 5 * time.Second
	defaultSubtaskTimeout = 45 * time.Minute
	maxSubtaskOutputBytes = 16 << 20
)

// SubtaskWorkerPool manages a pool of goroutines that BLPop from Redis queues
// and execute single-stage subtasks, reporting results back via the gRPC stream.
type SubtaskWorkerPool struct {
	q          *queue.Queue
	eng        *engine.PipelineEngine
	nodeID     string
	capability string
	numWorkers int
	send       func(*scanv1.ScannerMessage) error
	log        *zap.Logger
}

// NewSubtaskWorkerPool creates a pool for a single capability queue.
func NewSubtaskWorkerPool(
	rdb *redis.Client,
	eng *engine.PipelineEngine,
	nodeID, capability string,
	numWorkers int,
	send func(*scanv1.ScannerMessage) error,
	log *zap.Logger,
) *SubtaskWorkerPool {
	return &SubtaskWorkerPool{
		q:          queue.New(rdb),
		eng:        eng,
		nodeID:     nodeID,
		capability: capability,
		numWorkers: numWorkers,
		send:       send,
		log:        log,
	}
}

// Run starts the worker goroutines and blocks until ctx is cancelled.
func (p *SubtaskWorkerPool) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < p.numWorkers; i++ {
		wg.Add(1)
		go func(workerIdx int) {
			defer wg.Done()
			p.workerLoop(ctx, workerIdx)
		}(i)
	}
	wg.Wait()
}

func (p *SubtaskWorkerPool) workerLoop(ctx context.Context, idx int) {
	p.log.Info("subtask worker started",
		zap.String("capability", p.capability),
		zap.Int("worker", idx),
	)
	for {
		if ctx.Err() != nil {
			return
		}
		st, err := p.q.BLPop(ctx, p.capability, blpopTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			p.log.Error("BLPop error", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}
		if st == nil {
			continue // timeout, loop again
		}
		cancelled, err := p.q.IsTaskCancelled(ctx, st.TaskID.Hex())
		if err == nil && cancelled {
			_ = p.q.DropPending(ctx, st.TaskID.Hex(), st.ID)
			continue
		}
		currentRun, err := p.q.TaskRunID(ctx, st.TaskID.Hex())
		if err == nil && currentRun != "" && st.RunID != currentRun {
			_ = p.q.DropPending(ctx, st.TaskID.Hex(), st.ID)
			continue
		}
		p.executeSubtask(ctx, st)
	}
}

func (p *SubtaskWorkerPool) executeSubtask(ctx context.Context, st *models.Subtask) {
	cancelled, err := p.q.IsTaskCancelled(ctx, st.TaskID.Hex())
	if err == nil && cancelled {
		_ = p.q.DropPending(ctx, st.TaskID.Hex(), st.ID)
		return
	}
	currentRun, err := p.q.TaskRunID(ctx, st.TaskID.Hex())
	if err == nil && currentRun != "" && st.RunID != currentRun {
		_ = p.q.DropPending(ctx, st.TaskID.Hex(), st.ID)
		return
	}
	// Acquire lease.
	ok, err := p.q.Lease(ctx, st.ID, p.nodeID, subtaskLeaseTTL)
	if err != nil || !ok {
		p.log.Warn("lease acquisition failed, dropping subtask",
			zap.String("subtask_id", st.ID),
		)
		return
	}

	p.log.Info("executing subtask",
		zap.String("subtask_id", st.ID),
		zap.String("task_id", st.TaskID.Hex()),
		zap.String("stage", st.Stage),
		zap.Int("targets", len(st.Targets)),
	)

	// Spawn lease renewal goroutine.
	leaseCtx, cancelLease := context.WithCancel(ctx)
	go p.renewLease(leaseCtx, st.ID)

	results := make(chan *engine.ScanResult, 256)
	progress := make(chan *engine.Progress, 256)

	// Forward progress to gRPC stream.
	var progressWg sync.WaitGroup
	progressWg.Add(1)
	go func() {
		defer progressWg.Done()
		p.forwardProgress(st, progress)
	}()

	// Collect results, preserving the result type alongside each data blob.
	type typedResult struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	var collected []json.RawMessage
	var collectedBytes int
	var outputTruncated bool
	var collectWg sync.WaitGroup
	collectWg.Add(1)
	go func() {
		defer collectWg.Done()
		for r := range results {
			if b, err := json.Marshal(typedResult{Type: r.Type, Data: r.Data}); err == nil {
				if collectedBytes+len(b) > maxSubtaskOutputBytes {
					outputTruncated = true
					continue
				}
				collectedBytes += len(b)
				collected = append(collected, b)
			}
		}
	}()

	stageCtx, cancelStage := context.WithTimeout(ctx, subtaskTimeout(st.Params))
	runErr := p.eng.RunSingleStage(stageCtx, st.TaskID.Hex(), st.Stage, st.Targets, st.Params, st.Blacklist, results, progress)
	cancelStage()
	if runErr == nil {
		// RunSingleStage intentionally executes only one stage and therefore
		// does not emit the pipeline's normal 100% boundary event. Emit it here
		// so task details can mark queue-mode stages complete after their logs.
		select {
		case progress <- &engine.Progress{Stage: st.Stage, Percent: 100, Message: "done"}:
		case <-ctx.Done():
		}
	}
	close(results)
	close(progress)
	collectWg.Wait()
	progressWg.Wait()
	cancelLease()

	// Build completion message.
	complete := &scanv1.SubtaskComplete{
		SubtaskId: st.ID,
		TaskId:    st.TaskID.Hex(),
		Stage:     st.Stage,
		NodeId:    p.nodeID,
		Success:   runErr == nil,
	}
	if runErr != nil {
		complete.ErrorMsg = runErr.Error()
	} else if outputTruncated {
		complete.Success = false
		complete.ErrorMsg = fmt.Sprintf("subtask output exceeded %d bytes", maxSubtaskOutputBytes)
	} else {
		output, _ := json.Marshal(collected)
		complete.Output = output
	}

	// Report to server via gRPC stream.
	msg := &scanv1.ScannerMessage{
		Payload: &scanv1.ScannerMessage_SubtaskComplete{SubtaskComplete: complete},
	}
	if err := p.send(msg); err != nil {
		p.log.Error("send SubtaskComplete failed", zap.String("subtask_id", st.ID), zap.Error(err))
	}

	p.log.Info("subtask complete",
		zap.String("subtask_id", st.ID),
		zap.String("stage", st.Stage),
		zap.Bool("success", complete.Success),
		zap.Int("results", len(collected)),
	)
}

func subtaskTimeout(params map[string]string) time.Duration {
	for _, key := range []string{"subtask.timeout", "task.timeout"} {
		if raw := params[key]; raw != "" {
			if d, err := time.ParseDuration(raw); err == nil && d > 0 {
				return d
			}
			if seconds, err := time.ParseDuration(raw + "s"); err == nil && seconds > 0 {
				return seconds
			}
		}
	}
	return defaultSubtaskTimeout
}

func (p *SubtaskWorkerPool) renewLease(ctx context.Context, subtaskID string) {
	ticker := time.NewTicker(renewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ok, err := p.q.Renew(ctx, subtaskID, p.nodeID, subtaskLeaseTTL)
			if err != nil || !ok {
				p.log.Warn("lease renewal failed",
					zap.String("subtask_id", subtaskID),
					zap.Bool("ok", ok),
					zap.Error(err),
				)
			}
		}
	}
}

func (p *SubtaskWorkerPool) forwardProgress(st *models.Subtask, progress <-chan *engine.Progress) {
	for prog := range progress {
		msg := &scanv1.ScannerMessage{
			Payload: &scanv1.ScannerMessage_SubtaskProgress{
				SubtaskProgress: &scanv1.SubtaskProgress{
					SubtaskId: st.ID,
					TaskId:    st.TaskID.Hex(),
					Stage:     prog.Stage,
					Percent:   prog.Percent,
					Message:   prog.Message,
					Log:       prog.Log,
					Level:     prog.Level,
					NodeId:    p.nodeID,
				},
			},
		}
		if err := p.send(msg); err != nil {
			p.log.Warn("send SubtaskProgress failed", zap.Error(err))
			// Drain remaining progress to unblock RunSingleStage.
			go func() {
				for range progress {
				}
			}()
			return
		}
	}
}

// StartSubtaskWorkers starts one SubtaskWorkerPool per capability from the
// scanner config, if queue.redis_addr is set and num_workers > 0.
// Returns a cancel function to stop all pools.
func StartSubtaskWorkers(
	ctx context.Context,
	rdbAddr, rdbPass string,
	eng *engine.PipelineEngine,
	nodeID string,
	capabilities []string,
	numWorkers int,
	send func(*scanv1.ScannerMessage) error,
	log *zap.Logger,
) (cancel func()) {
	if rdbAddr == "" || numWorkers <= 0 {
		return func() {}
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     rdbAddr,
		Password: rdbPass,
	})

	workerCtx, cancelFn := context.WithCancel(ctx)
	var wg sync.WaitGroup

	for _, cap := range capabilities {
		cap := cap
		pool := NewSubtaskWorkerPool(rdb, eng, nodeID, cap, numWorkers, send, log)
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.Run(workerCtx)
		}()
	}

	log.Info("subtask workers started",
		zap.Strings("capabilities", capabilities),
		zap.Int("workers_per_cap", numWorkers),
	)

	return func() {
		cancelFn()
		wg.Wait()
		rdb.Close()
	}
}

func shortSubtaskID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func init() {
	_ = fmt.Sprintf // suppress unused import
}
