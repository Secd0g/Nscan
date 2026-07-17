package subdomain

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// baiduCollector 通过百度搜索引擎提取子域名
type baiduCollector struct{}

func (c *baiduCollector) Name() string { return "baidu" }

var domainRe = regexp.MustCompile(`[a-zA-Z0-9][-a-zA-Z0-9]*\.[a-zA-Z0-9][-a-zA-Z0-9.]*`)

func (c *baiduCollector) Collect(ctx context.Context, domain string) ([]string, error) {
	seen := make(map[string]struct{})
	var results []string
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// 搜索多页，每页10条
	maxPages := 5
	for page := 0; page < maxPages; page++ {
		if ctx.Err() != nil {
			break
		}
		url := fmt.Sprintf("https://www.baidu.com/s?wd=site:%s&pn=%d&rn=10", domain, page*10)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		// 从页面内容中提取所有匹配的子域名
		matches := domainRe.FindAllString(string(body), -1)
		for _, m := range matches {
			m = strings.ToLower(m)
			if strings.HasSuffix(m, "."+domain) || m == domain {
				if _, ok := seen[m]; !ok && m != domain {
					seen[m] = struct{}{}
					results = append(results, m)
				}
			}
		}

		// 短暂延迟避免被封
		select {
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			return results, nil
		}
	}
	return results, nil
}
