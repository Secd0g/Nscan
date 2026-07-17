package dir

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
	"go.uber.org/zap"
)

const StageName = "dir"

type Stage struct {
	log *zap.Logger
}

func New(log *zap.Logger) *Stage {
	return &Stage{log: log}
}

func (s *Stage) Name() string { return StageName }

type ffufResult struct {
	Input struct {
		FUZZ string `json:"FUZZ"`
	} `json:"input"`
	Position    int    `json:"position"`
	Status      int    `json:"status"`
	Length      int    `json:"length"`
	Words       int    `json:"words"`
	Lines       int    `json:"lines"`
	ContentType string `json:"content-type"`
	RedirectURL string `json:"redirectlocation"`
	URL         string `json:"url"`
	Host        string `json:"host"`
}

type dirResult struct {
	URL         string `json:"url"`
	Path        string `json:"path"`
	StatusCode  int    `json:"status_code"`
	ContentLen  int    `json:"content_len"`
	ContentType string `json:"content_type"`
	RedirectURL string `json:"redirect_url,omitempty"`
}

var defaultWordlist = []string{
	"admin", "login", "api", "config", "backup", "test", "dev",
	"console", "manager", "admin.php", "login.php", "index.php",
	"wp-admin", "wp-login.php", "wp-content", "wp-includes",
	"phpmyadmin", "phpinfo.php", ".env", ".git", ".git/config",
	".svn", ".DS_Store", "robots.txt", "sitemap.xml",
	"web.config", "server-status", "swagger", "api/v1",
	"actuator", "actuator/health", "info", "metrics",
	".htaccess", ".htpasswd", "crossdomain.xml",
	"admin/login", "user/login", "dashboard",
	"upload", "uploads", "static", "assets",
	"img", "images", "css", "js",
	"docs", "doc", "help", "readme",
	"debug", "trace", "status", "health",
	"graphql", "graphiql", "playground",
	"v1", "v2", "api/v2", "api/docs",
	"swagger-ui.html", "swagger/index.html",
	"elmah.axd", "trace.axd",
}

func (s *Stage) Run(
	ctx context.Context,
	input *engine.StageInput,
	params map[string]string,
	results chan<- *engine.ScanResult,
	progress chan<- *engine.Progress,
) (*engine.StageInput, error) {

	urls := input.HTTPURLs
	if len(urls) == 0 {
		for _, t := range input.Targets {
			if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") {
				urls = append(urls, t)
			} else {
				urls = append(urls, "http://"+t)
			}
		}
	}
	if len(urls) == 0 {
		engine.SendLog(progress, StageName, "warn", "[dir] 无可扫描URL, 跳过")
		return nil, nil
	}
	if before := len(urls); true {
		urls = engine.FilterURLs(urls)
		if diff := before - len(urls); diff > 0 {
			engine.SendLog(progress, StageName, "info", fmt.Sprintf("[dir] URL 去重: %d → %d (去除 %d 个重复模式)", before, len(urls), diff))
		}
	}

	path, err := exec.LookPath("ffuf")
	if err != nil {
		engine.SendLog(progress, StageName, "warn", "[dir] ffuf 未安装, 跳过目录扫描")
		return nil, nil
	}

	threads := parseInt(params["threads"], 50)
	excludeCodesStr := params["exclude_codes"]
	if excludeCodesStr == "" {
		excludeCodesStr = "404,403,500,503"
	}

	extensions := normalizeExtensions(params["extensions"])

	wordlistFile, cleanup, err := s.prepareWordlist(params)
	if err != nil {
		return nil, fmt.Errorf("prepare wordlist: %w", err)
	}
	defer cleanup()

	concurrency := 3
	if v := parseInt(params["dir.concurrency"], 0); v > 0 {
		concurrency = v
	}

	perTargetTimeout := 10 * time.Minute
	if v := strings.TrimSpace(params["dir.per_target_timeout"]); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			perTargetTimeout = d
		}
	}

	engine.SendLog(progress, StageName, "info",
		fmt.Sprintf("[dir] 开始目录扫描 (ffuf), %d 个目标, 并发 %d, 线程 %d/目标", len(urls), concurrency, threads))

	var done atomic.Int32
	total := int32(len(urls))

	opts := engine.PoolOptions{
		Concurrency:    concurrency,
		PerItemTimeout: perTargetTimeout,
	}

	var allFound []string
	var foundMu = make(chan []string, len(urls))

	engine.RunPool(ctx, urls, opts, func(tctx context.Context, target string) error {
		found, err := s.runFfuf(tctx, path, target, wordlistFile, threads, excludeCodesStr, extensions, params, results, progress)
		if err != nil {
			if tctx.Err() == context.DeadlineExceeded {
				engine.SendLog(progress, StageName, "warn", fmt.Sprintf("[dir] %s 超时，已跳过", target))
			} else {
				engine.SendLog(progress, StageName, "warn", fmt.Sprintf("[dir] %s 扫描失败: %v", target, err))
			}
		}
		foundMu <- found
		d := done.Add(1)
		pct := d * 100 / total
		select {
		case progress <- &engine.Progress{Stage: StageName, Percent: pct, Message: fmt.Sprintf("scanned %s", target)}:
		default:
		}
		return err
	})

	close(foundMu)
	for f := range foundMu {
		allFound = append(allFound, f...)
	}

	engine.SendLog(progress, StageName, "info",
		fmt.Sprintf("[dir] 扫描完成, 发现 %d 个路径", len(allFound)))

	if len(allFound) > 0 {
		return &engine.StageInput{HTTPURLs: allFound}, nil
	}
	return nil, nil
}

