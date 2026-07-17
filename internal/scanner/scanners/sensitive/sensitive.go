// Package sensitive 敏感信息扫描 stage：双引擎（正则 + TruffleHog）对页面做敏感信息检测。
// 支持分块匹配避免大 body 截断漏检，支持密钥在线验证。
package sensitive

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yourname/nscan/internal/scanner/engine"
	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

const StageName = "sensitive"

type Stage struct {
	log *zap.Logger
}

func New(log *zap.Logger) *Stage { return &Stage{log: log} }

func (s *Stage) Name() string { return StageName }

type compiledRule struct {
	ID       string
	Name     string
	Severity string
	Re       *regexp.Regexp
}

type wireRule struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Pattern  string `json:"pattern"`
	Severity string `json:"severity"`
}

// chunkHit 分块匹配命中结果
type chunkHit struct {
	RuleIdx int
	Matched string
}

func (s *Stage) Run(
	ctx context.Context,
	input *engine.StageInput,
	params map[string]string,
	results chan<- *engine.ScanResult,
	progress chan<- *engine.Progress,
) (*engine.StageInput, error) {
	rulesJSON := params["rules"]
	if strings.TrimSpace(rulesJSON) == "" {
		engine.SendLog(progress, StageName, "warn", "[sensitive] 未注入规则，跳过（请在「敏感规则」中启用至少一条）")
		return nil, nil
	}
	var wire []wireRule
	if err := json.Unmarshal([]byte(rulesJSON), &wire); err != nil {
		engine.SendLog(progress, StageName, "error", fmt.Sprintf("[sensitive] 规则解析失败: %v", err))
		return nil, nil
	}
	rules := make([]compiledRule, 0, len(wire))
	for _, w := range wire {
		re, err := regexp.Compile(w.Pattern)
		if err != nil {
			engine.SendLog(progress, StageName, "warn", fmt.Sprintf("[sensitive] 规则 %q 正则无效: %v", w.Name, err))
			continue
		}
		rules = append(rules, compiledRule{ID: w.ID, Name: w.Name, Severity: w.Severity, Re: re})
	}
	if len(rules) == 0 {
		engine.SendLog(progress, StageName, "warn", "[sensitive] 无可用规则，跳过")
		return nil, nil
	}

	// 初始化 TruffleHog 扫描器
	useTrufflehog := params["trufflehog"] != "false"
	verify := params["verify"] == "true"
	var thScanner *TruffleHogScanner
	if useTrufflehog {
		var err error
		thScanner, err = NewTruffleHogScanner(s.log)
		if err != nil {
			engine.SendLog(progress, StageName, "warn", fmt.Sprintf("[sensitive] TruffleHog 初始化失败: %v, 仅使用正则引擎", err))
		} else {
			engine.SendLog(progress, StageName, "info",
				fmt.Sprintf("[sensitive] TruffleHog 引擎就绪: %d 个检测器, verify=%v", thScanner.DetectorCount(), verify))
		}
	}

	chunkSize := parseInt(params["chunk_size"], 4096)
	chunkOverlap := parseInt(params["chunk_overlap"], 128)

	if len(input.CrawledPages) > 0 {
		return s.runFromCrawled(ctx, input.CrawledPages, rules, chunkSize, chunkOverlap, thScanner, verify, results, progress)
	}

	// 回退：无 CrawledPages 时自行抓取
	return s.runFallback(ctx, input, rules, params, chunkSize, chunkOverlap, thScanner, verify, results, progress)
}

