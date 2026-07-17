package onlinesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Shodan 客户端（https://api.shodan.io）
// 注意：Shodan 分页固定 100 条/页，Size 参数会向上取整到页数；仅 Page=1 时能保证精确。
type Shodan struct {
	Key  string
	http *http.Client
}

func NewShodan(key string) *Shodan {
	return &Shodan{
		Key:  strings.TrimSpace(key),
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *Shodan) Name() string { return "shodan" }

// BuildQuery 组装 Shodan 查询：hostname:xxx / ip:1.2.3.4 / net:1.2.3.0/24，多目标空格 OR。
// Shodan 语法用空格拼接即 OR；CIDR 需用 net: 关键字。
func (s *Shodan) BuildQuery(targets []Target) string {
	parts := make([]string, 0, len(targets))
	for _, t := range targets {
		v := strings.TrimSpace(t.Value)
		if v == "" {
			continue
		}
		switch t.Kind {
		case "domain":
			parts = append(parts, fmt.Sprintf(`hostname:%s`, v))
		case "ip":
			parts = append(parts, fmt.Sprintf(`ip:%s`, v))
		case "cidr":
			parts = append(parts, fmt.Sprintf(`net:%s`, v))
		}
	}
	return strings.Join(parts, " ")
}

func (s *Shodan) Search(ctx context.Context, opts SearchOptions) ([]SearchResult, int, error) {
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.Size <= 0 {
		opts.Size = 20
	}
	if strings.TrimSpace(opts.Query) == "" {
		return nil, 0, fmt.Errorf("empty query")
	}
	if s.Key == "" {
		return nil, 0, fmt.Errorf("shodan 需要 API Key")
	}

	u := &url.URL{Scheme: "https", Host: "api.shodan.io", Path: "/shodan/host/search"}
	q := u.Query()
	q.Set("key", s.Key)
	q.Set("query", opts.Query)
	q.Set("page", fmt.Sprintf("%d", opts.Page))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "nscan-onlinesearch/1.0")

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, 0, fmt.Errorf("shodan http %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var envelope struct {
		Total   int    `json:"total"`
		Error   string `json:"error,omitempty"`
		Matches []struct {
			IPStr     string   `json:"ip_str"`
			Port      int      `json:"port"`
			Hostnames []string `json:"hostnames"`
			Domains   []string `json:"domains"`
			Transport string   `json:"transport"` // tcp/udp
			OS        string   `json:"os,omitempty"`
			Product   string   `json:"product,omitempty"`
			Data      string   `json:"data"` // banner
			HTTP      *struct {
				Host       string `json:"host"`
				Title      string `json:"title"`
				Server     string `json:"server"`
				StatusCode int    `json:"status"`
			} `json:"http,omitempty"`
			SSL *struct {
				Cert struct {
					Subject struct {
						CN string `json:"CN"`
					} `json:"subject"`
				} `json:"cert"`
			} `json:"ssl,omitempty"`
			Location struct {
				CountryName string `json:"country_name"`
				RegionCode  string `json:"region_code"`
				City        string `json:"city"`
			} `json:"location"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, 0, fmt.Errorf("shodan 解析失败: %w (body=%s)", err, truncate(string(body), 400))
	}
	if envelope.Error != "" {
		return nil, 0, fmt.Errorf("shodan: %s", envelope.Error)
	}

	// Shodan 每页固定 100 条；按 opts.Size 截断
	limit := opts.Size
	if limit > len(envelope.Matches) {
		limit = len(envelope.Matches)
	}
	out := make([]SearchResult, 0, limit)
	for i := 0; i < limit; i++ {
		r := envelope.Matches[i]
		host := ""
		if r.HTTP != nil && r.HTTP.Host != "" {
			host = r.HTTP.Host
		} else if len(r.Hostnames) > 0 {
			host = r.Hostnames[0]
		} else if len(r.Domains) > 0 {
			host = r.Domains[0]
		} else {
			host = r.IPStr
		}
		title, server, proto, urlStr, cert := "", "", "", "", ""
		if r.HTTP != nil {
			title = r.HTTP.Title
			server = r.HTTP.Server
			// Shodan 用 SSL 有无判断 https
			if r.SSL != nil {
				proto = "https"
				cert = r.SSL.Cert.Subject.CN
			} else {
				proto = "http"
			}
			urlStr = buildURL(proto, host, r.Port)
		} else {
			proto = r.Transport
		}
		out = append(out, SearchResult{
			IP:       r.IPStr,
			Port:     r.Port,
			Host:     host,
			URL:      urlStr,
			Title:    title,
			Server:   server,
			Country:  r.Location.CountryName,
			Region:   r.Location.RegionCode,
			City:     r.Location.City,
			Protocol: proto,
			Cert:     truncate(cert, 200),
			Banner:   truncate(r.Data, 500),
			OS:       r.OS,
			Provider: "shodan",
		})
	}
	return out, envelope.Total, nil
}
