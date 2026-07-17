package crawler

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/ledongthuc/pdf"
	"github.com/yourname/nscan/internal/scanner/engine"
	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

const StageName = "crawler"

type Stage struct {
	log *zap.Logger
}

func New(log *zap.Logger) *Stage { return &Stage{log: log} }

func (s *Stage) Name() string { return StageName }

type crawlItem struct {
	URL   string
	Depth int
}

func (s *Stage) Run(
	ctx context.Context,
	input *engine.StageInput,
	params map[string]string,
	results chan<- *engine.ScanResult,
	progress chan<- *engine.Progress,
) (*engine.StageInput, error) {
	maxPages := parseInt(params["max_pages"], 5000)
	maxDepth := parseInt(params["max_depth"], 3)
	threads := parseInt(params["threads"], 20)
	timeout := time.Duration(parseInt(params["timeout"], 10)) * time.Second
	maxBody := int64(parseInt(params["max_body_kb"], 1024)) * 1024
	headless := params["headless"] == "true"

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
		engine.SendLog(progress, StageName, "warn", "[crawler] 无 HTTP 目标，跳过")
		return nil, nil
	}

	urls = engine.FilterURLs(urls)

	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
			MaxIdleConns:      200,
			IdleConnTimeout:   30 * time.Second,
			DisableKeepAlives: false,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Do not silently leave the authorized target scope through a
			// redirect. The original response is still recorded.
			return http.ErrUseLastResponse
		},
	}

	// Headless: 共享一个 browser allocator
	var browserCtx context.Context
	var browserCancel context.CancelFunc
	if headless {
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("ignore-certificate-errors", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("headless", true),
		)
		allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
		browserCtx, browserCancel = chromedp.NewContext(allocCtx)
		defer allocCancel()
		defer browserCancel()
		engine.SendLog(progress, StageName, "info", "[crawler] Headless Chrome 模式已启用")
	}

	mode := "静态"
	if headless {
		mode = "Headless"
	}
	engine.SendLog(progress, StageName, "info",
		fmt.Sprintf("[crawler] 开始爬取: %d 个种子, 最大深度 %d, 最多 %d 页, %d 线程, 模式: %s",
			len(urls), maxDepth, maxPages, threads, mode))

	var (
		visited      sync.Map
		bodyHashes   sync.Map
		pagesMu      sync.Mutex
		pages        []engine.CrawledPage
		newURLs      []string
		pdfCount     int32
		totalCrawled int32
		queue        = make(chan crawlItem, 10000)
		active       sync.WaitGroup
	)

	for _, u := range urls {
		visited.Store(u, true)
		active.Add(1)
		atomic.AddInt32(&totalCrawled, 1)
		queue <- crawlItem{URL: u, Depth: 0}
	}

	go func() {
		active.Wait()
		close(queue)
	}()

	var wg sync.WaitGroup
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range queue {
				if ctx.Err() != nil {
					active.Done()
					continue
				}

				var body []byte
				var headers map[string]string

				if headless {
					body, headers = fetchPageHeadless(browserCtx, item.URL, maxBody, timeout)
				} else {
					body, headers = fetchPage(ctx, client, item.URL, maxBody)
				}

				if body == nil {
					active.Done()
					continue
				}

				// PDF 检测与提取
				ct := headers["Content-Type"]
				if strings.Contains(ct, "application/pdf") || isPDFBytes(body) {
					if text, err := extractPDFText(body); err == nil && len(text) > 0 {
						body = text
						headers["X-PDF-Extracted"] = "true"
						atomic.AddInt32(&pdfCount, 1)
					}
				}

				hash := md5.Sum(body)
				hashStr := hex.EncodeToString(hash[:])
				if _, loaded := bodyHashes.LoadOrStore(hashStr, true); loaded {
					active.Done()
					continue
				}

				pagesMu.Lock()
				pages = append(pages, engine.CrawledPage{
					URL:     item.URL,
					Body:    body,
					Headers: headers,
				})
				newURLs = append(newURLs, item.URL)
				pagesMu.Unlock()

				// 发射爬虫资产到结果流
				source := "static"
				if headless {
					source = "headless"
				}
				if headers["X-PDF-Extracted"] == "true" {
					source = "pdf"
				}
				title := ""
				if m := titleRe.FindSubmatch(body); len(m) > 1 {
					title = strings.TrimSpace(string(m[1]))
				}
				asset := models.CrawlerAsset{
					URL:         item.URL,
					StatusCode:  200,
					ContentType: ct,
					ContentLen:  len(body),
					Title:       title,
					Depth:       item.Depth,
					Source:      source,
				}
				if r, err := engine.NewResult("crawler", asset); err == nil {
					select {
					case results <- r:
					default:
					}
				}

				if item.Depth < maxDepth && !strings.Contains(ct, "application/pdf") {
					extracted := extractLinks(body, item.URL)
					for _, link := range extracted {
						if _, loaded := visited.LoadOrStore(link, true); !loaded {
							if int(atomic.AddInt32(&totalCrawled, 1)) <= maxPages {
								active.Add(1)
								select {
								case queue <- crawlItem{URL: link, Depth: item.Depth + 1}:
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
				case progress <- &engine.Progress{Stage: StageName, Message: item.URL}:
				default:
				}
			}
		}()
	}

	wg.Wait()

	crawled := atomic.LoadInt32(&totalCrawled)
	pdfs := atomic.LoadInt32(&pdfCount)
	msg := fmt.Sprintf("[crawler] 完成, 爬取 %d 个页面, 保留 %d 个 (去重后)", crawled, len(pages))
	if pdfs > 0 {
		msg += fmt.Sprintf(", 提取 %d 个 PDF", pdfs)
	}
	engine.SendLog(progress, StageName, "info", msg)

	return &engine.StageInput{
		CrawledPages: pages,
		HTTPURLs:     newURLs,
	}, nil
}

// ── Headless 渲染 ───────────────────────────────────────────────────────────

func fetchPageHeadless(browserCtx context.Context, rawURL string, maxBytes int64, timeout time.Duration) ([]byte, map[string]string) {
	tabCtx, cancel := chromedp.NewContext(browserCtx)
	defer cancel()

	tabCtx, cancel = context.WithTimeout(tabCtx, timeout)
	defer cancel()

	var html string
	err := chromedp.Run(tabCtx,
		chromedp.Navigate(rawURL),
		chromedp.WaitReady("body"),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil || len(html) == 0 {
		return nil, nil
	}

	body := []byte(html)
	if int64(len(body)) > maxBytes {
		body = body[:maxBytes]
	}

	headers := map[string]string{
		"X-Rendered": "headless",
	}
	return body, headers
}

// ── PDF 提取 ────────────────────────────────────────────────────────────────

func isPDFBytes(data []byte) bool {
	return len(data) > 4 && string(data[:5]) == "%PDF-"
}

func extractPDFText(data []byte) ([]byte, error) {
	tmpFile, err := os.CreateTemp("", "nscan-pdf-*.pdf")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.Write(data); err != nil {
		return nil, err
	}
	tmpFile.Close()

	f, r, err := pdf.Open(tmpFile.Name())
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var buf bytes.Buffer
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(text)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// ── 静态抓取 ────────────────────────────────────────────────────────────────

func fetchPage(ctx context.Context, client *http.Client, rawURL string, maxBytes int64) ([]byte, map[string]string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil
	}
	req.Header.Set("User-Agent", "nscan-crawler/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if len(body) == 0 {
		return nil, nil
	}

	headers := make(map[string]string)
	for _, k := range []string{"Server", "X-Powered-By", "Set-Cookie", "Content-Type", "Authorization", "WWW-Authenticate"} {
		if v := resp.Header.Get(k); v != "" {
			headers[k] = v
		}
	}
	return body, headers
}

// ── 链接提取 ────────────────────────────────────────────────────────────────

var (
	hrefRe  = regexp.MustCompile(`href=["']?([^"'>\s]+)`)
	srcRe   = regexp.MustCompile(`src=["']?([^"'>\s]+)`)
	titleRe = regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
)

func extractLinks(body []byte, baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	var out []string
	extract := func(re *regexp.Regexp) {
		for _, m := range re.FindAllSubmatch(body, -1) {
			if len(m) < 2 {
				continue
			}
			link := strings.TrimSpace(string(m[1]))
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
