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

// Hunter 客户端（奇安信 Hunter：https://hunter.qianxin.com）
type Hunter struct {
	Key  string
	http *http.Client
}

func NewHunter(key string) *Hunter {
	return &Hunter{
		Key:  strings.TrimSpace(key),
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (h *Hunter) Name() string { return "hunter" }

// BuildQuery 组装 Hunter 查询：domain.suffix="xxx" / ip="1.2.3.4" / ip="1.2.3.0/24"，多目标 || 连接。
func (h *Hunter) BuildQuery(targets []Target) string {
	parts := make([]string, 0, len(targets))
	for _, t := range targets {
		v := strings.TrimSpace(t.Value)
		if v == "" {
			continue
		}
		switch t.Kind {
		case "domain":
			parts = append(parts, fmt.Sprintf(`domain.suffix="%s"`, v))
		case "ip", "cidr":
			parts = append(parts, fmt.Sprintf(`ip="%s"`, v))
		}
	}
	return strings.Join(parts, " || ")
}

func (h *Hunter) Search(ctx context.Context, opts SearchOptions) ([]SearchResult, int, error) {
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.Size <= 0 || opts.Size > 100 {
		opts.Size = 20
	}
	if strings.TrimSpace(opts.Query) == "" {
		return nil, 0, fmt.Errorf("empty query")
	}
	if h.Key == "" {
		return nil, 0, fmt.Errorf("hunter 需要 API Key")
	}

	u := &url.URL{Scheme: "https", Host: "hunter.qianxin.com", Path: "/openApi/search"}
	q := u.Query()
	q.Set("api-key", h.Key)
	q.Set("search", base64.URLEncoding.EncodeToString([]byte(opts.Query)))
	q.Set("page", fmt.Sprintf("%d", opts.Page))
	q.Set("page_size", fmt.Sprintf("%d", opts.Size))
	q.Set("is_web", "3") // 1=只 Web, 2=只非 Web, 3=全部
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "nscan-onlinesearch/1.0")
	resp, err := h.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, 0, fmt.Errorf("hunter http %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Total int `json:"total"`
			Arrs  []struct {
				IsRisk      string `json:"is_risk"`
				URL         string `json:"url"`
				IP          string `json:"ip"`
				Port        int    `json:"port"`
				WebTitle    string `json:"web_title"`
				Domain      string `json:"domain"`
				Protocol    string `json:"protocol"`
				Component   []struct {
					Name    string `json:"name"`
					Version string `json:"version"`
				} `json:"component"`
				OS       string `json:"os"`
				Server   string `json:"server"`
				Country  string `json:"country"`
				Province string `json:"province"`
				City     string `json:"city"`
			} `json:"arr"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, 0, fmt.Errorf("hunter 解析失败: %w (body=%s)", err, truncate(string(body), 400))
	}
	if envelope.Code != 200 {
		return nil, 0, fmt.Errorf("hunter: %s", envelope.Message)
	}
	out := make([]SearchResult, 0, len(envelope.Data.Arrs))
	for _, r := range envelope.Data.Arrs {
		out = append(out, SearchResult{
			IP:       r.IP,
			Port:     r.Port,
			Host:     firstNonEmpty(r.Domain, r.IP),
			URL:      r.URL,
			Title:    r.WebTitle,
			Server:   r.Server,
			Country:  r.Country,
			Region:   r.Province,
			City:     r.City,
			Protocol: r.Protocol,
			OS:       r.OS,
			Provider: "hunter",
		})
	}
	return out, envelope.Data.Total, nil
}