// runFromCrawled 消费 crawler stage 已爬取的页面
func (s *Stage) runFromCrawled(
	ctx context.Context,
	pages []engine.CrawledPage,
	rules []compiledRule,
	chunkSize, chunkOverlap int,
	thScanner *TruffleHogScanner,
	verify bool,
	results chan<- *engine.ScanResult,
	progress chan<- *engine.Progress,
) (*engine.StageInput, error) {
	engineInfo := "正则"
	if thScanner != nil {
		engineInfo = "正则 + TruffleHog"
	}
	engine.SendLog(progress, StageName, "info",
		fmt.Sprintf("[sensitive] 使用爬虫产出: %d 个页面, %d 条规则, 引擎: %s, 分块: %d/%d",
			len(pages), len(rules), engineInfo, chunkSize, chunkOverlap))

	var hits int
	for _, page := range pages {
		if ctx.Err() != nil {
			break
		}
		var sb strings.Builder
		for k, v := range page.Headers {
			sb.WriteString(k)
			sb.WriteString(": ")
			sb.WriteString(v)
			sb.WriteByte('\n')
		}
		sb.Write(page.Body)
		text := sb.String()

		// 正则分块匹配
		matches := chunkMatch(text, rules, chunkSize, chunkOverlap)
		for _, m := range matches {
			r := rules[m.RuleIdx]
			matched := m.Matched
			if len(matched) > 200 {
				matched = matched[:200] + "..."
			}
			asset := &models.SensitiveAsset{
				URL:      page.URL,
				RuleID:   r.ID,
				RuleName: r.Name,
				Severity: r.Severity,
				Matched:  matched,
				Context:  extractContext(text, m.Matched, 60),
				Source:   "regex",
			}
			if s.emitResult(ctx, results, asset) {
				hits++
				engine.SendLog(progress, StageName, "info",
					fmt.Sprintf("[sensitive] 命中 %s @ %s: %s", r.Name, page.URL, truncate(matched, 80)))
			}
		}

		// TruffleHog 扫描
		if thScanner != nil {
			thHits := thScanner.Scan(ctx, page.Body, verify)
			for _, h := range thHits {
				asset := &models.SensitiveAsset{
					URL:        page.URL,
					RuleName:   h.DetectorName,
					Severity:   "critical",
					Matched:    h.Raw,
					Source:     "trufflehog",
					Verified:   h.Verified,
					DetectorID: h.DetectorID,
				}
				if s.emitResult(ctx, results, asset) {
					hits++
					vLabel := ""
					if h.Verified != nil && *h.Verified {
						vLabel = " [VERIFIED]"
					}
					engine.SendLog(progress, StageName, "info",
						fmt.Sprintf("[sensitive] TruffleHog 命中 %s @ %s: %s%s",
							h.DetectorName, page.URL, truncate(h.Raw, 60), vLabel))
				}
			}
		}
	}
	engine.SendLog(progress, StageName, "info",
		fmt.Sprintf("[sensitive] 完成 (爬虫模式), 共命中 %d 条", hits))
	return nil, nil
}

func (s *Stage) emitResult(ctx context.Context, results chan<- *engine.ScanResult, asset *models.SensitiveAsset) bool {
	res, err := engine.NewResult("sensitive", asset)
	if err != nil {
		return false
	}
	select {
	case results <- res:
		return true
	case <-ctx.Done():
		return false
	}
}

// chunkMatch 分块匹配：将 text 按 chunkSize 分块（overlap 字节重叠），对每块跑所有规则。
// 结果按 ruleIdx+matched 去重。
func chunkMatch(text string, rules []compiledRule, chunkSize, overlap int) []chunkHit {
	if len(text) == 0 || len(rules) == 0 {
		return nil
	}
	type dedupKey struct {
		ruleIdx int
		matched string
	}
	seen := make(map[dedupKey]struct{})
	var out []chunkHit

	for start := 0; start < len(text); {
		end := start + chunkSize
		if end > len(text) {
			end = len(text)
		}
		chunk := text[start:end]

		for i, r := range rules {
			m := r.Re.FindString(chunk)
			if m == "" {
				continue
			}
			key := dedupKey{ruleIdx: i, matched: m}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, chunkHit{RuleIdx: i, Matched: m})
		}

		if end >= len(text) {
			break
		}
		start = end - overlap
		if start <= 0 {
			start = end
		}
	}
	return out
}

// ── 回退路径（无 CrawledPages 时自行抓取+爬取） ────────────────────────────────

