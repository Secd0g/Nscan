package nuclei

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/yourname/nscan/internal/scanner/engine"
	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

const StageName = "nuclei"

type Stage struct {
	log          *zap.Logger
	templatesDir string
}

func New(log *zap.Logger, templatesDir string) *Stage {
	return &Stage{log: log, templatesDir: templatesDir}
}

func (s *Stage) Name() string { return StageName }

// targetGroup groups targets by protocol type for nuclei template filtering.
type targetGroup struct {
	targets      []string
	protoFilter  string // "-pt" value (e.g. "http") — empty means no filter
	excludeProto string // "-ept" value (e.g. "http") — empty means no exclude
	tags         []string
	label        string
}

func (s *Stage) Run(
	ctx context.Context,
	input *engine.StageInput,
	params map[string]string,
	results chan<- *engine.ScanResult,
	progress chan<- *engine.Progress,
) (*engine.StageInput, error) {

	var groups []targetGroup

	// Group 1: HTTP URLs — only run HTTP protocol templates
	httpTargets := engine.FilterURLs(input.HTTPURLs)
	if len(httpTargets) == 0 {
		for _, t := range input.Targets {
			if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") {
				httpTargets = append(httpTargets, t)
			}
		}
	}
	if len(httpTargets) > 0 {
		var techTags []string
		if input.HTTPTechMap != nil {
			seen := make(map[string]struct{})
			for _, u := range httpTargets {
				for _, t := range input.HTTPTechMap[u] {
					tag := strings.ToLower(strings.ReplaceAll(t, " ", "-"))
					if _, ok := seen[tag]; !ok {
						seen[tag] = struct{}{}
						techTags = append(techTags, tag)
					}
				}
			}
		}
		groups = append(groups, targetGroup{
			targets:     httpTargets,
			protoFilter: "http",
			tags:        techTags,
			label:       "HTTP",
		})
	}

	// Group 2: non-HTTP host:port — exclude HTTP protocol templates
	var nonHTTPTargets []string
	nonHTTPTargets = append(nonHTTPTargets, input.Hosts...)
	if len(nonHTTPTargets) == 0 {
		for _, t := range input.Targets {
			if !strings.HasPrefix(t, "http://") && !strings.HasPrefix(t, "https://") && strings.Contains(t, ":") {
				nonHTTPTargets = append(nonHTTPTargets, t)
			}
		}
	}
	if len(nonHTTPTargets) > 0 {
		groups = append(groups, targetGroup{
			targets:      nonHTTPTargets,
			excludeProto: "http",
			label:        "TCP/UDP",
		})
	}

	// Group 3: bare domains/IPs without scheme or port — nuclei handles them natively
	if len(groups) == 0 && len(input.Targets) > 0 {
		var bareTargets []string
		for _, t := range input.Targets {
			if !strings.HasPrefix(t, "http://") && !strings.HasPrefix(t, "https://") && !strings.Contains(t, ":") {
				bareTargets = append(bareTargets, t)
			}
		}
		if len(bareTargets) > 0 {
			groups = append(groups, targetGroup{
				targets: bareTargets,
				label:   "ALL",
			})
		}
	}

	if len(groups) == 0 {
		engine.SendLog(progress, StageName, "warn", "[nuclei] 无可扫描目标, 跳过")
		return nil, nil
	}

	path, err := exec.LookPath("nuclei")
	if err != nil {
		engine.SendLog(progress, StageName, "warn", "[nuclei] nuclei 未安装, 跳过漏洞扫描")
		return nil, nil
	}

	severity := params["severity"]
	if severity == "" {
		severity = "critical,high,medium"
	}

	concurrency := 5
	if v := strings.TrimSpace(params["nuclei.concurrency"]); v != "" {
		if c, err := strconv.Atoi(v); err == nil && c > 0 {
			concurrency = c
		}
	}

	perTargetTimeout := 30 * time.Minute
	if v := strings.TrimSpace(params["nuclei.per_target_timeout"]); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			perTargetTimeout = d
		} else if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perTargetTimeout = time.Duration(n) * time.Second
		}
	}
	engine.SendLog(progress, StageName, "info", fmt.Sprintf("[nuclei] 单目标并发: %d, 超时: %s", concurrency, perTargetTimeout))

	var totalFound atomic.Int32

	for _, g := range groups {
		if ctx.Err() != nil {
			break
		}
		engine.SendLog(progress, StageName, "info", fmt.Sprintf("[nuclei] 开始 %s 组扫描: %d 个目标, 严重等级: %s", g.label, len(g.targets), severity))
		if len(g.tags) > 0 {
			engine.SendLog(progress, StageName, "info", fmt.Sprintf("[nuclei] %s 组技术栈标签: %s", g.label, strings.Join(g.tags, ",")))
		}

		var done atomic.Int32
		total := len(g.targets)

		opts := engine.PoolOptions{
			Concurrency:    concurrency,
			PerItemTimeout: perTargetTimeout,
		}

		grp := g
		engine.RunPool(ctx, g.targets, opts, func(tctx context.Context, target string) error {
			err := s.runNuclei(tctx, path, target, severity, params["global_proxy"], grp.protoFilter, grp.excludeProto, grp.tags, results, progress, &totalFound)
			if err != nil {
				if tctx.Err() == context.DeadlineExceeded {
					engine.SendLog(progress, StageName, "warn", fmt.Sprintf("[nuclei] %s 超时，已跳过", target))
				} else {
					engine.SendLog(progress, StageName, "warn", fmt.Sprintf("[nuclei] %s 扫描失败: %v", target, err))
				}
			}

			d := done.Add(1)
			pct := d * 100 / int32(total)
			select {
			case progress <- &engine.Progress{Stage: StageName, Percent: pct, Message: fmt.Sprintf("processed %s", target)}:
			default:
			}
			return err
		})
	}

	select {
	case progress <- &engine.Progress{Stage: StageName, Percent: 100, Message: fmt.Sprintf("found %d vulns", totalFound.Load())}:
	default:
	}
	s.log.Info("nuclei scan done", zap.Int32("vulns", totalFound.Load()))

	return nil, nil
}

