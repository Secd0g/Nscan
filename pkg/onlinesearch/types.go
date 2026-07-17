// Package onlinesearch 封装第三方资产搜索平台的查询客户端（Fofa/Hunter/Quake/Shodan），
// 归一化为统一的 SearchResult 结构，供上层"在线搜索"页面消费。
package onlinesearch

import "context"

// SearchResult 一条资产结果的归一化视图
type SearchResult struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Host     string `json:"host,omitempty"`    // 域名/子域名
	URL      string `json:"url,omitempty"`     // 完整 URL
	Title    string `json:"title,omitempty"`   // 页面标题
	Server   string `json:"server,omitempty"`  // Server header
	Country  string `json:"country,omitempty"`
	Region   string `json:"region,omitempty"` // 省/州
	City     string `json:"city,omitempty"`
	Protocol string `json:"protocol,omitempty"` // http/https/ssh...
	Cert     string `json:"cert,omitempty"`     // 证书主体
	Banner   string `json:"banner,omitempty"`
	OS       string `json:"os,omitempty"`
	Provider string `json:"provider"` // fofa|hunter|quake|shodan
}

// SearchOptions 用户输入
type SearchOptions struct {
	Query string
	Page  int
	Size  int // 每页条数
}

// Target 扫描输入的规范化目标，交给 provider 自行组装成语法。
type Target struct {
	Kind   string // "domain" | "ip" | "cidr"
	Value  string
}

// Client 各平台需要实现的接口
type Client interface {
	Search(ctx context.Context, opts SearchOptions) (results []SearchResult, total int, err error)
	Name() string
	// BuildQuery 依据扫描目标合成本 provider 的查询语句。返回空串表示无可查目标（调用方跳过）。
	BuildQuery(targets []Target) string
}