func (s *Stage) runFallback(
	ctx context.Context,
	input *engine.StageInput,
	rules []compiledRule,
	params map[string]string,
	chunkSize, chunkOverlap int,
	thScanner *TruffleHogScanner,
	verify bool,
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
		engine.SendLog(progress, StageName, "warn", "[sensitive] 无 HTTP 目标，跳过")
		return nil, nil
	}
	if before := len(urls); true {
		urls = engine.FilterURLs(urls)
		if diff := before - len(urls); diff > 0 {
			engine.SendLog(progress, StageName, "info", fmt.Sprintf("[sensitive] URL 去重: %d → %d (去除 %d 个重复模式)", before, len(urls), diff))
		}
	}
	threads := parseInt(params["threads"], 20)
	timeout := time.Duration(parseInt(params["timeout"], 10)) * time.Second
	maxBody := int64(parseInt(params["max_body_kb"], 512)) * 1024

	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
			MaxIdleConns:      200,
			IdleConnTimeout:   30 * time.Second,
			DisableKeepAlives: false,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	engine.SendLog(progress, StageName, "info",
		fmt.Sprintf("[sensitive] 开始扫描: %d 目标, %d 条规则, %d 线程 (回退模式, 含深度爬取)", len(urls), len(rules), threads))

	var visited sync.Map
	var visitedHashes sync.Map
	urlChan := make(chan string, 10000)
	var active sync.WaitGroup
	var hits int
	var mu sync.Mutex

	maxCrawled := int32(len(urls) * 50)
	if maxCrawled > 5000 {
		maxCrawled = 5000
	}
	var totalCrawled int32

	for _, u := range urls {
		visited.Store(u, true)
		atomic.AddInt32(&totalCrawled, 1)
		active.Add(1)
		urlChan <- u
	}

	go func() {
		active.Wait()
		close(urlChan)
	}()

	var wg sync.WaitGroup
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range urlChan {
				if ctx.Err() != nil {
					active.Done()
					continue
				}
				body := fetchBody(ctx, client, u, maxBody)
				if body != "" {
					hash := md5.Sum([]byte(body))
					hashStr := hex.EncodeToString(hash[:])
					if _, loaded := visitedHashes.LoadOrStore(hashStr, true); loaded {
						active.Done()
						continue
					}

					// 正则分块匹配
					matches := chunkMatch(body, rules, chunkSize, chunkOverlap)
					for _, m := range matches {
						r := rules[m.RuleIdx]
						matched := m.Matched
						if len(matched) > 200 {
							matched = matched[:200] + "..."
						}
						asset := &models.SensitiveAsset{
							URL:      u,
							RuleID:   r.ID,
							RuleName: r.Name,
							Severity: r.Severity,
							Matched:  matched,
							Context:  extractContext(body, m.Matched, 60),
							Source:   "regex",
						}
						res, err := engine.NewResult("sensitive", asset)
						if err == nil {
							select {
							case results <- res:
								mu.Lock()
								hits++
								mu.Unlock()
								engine.SendLog(progress, StageName, "info",
									fmt.Sprintf("[sensitive] 命中 %s @ %s: %s", r.Name, u, truncate(matched, 80)))
							case <-ctx.Done():
							}
						}
					}

					// TruffleHog
					if thScanner != nil {
						thHits := thScanner.Scan(ctx, []byte(body), verify)
						for _, h := range thHits {
							asset := &models.SensitiveAsset{
								URL:        u,
								RuleName:   h.DetectorName,
								Severity:   "critical",
								Matched:    h.Raw,
								Source:     "trufflehog",
								Verified:   h.Verified,
								DetectorID: h.DetectorID,
							}
							res, err := engine.NewResult("sensitive", asset)
							if err == nil {
								select {
								case results <- res:
									mu.Lock()
									hits++
									mu.Unlock()
								case <-ctx.Done():
								}
							}
						}
					}

					extracted := extractURLs(body, u)
					for _, nextURL := range extracted {
						if _, loaded := visited.LoadOrStore(nextURL, true); !loaded {
							if atomic.AddInt32(&totalCrawled, 1) <= maxCrawled {
								active.Add(1)
								select {
								case urlChan <- nextURL:
								case <-ctx.Done():
									active.Done()
									return
								}
							}
						}
					}
				}

				active.Done()

				select {
				case progress <- &engine.Progress{Stage: StageName, Percent: 0, Message: u}:
				default:
				}
			}
		}()
	}

	wg.Wait()
	engine.SendLog(progress, StageName, "info", fmt.Sprintf("[sensitive] 完成, 爬取 %d 个页面, 共命中 %d 条", atomic.LoadInt32(&totalCrawled), hits))
	return nil, nil
}

// ── 辅助函数 ────────────────────────────────────────────────────────────────

var (
	hrefRe = regexp.MustCompile(`href=["']?([^"'>\s]+)`)
	srcRe  = regexp.MustCompile(`src=["']?([^"'>\s]+)`)
)

func extractURLs(body string, baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}
	var out []string
	extract := func(re *regexp.Regexp) {
		matches := re.FindAllStringSubmatch(body, -1)
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			link := strings.TrimSpace(m[1])
			if strings.HasPrefix(link, "javascript:") || strings.HasPrefix(link, "mailto:") || strings.HasPrefix(link, "data:") {
				continue
			}
			parsed, err := url.Parse(link)
			if err != nil {
				continue
			}
			resolved := base.ResolveReference(parsed)
			if resolved.Host == base.Host {
				resolved.Fragment = ""
				out = append(out, resolved.String())
			}
		}
	}
	extract(hrefRe)
	extract(srcRe)
	return out
}

func fetchBody(ctx context.Context, client *http.Client, url string, maxBytes int64) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "nscan-sensitive/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, maxBytes))

	var sb strings.Builder
	for _, k := range []string{"Server", "X-Powered-By", "Set-Cookie", "Authorization", "WWW-Authenticate"} {
		if v := resp.Header.Get(k); v != "" {
			sb.WriteString(k)
			sb.WriteString(": ")
			sb.WriteString(v)
			sb.WriteByte('\n')
		}
	}
	sb.Write(buf)
	return sb.String()
}

func extractContext(text, match string, window int) string {
	idx := strings.Index(text, match)
	if idx < 0 {
		return ""
	}
	start := idx - window
	if start < 0 {
		start = 0
	}
	end := idx + len(match) + window
	if end > len(text) {
		end = len(text)
	}
	ctx := text[start:end]
	ctx = strings.ReplaceAll(ctx, "\n", " ")
	ctx = strings.ReplaceAll(ctx, "\r", " ")
	ctx = strings.TrimSpace(ctx)
	if len(ctx) > 300 {
		ctx = ctx[:300] + "..."
	}
	return ctx
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	var n int
	fmt.Sscanf(s, "%d", &n)
	if n <= 0 {
		return def
	}
	return n
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
