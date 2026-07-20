package subdomain

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yourname/nscan/internal/scanner/engine"
	dnsresolver "github.com/yourname/nscan/internal/scanner/dns"
	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

const FindomainStageName = "findomain"

// FindomainStage 是 findomain 子域名枚举的独立 Pipeline Stage
type FindomainStage struct {
	log *zap.Logger
}

func NewFindomainStage(log *zap.Logger) *FindomainStage {
	return &FindomainStage{log: log}
}

func (s *FindomainStage) Name() string { return FindomainStageName }

func (s *FindomainStage) Run(
	ctx context.Context,
	input *engine.StageInput,
	params map[string]string,
	results chan<- *engine.ScanResult,
	progress chan<- *engine.Progress,
) (*engine.StageInput, error) {

	var path string
	for _, name := range []string{"findomain", "findomain-linux", "findomain-macos", "findomain-osx"} {
		if p, err := exec.LookPath(name); err == nil {
			path = p
			break
		}
	}

	if path == "" {
		engine.SendLog(progress, FindomainStageName, "warn", "[findomain] 未安装，跳过。下载: https://github.com/Findomain/Findomain/releases")
		return nil, nil
	}

	perTargetTimeout := 5 * time.Minute
	if v := strings.TrimSpace(params["findomain.per_target_timeout"]); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			perTargetTimeout = d
		} else if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perTargetTimeout = time.Duration(n) * time.Second
		}
	}
	engine.SendLog(progress, FindomainStageName, "info", fmt.Sprintf("[findomain] 单目标超时: %s", perTargetTimeout))

	concurrency := 10
	if v := strings.TrimSpace(params["findomain.concurrency"]); v != "" {
		if c, err := strconv.Atoi(v); err == nil && c > 0 {
			concurrency = c
		}
	}
	engine.SendLog(progress, FindomainStageName, "info", fmt.Sprintf("[findomain] 单目标并发: %d", concurrency))

	var subdomains []string
	var mu sync.Mutex
	var done atomic.Int32
	total := len(input.Targets)

	opts := engine.PoolOptions{
		Concurrency:    concurrency,
		PerItemTimeout: perTargetTimeout,
	}

	engine.RunPool(ctx, input.Targets, opts, func(tctx context.Context, target string) error {
		if !isDomain(target) {
			engine.SendLog(progress, FindomainStageName, "info", fmt.Sprintf("[findomain] 跳过非域名目标: %s", target))
			done.Add(1)
			return nil
		}

		engine.SendLog(progress, FindomainStageName, "info", fmt.Sprintf("[findomain] 开始枚举: %s", target))
		subs, err := runFindomain(tctx, path, target, progress)
		if err != nil {
			if tctx.Err() == context.DeadlineExceeded {
				engine.SendLog(progress, FindomainStageName, "warn", fmt.Sprintf("[findomain] %s 超时，已跳过", target))
			} else {
				engine.SendLog(progress, FindomainStageName, "warn", fmt.Sprintf("[findomain] %s 枚举失败: %v", target, err))
			}
		}
		engine.SendLog(progress, FindomainStageName, "info", fmt.Sprintf("[findomain] %s 发现 %d 个子域名", target, len(subs)))

		for _, sub := range subs {
			mu.Lock()
			subdomains = append(subdomains, sub)
			mu.Unlock()
			
			ips, _ := dnsresolver.LookupHost(tctx, params["resolvers"], sub)
			asset := &models.SubdomainAsset{Domain: sub, IPs: ips, Sources: []string{FindomainStageName}}
			r, _ := engine.NewResult("subdomain", asset)
			select {
			case results <- r:
			case <-tctx.Done():
				return tctx.Err()
			}
		}

		d := done.Add(1)
		pct := d * 100 / int32(total)
		select {
		case progress <- &engine.Progress{Stage: FindomainStageName, Percent: pct, Message: fmt.Sprintf("processed %s", target)}:
		default:
		}
		
		return err
	})

	return &engine.StageInput{Subdomains: subdomains}, nil
}

func runFindomain(ctx context.Context, path, domain string, progress chan<- *engine.Progress) ([]string, error) {
	args := []string{
		"-t", domain,
		"-q",
	}

	engine.SendLog(progress, FindomainStageName, "info", fmt.Sprintf("[findomain] 执行命令: %s %v", path, args))
	cmd := exec.CommandContext(ctx, path, args...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if out.Len() == 0 {
			return nil, fmt.Errorf("%w, stderr: %s", err, stderr.String())
		}
	}

	if stderr.Len() > 0 {
		engine.SendLog(progress, FindomainStageName, "warn", fmt.Sprintf("[findomain] 部分错误: %s", stderr.String()[:min(len(stderr.String()), 200)]))
	}

	var results []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, ".") {
			continue
		}
		if strings.HasPrefix(line, "[") || strings.HasPrefix(line, "\x1b") {
			continue
		}
		results = append(results, strings.ToLower(line))
	}

	return results, nil
}
