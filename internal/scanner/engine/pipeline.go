package engine

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/yourname/nscan/pkg/proto/scanv1"
	"go.uber.org/zap"
)

// EngineStats 引擎运行时统计，供心跳上报
type EngineStats struct {
	ActiveTasks int32
	CPUPercent  int32
	MemPercent  int32
	Pools       map[string]PoolStats
}

// PipelineEngine 管理所有内置 Stage 和插件，执行扫描任务
type PipelineEngine struct {
	stages      map[string]Stage // 已注册的 stage（内置 + 插件）
	activeTasks int32            // 原子计数
	ctxmgr      *ContextManager  // 全局上下文管理器
	localDedup  *LocalDedup      // 本地 BigCache 去重层
	poolMgr     *PoolManager     // 模块级协程池管理
	recovery    *TaskRecovery    // 任务崩溃恢复
	log         *zap.Logger
}

func NewPipelineEngine(log *zap.Logger) *PipelineEngine {
	dedup, err := NewLocalDedup(log)
	if err != nil {
		log.Warn("failed to init local dedup, continuing without it", zap.Error(err))
	}
	return &PipelineEngine{
		stages:     make(map[string]Stage),
		ctxmgr:     NewContextManager(),
		localDedup: dedup,
		poolMgr:    NewPoolManager(log),
		log:        log,
	}
}

// InitRecovery sets up PebbleDB-based task crash recovery.
func (e *PipelineEngine) InitRecovery(dataDir string) error {
	r, err := NewTaskRecovery(dataDir, e.log)
	if err != nil {
		return err
	}
	e.recovery = r
	return nil
}

// RecoverTasks loads persisted tasks from PebbleDB on startup.
func (e *PipelineEngine) RecoverTasks() ([]*scanv1.ScanTask, error) {
	if e.recovery == nil {
		return nil, nil
	}
	return e.recovery.RecoverTasks()
}

// PoolManager returns the global pool manager for stage use.
func (e *PipelineEngine) PoolManager() *PoolManager { return e.poolMgr }

// Shutdown releases all resources.
func (e *PipelineEngine) Shutdown() {
	if e.poolMgr != nil {
		e.poolMgr.Release()
	}
	if e.recovery != nil {
		e.recovery.Close()
	}
}

// Register 注册一个 Stage（内置或插件调用此方法）
func (e *PipelineEngine) Register(s Stage) {
	e.stages[s.Name()] = s
	e.log.Info("stage registered", zap.String("name", s.Name()))
}

