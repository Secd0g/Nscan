package onlinesearch

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Fofa 客户端。API endpoint: https://fofa.info/api/v1/search/all
// 直接用 API Key 认证（fofa 已统一 v1/v5 API，无需再分版本）。
type Fofa struct {
	Key  string
	http *http.Client
}

func NewFofa(key string) *Fofa {
	return &Fofa{
		Key:  strings.TrimSpace(key),
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (f *Fofa) Name() string { return "fofa" }

// BuildQuery 组装 fofa 查询：domain="xxx" / ip="1.2.3.4" / ip="1.2.3.0/24"，多目标 || 连接。
func (f *Fofa) BuildQuery(targets []Target) string {
	parts := make([]string, 0, len(targets))
	for _, t := range targets {
		v := strings.TrimSpace(t.Value)
		if v == "" {
			continue
		}
		switch t.Kind {
		case "domain":
			parts = append(parts, fmt.Sprintf(`domain="%s"`, v))
		case "ip", "cidr":
			parts = append(parts, fmt.Sprintf(`ip="%s"`, v))
		}
	}
	return strings.Join(parts, " || ")
}

func (f *Fofa) Search(ctx context.Context, opts SearchOptions) ([]SearchResult, int, error) {
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.Size <= 0 || opts.Size > 10000 {
		opts.Size = 20
	}
	if strings.TrimSpace(opts.Query) == "" {
		return nil, 0, fmt.Errorf("empty query")
	}
	if f.Key == "" {
		return nil, 0, fmt.Errorf("fofa 需要 API Key")
	}

	qb := base64.StdEncoding.EncodeToString([]byte(opts.Query))
	fields := "ip,port,host,title,domain,server,country_name,region,city,protocol,cert,os"
	u := &url.URL{Scheme: "https", Host: "fofa.info", Path: "/api/v1/search/all"}
	q := u.Query()
	q.Set("key", f.Key)
	q.Set("qbase64", qb)
	q.Set("fields", fields)
	q.Set("page", fmt.Sprintf("%d", opts.Page))
	q.Set("size", fmt.Sprintf("%d", opts.Size))
	u.RawQuery = q.Encode()

	body, err := f.doGet(ctx, u.String())
	if err != nil {
		return nil, 0, err
	}
	var resp struct {
		Error   bool       `json:"error"`
		Errmsg  string     `json:"errmsg"`
		Size    int        `json:"size"`
		Results [][]string `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("fofa 解析失败: %w (body=%s)", err, truncate(string(body), 400))
	}
	if resp.Error {
		return nil, 0, fmt.Errorf("fofa: %s", resp.Errmsg)
	}
	out := make([]SearchResult, 0, len(resp.Results))
	for _, row := range resp.Results {
		get := func(i int) string {
			if i < len(row) {
				return row[i]
			}
			return ""
		}
		port := 0
		fmt.Sscanf(get(1), "%d", &port)
		out = append(out, SearchResult{
			IP:       get(0),
			Port:     port,
			Host:     firstNonEmpty(get(2), get(4)),
			Title:    get(3),
			Server:   get(5),
			Country:  get(6),
			Region:   get(7),
			City:     get(8),
			Protocol: get(9),
			Cert:     truncate(get(10), 200),
			OS:       get(11),
			URL:      buildURL(get(9), firstNonEmpty(get(2), get(0)), port),
			Provider: "fofa",
		})
	}
	return out, resp.Size, nil
}

func (f *Fofa) doGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "nscan-onlinesearch/1.0")
	resp, err := f.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fofa http %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return body, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func buildURL(proto, host string, port int) string {
	if host == "" {
		return ""
	}
	scheme := proto
	if scheme == "" {
		if port == 443 {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	if (scheme == "http" && port == 80) || (scheme == "https" && port == 443) {
		return fmt.Sprintf("%s://%s", scheme, host)
	}
	if port > 0 {
		return fmt.Sprintf("%s://%s:%d", scheme, host, port)
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}
