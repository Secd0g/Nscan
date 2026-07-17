package engine

import (
	"context"
	"encoding/json"
)

// ScanResult 单条扫描结果，通过 chan 流式发送给 Agent
type ScanResult struct {
	Type string // "subdomain" | "port" | "http" | "vuln" | "dir" | "sensitive"
	Data []byte // JSON 编码的具体资产
}

func NewResult(typ string, v any) (*ScanResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &ScanResult{Type: typ, Data: data}, nil
}

// Progress 扫描进度或日志行（二选一：Percent>0 或 Log 非空）
type Progress struct {
	Stage   string
	Percent int32
	Message string
	Log     string // 日志行（非空时为日志事件，忽略 Percent）
	Level   string // "info" | "warn" | "error" | "debug"
}

// CrawledPage 爬虫产出的单个页面（URL + 响应体 + 关键响应头）
type CrawledPage struct {
	URL     string
	Body    []byte
	Headers map[string]string
}

// StageInput 上一个 Stage 产生的数据作为下一个 Stage 的输入
type StageInput struct {
	// 各类资产列表，后续 stage 按需消费
	Targets      []string            // 原始目标
	Subdomains   []string            // 子域名枚举结果
	Hosts        []string            // IP:Port 列表
	HTTPURLs     []string            // 可访问的 HTTP URL 列表
	HTTPTechMap  map[string][]string // URL → 技术栈标签（httpx 探测产出，nuclei 按技术栈过滤模板）
	CrawledPages []CrawledPage       // 爬虫产出的页面，供 sensitive 等下游消费
}

// Stage 代表 Pipeline 中的一个扫描阶段
type Stage interface {
	// Name 阶段唯一标识，对应 TaskConfig.Stages 中的字符串
	Name() string

	// Run 执行扫描，将结果写入 results，进度写入 progress
	// input 是上一阶段的输出；params 是任务配置中该阶段的参数
	// 返回本阶段产生的输出（供下一阶段消费）
	Run(ctx context.Context,
		input *StageInput,
		params map[string]string,
		results chan<- *ScanResult,
		progress chan<- *Progress,
	) (*StageInput, error)
}

// Plugin 与 Stage 相同接口，运行时动态注册
type Plugin = Stage