// Run 按 task.Config.Stages 顺序执行各阶段，结果和进度写入对应 channel
// 调用方负责关闭 ctx 以取消任务
func (e *PipelineEngine) Run(
	ctx context.Context,
	task *scanv1.ScanTask,
	results chan<- *ScanResult,
	progress chan<- *Progress,
) error {
	taskCtx, cancel := e.ctxmgr.Register(ctx, task.TaskId)
	defer func() {
		cancel()
		atomic.AddInt32(&e.activeTasks, -1)
		if e.recovery != nil {
			_ = e.recovery.RemoveTask(task.TaskId)
		}
	}()
	atomic.AddInt32(&e.activeTasks, 1)

	// Persist task for crash recovery
	if e.recovery != nil {
		if err := e.recovery.SaveTask(task); err != nil {
			e.log.Warn("failed to persist task for recovery", zap.Error(err))
		}
	}
	if len(task.Config.GetStages()) == 0 {
		close(results)
		close(progress)
		return fmt.Errorf("task %s has no stages configured", task.TaskId)
	}

	// Local dedup: proxy channel filters duplicates before forwarding
	var dedupDone chan struct{}
	actualResults := results
	if e.localDedup != nil {
		proxy := make(chan *ScanResult, cap(results))
		dedupDone = make(chan struct{})
		var dedupDropped int64
		go func() {
			defer close(dedupDone)
			for r := range proxy {
				key := ResultKey(r)
				if e.localDedup.IsSeen("result", key) {
					dedupDropped++
					continue
				}
				select {
				case actualResults <- r:
				case <-taskCtx.Done():
					return
				}
			}
			if dedupDropped > 0 {
				e.log.Info("local dedup stats",
					zap.String("task_id", task.TaskId),
					zap.Int64("dropped", dedupDropped),
				)
			}
		}()
		results = proxy
	}

	// Passive scanner sidecar
	passive := NewPassiveScanner(e.log, 1000)
	for _, rule := range DefaultPassiveRules() {
		passive.AddRule(rule)
	}
	passive.Start(taskCtx)

	// Forward passive findings to the results channel
	passiveDone := make(chan struct{})
	go func() {
		defer close(passiveDone)
		for r := range passive.Results() {
			select {
			case results <- r:
			case <-taskCtx.Done():
				return
			}
		}
	}()

	stages := task.Config.GetStages()
	var pipelineErr error

	params := task.Config.GetParams()
	targets := task.Targets
	if runtime.GOOS == "linux" && isRunningInDocker() {
		for i, t := range targets {
			t = strings.ReplaceAll(t, "://127.0.0.1", "://host.docker.internal")
			t = strings.ReplaceAll(t, "://localhost", "://host.docker.internal")
			targets[i] = t
		}
	}
	input := &StageInput{Targets: targets}
	blacklistChecker := NewBlacklistChecker(task.Blacklist)

	// Filter initial targets
	var skippedCount int
	input = blacklistChecker.FilterInput(input, func(skipped string) {
		SendLog(progress, "_pipeline", "info", fmt.Sprintf("跳过黑名单目标: %s", skipped))
		skippedCount++
	})

	e.log.Info("pipeline start",
		zap.String("task_id", task.TaskId),
		zap.Strings("stages", stages),
		zap.Strings("targets", input.Targets),
		zap.Int("skipped", skippedCount),
	)

	SendLog(progress, "_pipeline", "info", fmt.Sprintf("任务开始, 目标: %d 个, 跳过: %d 个, 阶段: %v", len(input.Targets), skippedCount, stages))

	// resultFanout wraps result emission to also feed passive scanner
	resultFanout := func(r *ScanResult) {
		select {
		case results <- r:
		case <-taskCtx.Done():
			return
		}
		if r.Type == "http" {
			passive.Feed(r)
		}
	}
	// Wrap results channel for stages — use a proxy that feeds passive scanner
	stageCh := make(chan *ScanResult, 100)
	stageFanoutDone := make(chan struct{})
	go func() {
		defer close(stageFanoutDone)
		for r := range stageCh {
			resultFanout(r)
		}
	}()

	for i, name := range stages {
		if taskCtx.Err() != nil {
			break
		}

		stage, ok := e.stages[name]
		if !ok {
			e.log.Warn("stage not found, skipping", zap.String("stage", name))
			SendLog(progress, name, "warn", fmt.Sprintf("[%s] 阶段未注册, 跳过", name))
			pipelineErr = fmt.Errorf("stage %q is not registered", name)
			break
		}

		stageParams := filterParams(params, name+".")

		e.log.Info("stage start",
			zap.String("task_id", task.TaskId),
			zap.String("stage", name),
			zap.Int("step", i+1),
			zap.Int("total", len(stages)),
		)

		inputCount := len(input.Targets) + len(input.Subdomains) + len(input.Hosts) + len(input.HTTPURLs)
		sendProgress(progress, name, 0, "started")
		SendLog(progress, name, "info", fmt.Sprintf("[%s] 阶段开始 (%d/%d), 输入 %d 个目标", name, i+1, len(stages), inputCount))

		startTime := time.Now()
		out, err := stage.Run(taskCtx, input, stageParams, stageCh, progress)
		elapsed := time.Since(startTime)

		if err != nil {
			pipelineErr = err
			e.log.Error("stage failed",
				zap.String("task_id", task.TaskId),
				zap.String("stage", name),
				zap.Error(err),
			)
			SendLog(progress, name, "error", fmt.Sprintf("[%s] 阶段失败 (耗时 %s): %v", name, elapsed.Round(time.Millisecond), err))
			sendProgress(progress, name, 100, fmt.Sprintf("failed: %v", err))
			break
		}

		outCount := 0
		if out != nil {
			outCount = len(out.Subdomains) + len(out.Hosts) + len(out.HTTPURLs)
		}
		if outCount > 0 {
			SendLog(progress, name, "info", fmt.Sprintf("[%s] 阶段完成, 发现 %d 个资产, 耗时 %s", name, outCount, elapsed.Round(time.Millisecond)))
		} else {
			SendLog(progress, name, "info", fmt.Sprintf("[%s] 阶段完成, 耗时 %s", name, elapsed.Round(time.Millisecond)))
		}
		sendProgress(progress, name, 100, "done")

		if out != nil {
			out = blacklistChecker.FilterInput(out, func(skipped string) {
				SendLog(progress, name, "info", fmt.Sprintf("[%s] 跳过黑名单资产: %s", name, skipped))
			})
			input = mergeInput(input, out)
		}
	}

	totalAssets := len(input.Subdomains) + len(input.Hosts) + len(input.HTTPURLs)
	if pipelineErr == nil {
		SendLog(progress, "_pipeline", "info", fmt.Sprintf("任务完成, 累计发现 %d 个资产", totalAssets))
	} else {
		SendLog(progress, "_pipeline", "error", fmt.Sprintf("任务失败, 累计发现 %d 个资产", totalAssets))
	}

	// Shutdown chain: stage fanout → passive → dedup → actual results
	close(stageCh)
	<-stageFanoutDone
	passive.Close()
	<-passiveDone

	if dedupDone != nil {
		close(results) // close proxy
		<-dedupDone
		close(actualResults)
		if e.localDedup != nil {
			e.localDedup.Reset()
		}
	} else {
		close(results)
	}
	close(progress)

	e.log.Info("pipeline done", zap.String("task_id", task.TaskId))
	return pipelineErr
}

