// Package cronjob 周期性检查定时扫描任务，到点时创建实际扫描任务。
package cronjob

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yourname/nscan/internal/server/cron"
	"github.com/yourname/nscan/internal/server/repositories"
	"github.com/yourname/nscan/internal/server/scheduler"
	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

// Runner 驱动定时扫描：每分钟轮询到期任务并提交扫描。
type Runner struct {
	repo  *repositories.ScheduledRepo
	sched *scheduler.Scheduler
	log   *zap.Logger
}

func New(repo *repositories.ScheduledRepo, sched *scheduler.Scheduler, log *zap.Logger) *Runner {
	return &Runner{repo: repo, sched: sched, log: log}
}

// Run 启动轮询循环，对齐到整分钟边界后每分钟检查一次。
func (r *Runner) Run(ctx context.Context) {
	// 对齐到下一个整分钟，避免同一分钟内因秒数不同重复/漏触发。
	next := time.Now().Truncate(time.Minute).Add(time.Minute)
	timer := time.NewTimer(time.Until(next))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			r.tick(ctx, time.Now())
			next = next.Add(time.Minute)
			timer.Reset(time.Until(next))
		}
	}
}

func (r *Runner) tick(ctx context.Context, now time.Time) {
	jobs, err := r.repo.ListDue(ctx, now)
	if err != nil {
		r.log.Warn("list due scheduled jobs failed", zap.Error(err))
		return
	}
	for i := range jobs {
		r.fire(ctx, &jobs[i], now)
	}
}

// fire 为一个到期的定时任务创建扫描任务，并推进其 next_run。
// 无论提交成功与否都会推进 next_run，避免下一分钟对同一 job 反复重试。
func (r *Runner) fire(ctx context.Context, job *models.ScheduledJob, now time.Time) {
	// 有 modules 时，从 modules 推导 stages / params（与 API CreateTask 里的展开逻辑一致），
	// 这样 brute/dir/sensitive/search 这些非"传统 4 阶段"的模块也能被定时任务触发。
	stages := job.Stages
	params := cloneStringMap(job.Params)
	if params == nil {
		params = map[string]string{}
	}
	if len(job.Modules) > 0 {
		derivedStages, derivedParams := deriveFromModules(job.Modules)
		if len(derivedStages) > 0 {
			stages = derivedStages
		}
		for k, v := range derivedParams {
			if _, exists := params[k]; !exists {
				params[k] = v
			}
		}
	}

	task := &models.Task{
		UserID:       job.UserID,
		ProjectID:    job.ProjectID,
		Name:         job.Name + " (定时 " + now.Format("01-02 15:04") + ")",
		TemplateID:   job.TemplateID,
		TemplateName: job.TemplateName,
		Targets:      job.Targets,
		NodeIDs:      job.NodeIDs,
		Modules:      job.Modules,
		Config: models.TaskConfig{
			Stages: stages,
			Params: params,
		},
	}
	err := r.sched.Submit(ctx, task)
	// 撞名回退：分钟级时间戳撞名（比如手动新建了同名任务）时加秒后缀再试一次
	if errors.Is(err, scheduler.ErrTaskNameDuplicate) {
		task.Name = task.Name[:len(task.Name)-1] + ":" + now.Format("05") + ")"
		err = r.sched.Submit(ctx, task)
	}
	if err != nil {
		r.log.Warn("scheduled task submit failed",
			zap.String("job_id", job.ID.Hex()), zap.Error(err))
	} else {
		r.log.Info("scheduled task created",
			zap.String("job_id", job.ID.Hex()),
			zap.String("task_id", task.ID.Hex()))
	}

	nextRun := NextRun(job.Cron, now)
	if err := r.repo.MarkRun(ctx, job.ID, now, nextRun); err != nil {
		r.log.Warn("mark scheduled run failed", zap.String("job_id", job.ID.Hex()), zap.Error(err))
	}
}

// deriveFromModules 从 modules 结构里提取 stages 顺序和扁平化的参数键值对。
// 与 api/task.go CreateTask 的展开逻辑保持一致。
func deriveFromModules(mods map[string][]models.StagePlugin) (stages []string, params map[string]string) {
	params = map[string]string{}

	pluginToStage := map[string]string{
		"subfinder":   "subdomain",
		"ksubdomain":  "subdomain",
		"shuffledns":  "shuffledns",
		"bbot":        "bbot",
		"findomain":   "findomain",
		"naabu":       "port",
		"httpx":       "http",
		"fingerprint": "http",
		"nuclei":      "nuclei",
		"dirscan":     "dir",
		"brutescan":   "brute",
		"onlinesearch": "search",
		"crawler":    "crawler",
		"sensitive":   "sensitive",
	}

	// moduleOrder 与 api/task.go 保持同步
	order := []string{"search", "subdomain", "port", "http", "crawler", "vuln", "brute", "dir", "sensitive"}
	for _, mod := range order {
		plugins, ok := mods[mod]
		if !ok {
			continue
		}
		for _, sp := range plugins {
			if !sp.Enabled {
				continue
			}

			stageName := sp.Name
			if mapped, exists := pluginToStage[sp.Name]; exists {
				stageName = mapped
			}

			found := false
			for _, s := range stages {
				if s == stageName {
					found = true
					break
				}
			}
			if !found {
				stages = append(stages, stageName)
			}

			for k, v := range sp.Params {
				params[stageName+"."+k] = flattenParam(v)
			}
			// brutescan 特殊映射
			if sp.Name == "brutescan" {
				if svcs, ok := params["brute.services"]; ok && svcs != "" {
					params["brute.protocols"] = svcs
				}
			}
		}
	}
	return
}

func flattenParam(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case nil:
		return ""
	case []interface{}:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			} else {
				parts = append(parts, fmt.Sprint(item))
			}
		}
		return strings.Join(parts, ",")
	case float64:
		return fmt.Sprintf("%g", val)
	default:
		return fmt.Sprint(val)
	}
}

func cloneStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// NextRun 计算 cron 表达式在 after 之后的下次运行时间；解析失败返回 nil。
func NextRun(expr string, after time.Time) *time.Time {
	sched, err := cron.Parse(expr)
	if err != nil {
		return nil
	}
	t := sched.Next(after)
	if t.IsZero() {
		return nil
	}
	return &t
}