// nuclei -jsonl output structure
type nucleiResult struct {
	TemplateID string `json:"template-id"`
	Name       string `json:"info.name"`
	Severity   string `json:"info.severity"`
	MatchedAt  string `json:"matched-at"`
	Host       string `json:"host"`
	Info       struct {
		Name     string `json:"name"`
		Severity string `json:"severity"`
	} `json:"info"`
	Request  string `json:"request"`
	Response string `json:"response"`
}

func (s *Stage) runNuclei(
	ctx context.Context,
	path string,
	target string,
	severity string,
	proxy string,
	protoFilter string,
	excludeProto string,
	tags []string,
	results chan<- *engine.ScanResult,
	progress chan<- *engine.Progress,
	found *atomic.Int32,
) error {
	args := []string{
		"-silent", "-jsonl",
		"-severity", severity,
		"-nc",
		"-u", target,
	}
	if s.templatesDir != "" {
		args = append(args, "-t", s.templatesDir)
	}
	if protoFilter != "" {
		args = append(args, "-pt", protoFilter)
	}
	if excludeProto != "" {
		args = append(args, "-ept", excludeProto)
	}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}

	cmd := exec.CommandContext(ctx, path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if proxy != "" {
		cmd.Env = append(os.Environ(),
			"HTTP_PROXY="+proxy,
			"HTTPS_PROXY="+proxy,
			"ALL_PROXY="+proxy,
		)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("nuclei stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("nuclei start: %w", err)
	}
	pgid := cmd.Process.Pid
	go func() {
		<-ctx.Done()
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		if ctx.Err() != nil {
			cmd.Process.Kill()
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var nr nucleiResult
		if err := json.Unmarshal([]byte(line), &nr); err != nil {
			continue
		}

		asset := &models.VulnAsset{
			Target:     nr.Host,
			TemplateID: nr.TemplateID,
			Name:       nr.Info.Name,
			Severity:   nr.Info.Severity,
			MatchedAt:  nr.MatchedAt,
			Request:    nr.Request,
			Response:   nr.Response,
		}
		r, _ := engine.NewResult("vuln", asset)
		select {
		case results <- r:
		case <-ctx.Done():
		}
		found.Add(1)
		engine.SendLog(progress, StageName, "info", fmt.Sprintf("[nuclei] %s 发现漏洞: [%s] %s → %s", target, nr.Info.Severity, nr.Info.Name, nr.MatchedAt))
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		s.log.Warn("nuclei stdout scan error", zap.Error(err))
	}
	if err := cmd.Wait(); err != nil && ctx.Err() == nil {
		s.log.Warn("nuclei exited with error", zap.Error(err))
	}

	return nil
}