// RunSingleStage executes a single named stage for the given targets and params.
// Used by the SubtaskWorker in Phase-3 queue mode.
func (e *PipelineEngine) RunSingleStage(
	ctx context.Context,
	taskID string,
	stageName string,
	targets []string,
	params map[string]string,
	blacklist []*scanv1.BlacklistRule,
	results chan<- *ScanResult,
	progress chan<- *Progress,
) error {
	// Queue mode can run many subtasks of the same task concurrently. Do not
	// register them under the shared task ID: ContextManager is intentionally
	// one-cancel-per-key and would overwrite sibling contexts.
	taskCtx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		atomic.AddInt32(&e.activeTasks, -1)
	}()
	atomic.AddInt32(&e.activeTasks, 1)

	stage, ok := e.stages[stageName]
	if !ok {
		return fmt.Errorf("stage %q not registered", stageName)
	}

	checker := NewBlacklistChecker(blacklist)
	input := &StageInput{Targets: targets}
	input = checker.FilterInput(input, func(skipped string) {
		SendLog(progress, stageName, "info", fmt.Sprintf("跳过黑名单目标: %s", skipped))
	})

	stageParams := filterParams(params, stageName+".")

	_, err := stage.Run(taskCtx, input, stageParams, results, progress)
	return err
}

// Cancel 取消指定任务
func (e *PipelineEngine) Cancel(taskID string) {
	e.ctxmgr.Cancel(taskID)
}

// Stats 返回当前引擎统计
func (e *PipelineEngine) Stats() EngineStats {
	cpuPct := int32(0)
	if pcts, err := cpu.Percent(100*time.Millisecond, false); err == nil && len(pcts) > 0 {
		cpuPct = int32(pcts[0])
	}
	memPct := int32(0)
	if v, err := mem.VirtualMemory(); err == nil {
		memPct = int32(v.UsedPercent)
	}
	var pools map[string]PoolStats
	if e.poolMgr != nil {
		pools = e.poolMgr.Stats()
	}
	return EngineStats{
		ActiveTasks: atomic.LoadInt32(&e.activeTasks),
		CPUPercent:  cpuPct,
		MemPercent:  memPct,
		Pools:       pools,
	}
}

// ── 辅助函数 ──────────────────────────────────────────────────────────────────

func filterParams(params map[string]string, prefix string) map[string]string {
	out := make(map[string]string)
	plen := len(prefix)
	for k, v := range params {
		if len(k) > plen && k[:plen] == prefix {
			out[k[plen:]] = v
		}
	}
	return out
}

func mergeInput(base, next *StageInput) *StageInput {
	merged := &StageInput{
		Targets:      base.Targets,
		Subdomains:   append(base.Subdomains, next.Subdomains...),
		Hosts:        append(base.Hosts, next.Hosts...),
		HTTPURLs:     append(base.HTTPURLs, next.HTTPURLs...),
		CrawledPages: append(base.CrawledPages, next.CrawledPages...),
	}
	if len(base.HTTPTechMap) > 0 || len(next.HTTPTechMap) > 0 {
		merged.HTTPTechMap = make(map[string][]string, len(base.HTTPTechMap)+len(next.HTTPTechMap))
		for k, v := range base.HTTPTechMap {
			merged.HTTPTechMap[k] = v
		}
		for k, v := range next.HTTPTechMap {
			merged.HTTPTechMap[k] = v
		}
	}
	return merged
}

func sendProgress(ch chan<- *Progress, stage string, pct int32, msg string) {
	select {
	case ch <- &Progress{Stage: stage, Percent: pct, Message: msg}:
	default:
	}
}

func SendLog(ch chan<- *Progress, stage, level, msg string) {
	select {
	case ch <- &Progress{Stage: stage, Log: msg, Level: level}:
	default:
	}
}

func isRunningInDocker() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}
