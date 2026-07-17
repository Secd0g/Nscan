package onlinesearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Quake 客户端（360 Quake：https://quake.360.net）
type Quake struct {
	Key  string
	http *http.Client
}

func NewQuake(key string) *Quake {
	return &Quake{
		Key:  strings.TrimSpace(key),
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (q *Quake) Name() string { return "quake" }

// BuildQuery 组装 Quake 查询：domain: "xxx" / ip: "1.2.3.4" / ip: "1.2.3.0/24"，多目标 OR 连接。
func (q *Quake) BuildQuery(targets []Target) string {
	parts := make([]string, 0, len(targets))
	for _, t := range targets {
		v := strings.TrimSpace(t.Value)
		if v == "" {
			continue
		}
		switch t.Kind {
		case "domain":
			parts = append(parts, fmt.Sprintf(`domain: "%s"`, v))
		case "ip", "cidr":
			parts = append(parts, fmt.Sprintf(`ip: "%s"`, v))
		}
	}
	return strings.Join(parts, " OR ")
}

func (q *Quake) Search(ctx context.Context, opts SearchOptions) ([]SearchResult, int, error) {
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.Size <= 0 || opts.Size > 500 {
		opts.Size = 20
	}
	if strings.TrimSpace(opts.Query) == "" {
		return nil, 0, fmt.Errorf("empty query")
	}
	if q.Key == "" {
		return nil, 0, fmt.Errorf("quake 需要 API Key")
	}

	start := (opts.Page - 1) * opts.Size
	reqBody, _ := json.Marshal(map[string]any{
		"query":        opts.Query,
		"start":        start,
		"size":         opts.Size,
		"ignore_cache": false,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://quake.360.net/api/v3/search/quake_service",
		bytes.NewReader(reqBody))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("X-QuakeToken", q.Key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "nscan-onlinesearch/1.0")

	resp, err := q.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, 0, fmt.Errorf("quake http %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var envelope struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
		Meta    struct {
			Pagination struct {
				Total int `json:"total"`
			} `json:"pagination"`
		} `json:"meta"`
		Data []struct {
			IP       string `json:"ip"`
			Port     int    `json:"port"`
			Hostname string `json:"hostname"`
			Domain   string `json:"domain"`
			Service  struct {
				Name string `json:"name"` // http, ssh, ...
				HTTP struct {
					Host   string `json:"host"`
					Title  string `json:"title"`
					Server string `json:"server"`
				} `json:"http"`
				Cert string `json:"cert"`
			} `json:"service"`
			Location struct {
				CountryEN  string `json:"country_en"`
				CountryCN  string `json:"country_cn"`
				ProvinceEN string `json:"province_en"`
				ProvinceCN string `json:"province_cn"`
				CityEN     string `json:"city_en"`
				CityCN     string `json:"city_cn"`
			} `json:"location"`
			OSName string `json:"os_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, 0, fmt.Errorf("quake 解析失败: %w (body=%s)", err, truncate(string(body), 400))
	}
	// Quake code 可能是 int 或字符串 "0"/"200"
	codeOK := false
	switch v := envelope.Code.(type) {
	case float64:
		codeOK = int(v) == 0
	case string:
		codeOK = v == "0" || v == "200"
	}
	if !codeOK {
		return nil, 0, fmt.Errorf("quake: %s", envelope.Message)
	}

	out := make([]SearchResult, 0, len(envelope.Data))
	for _, r := range envelope.Data {
		host := firstNonEmpty(r.Service.HTTP.Host, firstNonEmpty(r.Domain, r.Hostname))
		url := ""
		if r.Service.Name == "http" || r.Service.Name == "https" {
			url = buildURL(r.Service.Name, firstNonEmpty(host, r.IP), r.Port)
		}
		out = append(out, SearchResult{
			IP:       r.IP,
			Port:     r.Port,
			Host:     firstNonEmpty(host, r.IP),
			URL:      url,
			Title:    r.Service.HTTP.Title,
			Server:   r.Service.HTTP.Server,
			Country:  firstNonEmpty(r.Location.CountryCN, r.Location.CountryEN),
			Region:   firstNonEmpty(r.Location.ProvinceCN, r.Location.ProvinceEN),
			City:     firstNonEmpty(r.Location.CityCN, r.Location.CityEN),
			Protocol: r.Service.Name,
			Cert:     truncate(r.Service.Cert, 200),
			OS:       r.OSName,
			Provider: "quake",
		})
	}
	return out, envelope.Meta.Pagination.Total, nil
}
