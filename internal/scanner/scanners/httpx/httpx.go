package httpx

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yourname/nscan/internal/scanner/engine"
	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

const StageName = "http"

var titleRe = regexp.MustCompile(`(?i)<title[^>]*>([^<]+)</title>`)

// Stage HTTP 服务探测阶段
type Stage struct {
	log     *zap.Logger
	client  *http.Client
	matcher *FingerprintMatcher
}

func New(log *zap.Logger) *Stage {
	return &Stage{
		log:     log,
		matcher: NewFingerprintMatcher(DefaultFingerprints),
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
			// 不自动跟随重定向，记录原始 URL
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (s *Stage) Name() string { return StageName }

func (s *Stage) Run(
	ctx context.Context,
	input *engine.StageInput,
	params map[string]string,
	results chan<- *engine.ScanResult,
	progress chan<- *engine.Progress,
) (*engine.StageInput, error) {

	urls := buildURLs(input)
	if len(urls) == 0 {
		engine.SendLog(progress, StageName, "warn", "[http] 无可探测 URL, 跳过")
		return nil, nil
	}
	var httpURLs []string
	techMap := make(map[string][]string)
	total := len(urls)

	engine.SendLog(progress, StageName, "info", fmt.Sprintf("[http] 开始探测 %d 个 URL", total))

	var client *http.Client = s.client
	if proxy := params["global_proxy"]; proxy != "" {
		if proxyURL, err := url.Parse(proxy); err == nil {
			tr := s.client.Transport.(*http.Transport).Clone()
			tr.Proxy = http.ProxyURL(proxyURL)
			client = &http.Client{
				Timeout:       s.client.Timeout,
				Transport:     tr,
				CheckRedirect: s.client.CheckRedirect,
			}
		}
	}

	concurrency := 20
	if v := strings.TrimSpace(params["httpx.concurrency"]); v != "" {
		if c, err := strconv.Atoi(v); err == nil && c > 0 {
			concurrency = c
		}
	}
	engine.SendLog(progress, StageName, "info", fmt.Sprintf("[http] 单目标并发: %d", concurrency))

	perTargetTimeout := 10 * time.Second
	if v := strings.TrimSpace(params["httpx.per_target_timeout"]); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			perTargetTimeout = d
		} else if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perTargetTimeout = time.Duration(n) * time.Second
		}
	}
	engine.SendLog(progress, StageName, "info", fmt.Sprintf("[http] 单目标超时: %s", perTargetTimeout))

	var mu sync.Mutex
	var done atomic.Int32

	opts := engine.PoolOptions{
		Concurrency:    concurrency,
		PerItemTimeout: perTargetTimeout,
	}

	doScreenshot := false
	if probes := params["probes"]; strings.Contains(probes, "screenshot") {
		doScreenshot = true
	}

	engine.RunPool(ctx, urls, opts, func(tctx context.Context, rawURL string) error {
		asset, err := s.probe(tctx, client, rawURL)
		if err != nil {
			s.log.Debug("[http] probe failed", zap.String("url", rawURL), zap.Error(err))
			engine.SendLog(progress, StageName, "debug", fmt.Sprintf("[http] %s 失败: %v", rawURL, err))
			done.Add(1)
			return nil
		}
		if doScreenshot {
			asset.ScreenshotPNG = captureScreenshot(tctx, rawURL)
		}

		r, _ := engine.NewResult("http", asset)
		select {
		case results <- r:
		case <-tctx.Done():
			return tctx.Err()
		}
		
		mu.Lock()
		httpURLs = append(httpURLs, rawURL)
		if len(asset.Tech) > 0 {
			techMap[rawURL] = asset.Tech
		}
		mu.Unlock()
		
		engine.SendLog(progress, StageName, "info", fmt.Sprintf("[http] %s [%d] %s", rawURL, asset.StatusCode, asset.Title))

		d := done.Add(1)
		pct := d * 100 / int32(total)
		select {
		case progress <- &engine.Progress{Stage: StageName, Percent: pct, Message: rawURL}:
		default:
		}
		
		return nil
	})
	engine.SendLog(progress, StageName, "info", fmt.Sprintf("[http] 探测完成, %d/%d 存活", len(httpURLs), total))

	return &engine.StageInput{HTTPURLs: httpURLs, HTTPTechMap: techMap}, nil
}

