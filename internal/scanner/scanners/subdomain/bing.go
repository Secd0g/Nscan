package subdomain

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// bingCollector 通过 Bing 搜索引擎提取子域名
type bingCollector struct{}

func (c *bingCollector) Name() string { return "bing" }

func (c *bingCollector) Collect(ctx context.Context, domain string) ([]string, error) {
	seen := make(map[string]struct{})
	var results []string
	client := &http.Client{Timeout: 15 * time.Second}

	maxPages := 5
	for page := 0; page < maxPages; page++ {
		if ctx.Err() != nil {
			break
		}
		url := fmt.Sprintf("https://www.bing.com/search?q=site:%s&first=%d", domain, page*10+1)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

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

		select {
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			return results, nil
		}
	}
	return results, nil
}