func (s *Stage) prepareWordlist(params map[string]string) (string, func(), error) {
	if raw := strings.TrimSpace(params["wordlist_lines"]); raw != "" {
		f, err := os.CreateTemp("", "nscan-dir-*.txt")
		if err != nil {
			return "", func() {}, err
		}
		written := 0
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			line = strings.TrimPrefix(line, "/")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if _, err := fmt.Fprintln(f, line); err != nil {
				f.Close()
				os.Remove(f.Name())
				return "", func() {}, err
			}
			written++
		}
		if written == 0 {
			f.Close()
			os.Remove(f.Name())
			return s.prepareWordlist(map[string]string{})
		}
		if err := f.Close(); err != nil {
			os.Remove(f.Name())
			return "", func() {}, err
		}
		return f.Name(), func() { os.Remove(f.Name()) }, nil
	}

	f, err := os.CreateTemp("", "nscan-dir-default-*.txt")
	if err != nil {
		return "", func() {}, err
	}
	for _, w := range defaultWordlist {
		if _, err := fmt.Fprintln(f, w); err != nil {
			f.Close()
			os.Remove(f.Name())
			return "", func() {}, err
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", func() {}, err
	}
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}

func (s *Stage) runFfuf(
	ctx context.Context,
	path string,
	target string,
	wordlistFile string,
	threads int,
	filterCodes string,
	extensions string,
	params map[string]string,
	results chan<- *engine.ScanResult,
	progress chan<- *engine.Progress,
) ([]string, error) {
	target = strings.TrimRight(target, "/")

	args := []string{
		"-u", target + "/FUZZ",
		"-w", wordlistFile,
		"-t", strconv.Itoa(threads),
		"-fc", filterCodes,
		"-ac",
		"-json",
		"-s",
		"-noninteractive",
		"-timeout", strconv.Itoa(parseInt(params["timeout"], 15)),
	}

	if extensions != "" {
		args = append(args, "-e", extensions)
	}

	options := parseOptionSet(params["options"])
	if options["follow_redirects"] {
		args = append(args, "-r")
	}
	if options["recursive"] {
		args = append(args, "-recursion")
	}
	if proxy := strings.TrimSpace(params["global_proxy"]); proxy != "" {
		args = append(args, "-x", proxy)
	}

	cmd := exec.CommandContext(ctx, path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ffuf stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffuf start: %w", err)
	}
	pgid := cmd.Process.Pid
	processDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		case <-processDone:
		}
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var foundURLs []string

	for scanner.Scan() {
		if ctx.Err() != nil {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var fr ffufResult
		if err := json.Unmarshal([]byte(line), &fr); err != nil {
			continue
		}

		dr := dirResult{
			URL:         fr.URL,
			Path:        "/" + fr.Input.FUZZ,
			StatusCode:  fr.Status,
			ContentLen:  fr.Length,
			ContentType: fr.ContentType,
			RedirectURL: fr.RedirectURL,
		}
		r, _ := engine.NewResult("dir", dr)
		select {
		case results <- r:
		case <-ctx.Done():
		}

		foundURLs = append(foundURLs, fr.URL)
		engine.SendLog(progress, StageName, "info",
			fmt.Sprintf("[dir] %d %s (%d bytes)", fr.Status, fr.URL, fr.Length))
	}

	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	close(processDone)
	if ctx.Err() != nil {
		return foundURLs, ctx.Err()
	}
	if scanErr != nil {
		return foundURLs, fmt.Errorf("read ffuf output: %w", scanErr)
	}
	if waitErr != nil {
		return foundURLs, fmt.Errorf("ffuf exited: %w", waitErr)
	}
	return foundURLs, nil
}

func normalizeExtensions(raw string) string {
	var normalized []string
	for _, extension := range strings.Split(raw, ",") {
		extension = strings.TrimSpace(extension)
		if extension == "" {
			continue
		}
		if !strings.HasPrefix(extension, ".") {
			extension = "." + extension
		}
		normalized = append(normalized, extension)
	}
	return strings.Join(normalized, ",")
}

func parseOptionSet(raw string) map[string]bool {
	options := make(map[string]bool)
	for _, option := range strings.Split(raw, ",") {
		if option = strings.TrimSpace(option); option != "" {
			options[option] = true
		}
	}
	return options
}

func parseInt(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return def
}