func (s *Stage) probe(ctx context.Context, client *http.Client, rawURL string) (*models.HTTPAsset, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, err
	}

	title := extractTitle(string(body))
	tech := s.detectTech(resp.Header, string(body))
	banner := resp.Header.Get("Server")

	// 解析 URL 提取 domain / ip / port
	domain, ip, port := parseURLMeta(rawURL)

	// 规范化：http://host:443 → https://host:443，避免与标准 https 记录产生两条不同 URL
	savedURL := normalizeURL(rawURL)

	return &models.HTTPAsset{
		URL:        savedURL,
		Domain:     domain,
		IP:         ip,
		Port:       port,
		StatusCode: resp.StatusCode,
		Title:      title,
		Tech:       tech,
		Banner:     banner,
		ContentLen: resp.ContentLength,
		Source:     "httpx",
	}, nil
}

// captureScreenshot takes a screenshot of a URL using Chrome/Chromium headless.
// Returns nil if Chrome is not installed or capture fails.
func captureScreenshot(ctx context.Context, rawURL string) []byte {
	chrome := findChrome()
	if chrome == "" {
		return nil
	}
	tctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(tctx, chrome,
		"--headless=new",
		"--no-sandbox",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--disable-software-rasterizer",
		"--window-size=1280,800",
		"--screenshot=/dev/stdout",
		"--virtual-time-budget=5000",
		rawURL,
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil
	}
	data := buf.Bytes()
	if len(data) < 100 {
		return nil
	}
	return data
}

func findChrome() string {
	candidates := []string{"google-chrome", "chromium-browser", "chromium", "google-chrome-stable", "chrome"}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}

// parseURLMeta 从 URL 提取域名、IP（若 host 是 IP）、端口
func parseURLMeta(rawURL string) (domain, ip string, port int) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	host := u.Hostname()
	portStr := u.Port()

	if portStr == "" {
		if u.Scheme == "https" {
			port = 443
		} else {
			port = 80
		}
	} else {
		port, _ = strconv.Atoi(portStr)
	}

	if net.ParseIP(host) != nil {
		ip = host
	} else {
		domain = host
	}
	return
}

func buildURLs(input *engine.StageInput) []string {
	var urls []string
	seen := make(map[string]struct{})

	add := func(u string) {
		if _, ok := seen[u]; !ok {
			seen[u] = struct{}{}
			urls = append(urls, u)
		}
	}

	addHost := func(host string) {
		// 如果 host 是 IP:Port 形式，直接构建 http/https URL
		// 如果端口是 443/8443 等，优先构建 https
		h, p, err := net.SplitHostPort(host)
		if err == nil {
			switch p {
			case "443", "8443", "4443":
				// 优先 https，但也探 http，以便更新历史上以 http:// 存储的同端口记录
				add(fmt.Sprintf("https://%s:%s", h, p))
				add(fmt.Sprintf("http://%s:%s", h, p))
			case "80", "8080", "8000", "8888":
				add(fmt.Sprintf("http://%s:%s", h, p))
			default:
				add(fmt.Sprintf("http://%s:%s", h, p))
				add(fmt.Sprintf("https://%s:%s", h, p))
			}
		} else {
			add(fmt.Sprintf("http://%s", host))
			add(fmt.Sprintf("https://%s", host))
		}
	}

	// 原始目标（用户直接输入的域名/IP）
	for _, t := range input.Targets {
		if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") {
			add(t)
		} else {
			addHost(t)
		}
	}
	// 在线搜索（FOFA/Hunter 等）直接返回完整 URL，直接探测
	for _, u := range input.HTTPURLs {
		add(u)
	}
	// 从端口资产构建 URL
	for _, host := range input.Hosts {
		addHost(host)
	}
	// 直接使用子域名
	for _, sub := range input.Subdomains {
		add(fmt.Sprintf("http://%s", sub))
		add(fmt.Sprintf("https://%s", sub))
	}

	return urls
}

func extractTitle(body string) string {
	m := titleRe.FindStringSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return html.UnescapeString(strings.TrimSpace(m[1]))
}

// detectTech identifies technologies via header inspection and Aho-Corasick body matching.
func (s *Stage) detectTech(header http.Header, body string) []string {
	var tech []string

	server := header.Get("Server")
	if server != "" {
		tech = append(tech, server)
	}
	if xpb := header.Get("X-Powered-By"); xpb != "" {
		tech = append(tech, xpb)
	}

	combined := body + "\n" + server
	matched := s.matcher.Match(combined)
	tech = append(tech, matched...)

	return tech
}

// normalizeURL 将 http://host:443 这类方案/端口不匹配的 URL 规范化，
// 避免同一服务被以不同 URL 存入数据库导致去重失效。
func normalizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	host := u.Hostname()
	port := u.Port()
	switch {
	case u.Scheme == "http" && (port == "443" || port == "8443" || port == "4443"):
		u.Scheme = "https"
	case u.Scheme == "https" && port == "80":
		u.Scheme = "http"
	default:
		return rawURL
	}
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	}
	return u.String()
}
