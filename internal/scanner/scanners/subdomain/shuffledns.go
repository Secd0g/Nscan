package subdomain

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yourname/nscan/internal/scanner/engine"
	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

const ShufflednsStageName = "shuffledns"

// ShufflednsStage 是 shuffledns 的独立 Pipeline Stage
type ShufflednsStage struct {
	log *zap.Logger
}

func NewShufflednsStage(log *zap.Logger) *ShufflednsStage {
	return &ShufflednsStage{log: log}
}

func (s *ShufflednsStage) Name() string { return ShufflednsStageName }

func (s *ShufflednsStage) Run(
	ctx context.Context,
	input *engine.StageInput,
	params map[string]string,
	results chan<- *engine.ScanResult,
	progress chan<- *engine.Progress,
) (*engine.StageInput, error) {

	path, err := exec.LookPath("shuffledns")
	if err != nil {
		engine.SendLog(progress, ShufflednsStageName, "warn", "[shuffledns] 未安装，跳过。安装: go install -v github.com/projectdiscovery/shuffledns/cmd/shuffledns@latest")
		return nil, nil
	}

	wl := params["wordlist_lines"]
	resolvers := params["resolvers"]

	perTargetTimeout := 5 * time.Minute
	if v := strings.TrimSpace(params["shuffledns.per_target_timeout"]); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			perTargetTimeout = d
		} else if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perTargetTimeout = time.Duration(n) * time.Second
		}
	}
	engine.SendLog(progress, ShufflednsStageName, "info", fmt.Sprintf("[shuffledns] 单目标超时: %s", perTargetTimeout))

	concurrency := 5
	if v := strings.TrimSpace(params["shuffledns.concurrency"]); v != "" {
		if c, err := strconv.Atoi(v); err == nil && c > 0 {
			concurrency = c
		}
	}
	engine.SendLog(progress, ShufflednsStageName, "info", fmt.Sprintf("[shuffledns] 单目标并发: %d", concurrency))

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
			engine.SendLog(progress, ShufflednsStageName, "info", fmt.Sprintf("[shuffledns] 跳过非域名目标: %s", target))
			done.Add(1)
			return nil
		}

		engine.SendLog(progress, ShufflednsStageName, "info", fmt.Sprintf("[shuffledns] 开始处理: %s", target))

		var subs []string
		var err error
		if wl != "" {
			// 爆破模式
			engine.SendLog(progress, ShufflednsStageName, "info", fmt.Sprintf("[shuffledns] 字典模式"))
			subs, err = runShufflednsBrute(tctx, path, target, wl, resolvers, progress)
		} else if len(input.Subdomains) > 0 {
			// 解析模式，利用之前 stage 传来的已发现子域名
			engine.SendLog(progress, ShufflednsStageName, "info", fmt.Sprintf("[shuffledns] 解析模式，待验证: %d 个", len(input.Subdomains)))
			resolveList := strings.Join(input.Subdomains, "\n")
			subs, err = runShufflednsResolve(tctx, path, target, resolveList, resolvers, progress)
		} else {
			engine.SendLog(progress, ShufflednsStageName, "info", "[shuffledns] 无字典且无待解析列表，跳过")
			done.Add(1)
			return nil
		}

		if err != nil {
			if tctx.Err() == context.DeadlineExceeded {
				engine.SendLog(progress, ShufflednsStageName, "warn", fmt.Sprintf("[shuffledns] %s 超时，已跳过", target))
			} else {
				engine.SendLog(progress, ShufflednsStageName, "warn", fmt.Sprintf("[shuffledns] %s 处理失败: %v", target, err))
			}
		}
		engine.SendLog(progress, ShufflednsStageName, "info", fmt.Sprintf("[shuffledns] %s 发现/验证 %d 个子域名", target, len(subs)))

		for _, sub := range subs {
			mu.Lock()
			subdomains = append(subdomains, sub)
			mu.Unlock()
			
			asset := &models.SubdomainAsset{Domain: sub, Sources: []string{ShufflednsStageName}}
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
		case progress <- &engine.Progress{Stage: ShufflednsStageName, Percent: pct, Message: fmt.Sprintf("processed %s", target)}:
		default:
		}
		
		return err
	})

	return &engine.StageInput{Subdomains: subdomains}, nil
}

func runShufflednsBrute(ctx context.Context, path, domain, wordlistLines, resolvers string, progress chan<- *engine.Progress) ([]string, error) {
	wlFile, err := os.CreateTemp("", "shuffledns_wl_*.txt")
	if err != nil {
		return nil, fmt.Errorf("create wordlist file: %w", err)
	}
	defer os.Remove(wlFile.Name())
	if _, err := wlFile.WriteString(wordlistLines); err != nil {
		wlFile.Close()
		return nil, fmt.Errorf("write wordlist: %w", err)
	}
	wlFile.Close()

	return runShuffledns(ctx, path, domain, []string{"-d", domain, "-w", wlFile.Name()}, resolvers, progress)
}

func runShufflednsResolve(ctx context.Context, path, domain, resolveList, resolvers string, progress chan<- *engine.Progress) ([]string, error) {
	listFile, err := os.CreateTemp("", "shuffledns_list_*.txt")
	if err != nil {
		return nil, fmt.Errorf("create list file: %w", err)
	}
	defer os.Remove(listFile.Name())
	if _, err := listFile.WriteString(resolveList); err != nil {
		listFile.Close()
		return nil, fmt.Errorf("write list: %w", err)
	}
	listFile.Close()

	return runShuffledns(ctx, path, domain, []string{"-d", domain, "-list", listFile.Name()}, resolvers, progress)
}

func runShuffledns(ctx context.Context, path, domain string, extraArgs []string, resolvers string, progress chan<- *engine.Progress) ([]string, error) {
	if resolvers == "" {
		resolvers = "8.8.8.8\n1.1.1.1\n223.5.5.5\n114.114.114.114"
	}
	rFile, err := os.CreateTemp("", "shuffledns_resolvers_*.txt")
	if err != nil {
		return nil, fmt.Errorf("create resolvers file: %w", err)
	}
	defer os.Remove(rFile.Name())
	if _, err := rFile.WriteString(resolvers); err != nil {
		rFile.Close()
		return nil, fmt.Errorf("write resolvers: %w", err)
	}
	rFile.Close()

	outFile, err := os.CreateTemp("", "shuffledns_out_*.txt")
	if err != nil {
		return nil, fmt.Errorf("create output file: %w", err)
	}
	outFile.Close()
	defer os.Remove(outFile.Name())

	args := append(extraArgs,
		"-r", rFile.Name(),
		"-o", outFile.Name(),
		"-silent",
	)

	engine.SendLog(progress, ShufflednsStageName, "info", fmt.Sprintf("[shuffledns] 执行命令: %s %v", path, args))
	cmd := exec.CommandContext(ctx, path, args...)
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	if err := cmd.Run(); err != nil {
		combinedStr := combined.String()
		if combinedStr != "" {
			engine.SendLog(progress, ShufflednsStageName, "warn", fmt.Sprintf("[shuffledns] 部分错误: %s", combinedStr[:min(len(combinedStr), 200)]))
		}
	}

	data, err := os.ReadFile(outFile.Name())
	if err != nil {
		return nil, fmt.Errorf("read output: %w", err)
	}

	var results []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && strings.Contains(line, ".") {
			results = append(results, strings.ToLower(line))
		}
	}
	return results, nil
}
