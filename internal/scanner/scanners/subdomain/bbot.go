package subdomain

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/yourname/nscan/internal/scanner/engine"
	dnsresolver "github.com/yourname/nscan/internal/scanner/dns"
	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

const BbotStageName = "bbot"

type BbotStage struct {
	log *zap.Logger
}

// BBOT installs/checks shared Ansible collections in ~/.config and ~/.ansible
// during startup. Running several scans concurrently makes ansible-galaxy
// race on its temporary "targets" directory. Serialize invocations per
// scanner process; BBOT still performs its own internal enumeration work.
var bbotRunMu sync.Mutex

func NewBbotStage(log *zap.Logger) *BbotStage {
	return &BbotStage{log: log}
}

func (s *BbotStage) Name() string { return BbotStageName }

func (s *BbotStage) Run(
	ctx context.Context,
	input *engine.StageInput,
	params map[string]string,
	results chan<- *engine.ScanResult,
	progress chan<- *engine.Progress,
) (*engine.StageInput, error) {

	if runtime.GOOS != "linux" {
		engine.SendLog(progress, BbotStageName, "warn", "[bbot] 仅支持 Linux 系统，当前系统跳过")
		return nil, nil
	}

	if pkJSON := params["bbot.provider_keys"]; pkJSON != "" {
		if err := writeBbotSecrets(pkJSON); err != nil {
			engine.SendLog(progress, BbotStageName, "warn", fmt.Sprintf("[bbot] 写入 secrets.yml 失败: %v", err))
		} else {
			engine.SendLog(progress, BbotStageName, "info", "[bbot] API 密钥已写入 secrets.yml")
		}
	}

	path, err := exec.LookPath("bbot")
	if err != nil {
		// 软链接可能丢失，直接找 pipx venv 里的二进制
		home := os.Getenv("HOME")
		if home == "" {
			home = "/root"
		}
		candidate := filepath.Join(home, ".local", "pipx", "venvs", "bbot", "bin", "bbot")
		if _, serr := os.Stat(candidate); serr == nil {
			path = candidate
		} else {
			engine.SendLog(progress, BbotStageName, "warn", "[bbot] 未安装，跳过。安装: pipx install bbot")
			return nil, nil
		}
	}

	perTargetTimeout := 60 * time.Minute
	if v := strings.TrimSpace(params["bbot.per_target_timeout"]); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			perTargetTimeout = d
		} else if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perTargetTimeout = time.Duration(n) * time.Second
		}
	}
	engine.SendLog(progress, BbotStageName, "info", fmt.Sprintf("[bbot] 单目标超时: %s", perTargetTimeout))

	concurrency := 3
	if v := strings.TrimSpace(params["bbot.concurrency"]); v != "" {
		if c, err := strconv.Atoi(v); err == nil && c > 0 {
			concurrency = c
		}
	}
	engine.SendLog(progress, BbotStageName, "info", fmt.Sprintf("[bbot] 单目标并发: %d", concurrency))

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
			engine.SendLog(progress, BbotStageName, "info", fmt.Sprintf("[bbot] 跳过非域名目标: %s", target))
			done.Add(1)
			return nil
		}

		engine.SendLog(progress, BbotStageName, "info", fmt.Sprintf("[bbot] 开始枚举: %s", target))

		count := 0
		emit := func(sub string) {
			mu.Lock()
			subdomains = append(subdomains, sub)
			mu.Unlock()
			ips, _ := dnsresolver.LookupHost(tctx, params["resolvers"], sub)
			asset := &models.SubdomainAsset{Domain: sub, IPs: ips, Sources: []string{BbotStageName}}
			r, _ := engine.NewResult("subdomain", asset)
			select {
			case results <- r:
				count++
			case <-tctx.Done():
			}
		}

		start := time.Now()
		err := runBbot(tctx, path, target, progress, emit)
		elapsed := time.Since(start).Round(time.Second)
		if err != nil {
			if tctx.Err() == context.DeadlineExceeded {
				engine.SendLog(progress, BbotStageName, "warn", fmt.Sprintf("[bbot] %s 超时(%s)，已跳过", target, elapsed))
			} else {
				engine.SendLog(progress, BbotStageName, "warn", fmt.Sprintf("[bbot] %s 枚举失败: %v", target, err))
			}
		}
		
		d := done.Add(1)
		engine.SendLog(progress, BbotStageName, "info", fmt.Sprintf("[bbot] (%d/%d) %s 发现 %d 个子域名, 耗时 %s", d, total, target, count, elapsed))

		pct := d * 100 / int32(total)
		select {
		case progress <- &engine.Progress{Stage: BbotStageName, Percent: pct, Message: fmt.Sprintf("processed %s", target)}:
		default:
		}
		
		return err
	})

	return &engine.StageInput{Subdomains: subdomains}, nil
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[mGKHF]`)

func stripAnsi(s string) string { return ansiEscape.ReplaceAllString(s, "") }

// writeBbotSecrets writes API keys from JSON map to ~/.config/bbot/secrets.yml
func writeBbotSecrets(pkJSON string) error {
	var keys map[string]string
	if err := json.Unmarshal([]byte(pkJSON), &keys); err != nil {
		return fmt.Errorf("parse provider_keys: %w", err)
	}
	home := os.Getenv("HOME")
	if home == "" {
		home = "/root"
	}
	cfgDir := filepath.Join(home, ".config", "bbot")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		return err
	}
	var sb strings.Builder
	for svc, key := range keys {
		sb.WriteString(svc)
		sb.WriteString(":\n  api_key: ")
		sb.WriteString(key)
		sb.WriteString("\n")
	}
	return os.WriteFile(filepath.Join(cfgDir, "secrets.yml"), []byte(sb.String()), 0o600)
}

func runBbot(ctx context.Context, path, domain string, progress chan<- *engine.Progress, emit func(string)) error {
	bbotRunMu.Lock()
	defer bbotRunMu.Unlock()

	// 用域名 MD5 作为扫描名，固定输出路径，与 scope-sentry 保持一致
	scanName := fmt.Sprintf("%x", md5.Sum([]byte(domain)))
	outDir := filepath.Join(os.TempDir(), "nscan_bbot_results")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	scanDir := filepath.Join(outDir, scanName)
	defer os.RemoveAll(scanDir)

	// 排除会无限扩展或耗时长的模块，每个单独传 -em 以确保所有 bbot 版本均生效
	args := []string{
		"-t", domain,
		"-f", "subdomain-enum",
		"-y",
		"-n", scanName,
		"-o", outDir,
		"-om", "json",
		"--ignore-failed-deps",
	}

	engine.SendLog(progress, BbotStageName, "info", fmt.Sprintf("[bbot] 执行命令: %s %v", path, args))
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("create pipe: %w", err)
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return fmt.Errorf("start bbot: %w", err)
	}
	pw.Close()

	// Kill entire process group when context is cancelled so bbot's child
	// processes (curl, dns resolvers, etc.) don't linger as orphans.
	pgid := cmd.Process.Pid
	go func() {
		<-ctx.Done()
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}()

	// 流式解析 output.json（bbot 逐行追加写入），发现一个子域名就实时 emit
	tailCtx, tailCancel := context.WithCancel(ctx)
	defer tailCancel()
	tailDone := make(chan struct{})
	go func() {
		defer close(tailDone)
		tailBbotJSON(tailCtx, filepath.Join(scanDir, "output.json"), emit)
	}()

	// bbot 的子进程（massdns、python）会继承 pw 的文件描述符。
	// bbot 主进程退出后这些子进程变成僵尸，pw 写端不关闭，sc.Scan() 永远阻塞。
	// 用独立 goroutine 读日志，靠 pr.Close() 强制中断。
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			line := stripAnsi(sc.Text())
			if line != "" {
				engine.SendLog(progress, BbotStageName, "info", line)
			}
		}
	}()

	// Wait 在单独 goroutine 里，主进程退出后强制关闭 pr 中断扫描 goroutine
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	select {
	case waitErr := <-waitDone:
		if waitErr != nil && ctx.Err() == nil {
			engine.SendLog(progress, BbotStageName, "warn", fmt.Sprintf("[bbot] 命令异常退出: %v", waitErr))
		}
	case <-ctx.Done():
	}
	// bbot 主进程退出后，massdns/python 子进程仍持有 pw 写端导致 pipe 永不 EOF。
	// Kill 整个进程组确保所有子进程退出，pipe 才能关闭。
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	pr.Close()
	<-scanDone

	// 让 tail 再消费一小段，读完剩余行后停止
	time.Sleep(500 * time.Millisecond)
	tailCancel()
	<-tailDone
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// tailBbotJSON 持续读取 output.json 追加行，遇到 in-scope 的 DNS_NAME 事件就调用 emit
func tailBbotJSON(ctx context.Context, jsonPath string, emit func(string)) {
	// 等待文件出现（bbot 可能过几秒才创建）
	var f *os.File
	for {
		if ctx.Err() != nil {
			return
		}
		if fh, err := os.Open(jsonPath); err == nil {
			f = fh
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	defer f.Close()

	seen := make(map[string]bool)
	reader := bufio.NewReader(f)
	var pending strings.Builder

	for {
		if ctx.Err() != nil {
			// 排空最后剩余的数据
			flushBbotBuffer(reader, &pending, seen, emit)
			return
		}
		line, err := reader.ReadString('\n')
		if line != "" {
			pending.WriteString(line)
			if strings.HasSuffix(line, "\n") {
				parseBbotLine(strings.TrimSpace(pending.String()), seen, emit)
				pending.Reset()
			}
		}
		if err != nil {
			// EOF：等待更多数据
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func flushBbotBuffer(reader *bufio.Reader, pending *strings.Builder, seen map[string]bool, emit func(string)) {
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			pending.WriteString(line)
			if strings.HasSuffix(line, "\n") {
				parseBbotLine(strings.TrimSpace(pending.String()), seen, emit)
				pending.Reset()
			}
		}
		if err != nil {
			if pending.Len() > 0 {
				parseBbotLine(strings.TrimSpace(pending.String()), seen, emit)
				pending.Reset()
			}
			return
		}
	}
}

func parseBbotLine(line string, seen map[string]bool, emit func(string)) {
	if line == "" {
		return
	}
	var ev bbotEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return
	}
	if ev.Type != "DNS_NAME" {
		return
	}
	hasSubdomain, isInScope := false, false
	for _, tag := range ev.Tags {
		if tag == "subdomain" {
			hasSubdomain = true
		}
		if tag == "in-scope" {
			isInScope = true
		}
	}
	if !hasSubdomain || !isInScope {
		return
	}
	sub := strings.ToLower(ev.Data)
	if seen[sub] {
		return
	}
	seen[sub] = true
	emit(sub)
}

type bbotEvent struct {
	Type string   `json:"type"`
	Data string   `json:"data"`
	Tags []string `json:"tags"`
}
