// Package search 在线资产搜索 stage：调用 Fofa/Hunter/Quake/Shodan 等外部平台，
// 结果作为 http_asset 写入项目。
package search

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/yourname/nscan/internal/scanner/engine"
	"github.com/yourname/nscan/pkg/models"
	"github.com/yourname/nscan/pkg/onlinesearch"
	"go.uber.org/zap"
)

// classifyTargets 把用户输入的扫描目标（原生字符串）归一化为 provider 可以理解的 Target。
// 支持: 域名、IPv4/IPv6、CIDR。空/非法条目跳过。
func classifyTargets(raw []string) []onlinesearch.Target {
	out := make([]onlinesearch.Target, 0, len(raw))
	for _, t := range raw {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		// 去掉可能带的 http(s):// 前缀和端口/路径
		t = strings.TrimPrefix(t, "http://")
		t = strings.TrimPrefix(t, "https://")
		if i := strings.IndexAny(t, "/:"); i >= 0 {
			// 特殊情况：CIDR 里带 /；先按 CIDR 试探
			if _, _, err := net.ParseCIDR(t); err == nil {
				out = append(out, onlinesearch.Target{Kind: "cidr", Value: t})
				continue
			}
			t = t[:i]
		}
		if net.ParseIP(t) != nil {
			out = append(out, onlinesearch.Target{Kind: "ip", Value: t})
			continue
		}
		if isDomain(t) {
			out = append(out, onlinesearch.Target{Kind: "domain", Value: t})
		}
	}
	return out
}

func isDomain(s string) bool {
	if s == "" || strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") {
		return false
	}
	if !strings.Contains(s, ".") {
		return false
	}
	for _, r := range s {
		if !(r == '-' || r == '.' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return true
}

// normalizeHost 把 fofa/quake 等返回的 host 字段规整成裸域名：
//   "https://mail.x.com"     → "mail.x.com"
//   "mail.x.com:8443"        → "mail.x.com"
//   "MAIL.X.COM/path?q=1"    → "mail.x.com"
// 剥不掉的（IPv6、乱码）交给 isDomain 兜底拒绝。
func normalizeHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	// IPv6 字面量含多个冒号，跳过端口剥离（保持原样，交给 isDomain 拒绝）
	if strings.Count(h, ":") == 1 {
		h = h[:strings.Index(h, ":")]
	}
	return h
}

const StageName = "search"

type Stage struct {
	log *zap.Logger
}

func New(log *zap.Logger) *Stage { return &Stage{log: log} }

func (s *Stage) Name() string { return StageName }

func (s *Stage) Run(
	ctx context.Context,
	input *engine.StageInput,
	params map[string]string,
	results chan<- *engine.ScanResult,
	progress chan<- *engine.Progress,
) (*engine.StageInput, error) {
	// providers: 用户勾选的数据源
	providers := parseCSV(params["providers"])
	if len(providers) == 0 {
		providers = []string{"fofa"}
	}
	size := parseInt(params["size"], 50)

	// 用户可选：自定义查询覆盖（advanced）。留空则完全自动。
	overrideQuery := strings.TrimSpace(params["query"])

	// 归一化扫描目标 —— 每个 provider 根据自己语法拼查询。
	structured := classifyTargets(input.Targets)
	if overrideQuery == "" && len(structured) == 0 {
		engine.SendLog(progress, StageName, "warn", "[search] 无可用扫描目标（域名/IP/CIDR），跳过")
		return nil, nil
	}
	engine.SendLog(progress, StageName, "info",
		fmt.Sprintf("[search] 开始查询: providers=%v, 目标 %d 条 (自动生成 provider 语法)", providers, len(structured)))

	out := &engine.StageInput{Targets: input.Targets}
	totalHits := 0
	for _, name := range providers {
		if ctx.Err() != nil {
			break
		}
		client, err := buildClient(name, params)
		if err != nil {
			engine.SendLog(progress, StageName, "warn", fmt.Sprintf("[search] 跳过 %s: %v", name, err))
			continue
		}
		// 每个 provider 用自己的语法拼；用户显式提供了 override 时优先用 override。
		query := overrideQuery
		if query == "" {
			query = client.BuildQuery(structured)
		}
		if query == "" {
			engine.SendLog(progress, StageName, "warn", fmt.Sprintf("[search] %s 无法为当前目标构建查询，跳过", name))
			continue
		}
		engine.SendLog(progress, StageName, "info", fmt.Sprintf("[search] %s: %s", name, truncate(query, 200)))
		list, total, err := client.Search(ctx, onlinesearch.SearchOptions{Query: query, Page: 1, Size: size})
		if err != nil {
			engine.SendLog(progress, StageName, "error", fmt.Sprintf("[search] %s 查询失败: %v", name, err))
			continue
		}
		engine.SendLog(progress, StageName, "info",
			fmt.Sprintf("[search] %s 返回 %d 条 (total=%d)", name, len(list), total))
		for _, r := range list {
			if ctx.Err() != nil {
				break
			}
			// 1. HTTP 资产（仅当有 URL 时；tcp/ssh/etc 服务不建 http 资产，避免脏数据）
			if r.URL != "" {
				httpAsset := &models.HTTPAsset{
					URL:    r.URL,
					Domain: r.Host,
					IP:     r.IP,
					Port:   r.Port,
					Title:  r.Title,
					Banner: r.Server,
					Source: r.Provider,
				}
				if res, err := engine.NewResult("http", httpAsset); err == nil {
					select {
					case results <- res:
						totalHits++
						out.HTTPURLs = append(out.HTTPURLs, r.URL)
					case <-ctx.Done():
					}
				}
			}
			// 2. 端口资产（IP + Port 齐全时写入 IP/端口库，供后续弱口令/漏扫复用）
			if r.IP != "" && r.Port > 0 {
				portAsset := &models.PortAsset{
					IP:       r.IP,
					Port:     r.Port,
					Protocol: "tcp",
					State:    "open",
					Service:  r.Protocol,
					Banner:   r.Server,
					Sources:  []string{r.Provider},
				}
				if res, err := engine.NewResult("port", portAsset); err == nil {
					select {
					case results <- res:
						out.Hosts = append(out.Hosts, fmt.Sprintf("%s:%d", r.IP, r.Port))
					case <-ctx.Done():
					}
				}
			}
			// 3. 子域名资产（Host 是域名时写入子域名库；先剥协议/端口/路径再判断，排除 IP）
			domain := normalizeHost(r.Host)
			if domain != "" && net.ParseIP(domain) == nil && isDomain(domain) {
				var ips []string
				if r.IP != "" {
					ips = []string{r.IP}
				}
				subAsset := &models.SubdomainAsset{
					Domain:  domain,
					IPs:     ips,
					Sources: []string{r.Provider},
				}
				if res, err := engine.NewResult("subdomain", subAsset); err == nil {
					select {
					case results <- res:
						out.Subdomains = append(out.Subdomains, domain)
					case <-ctx.Done():
					}
				}
			}
		}
	}
	engine.SendLog(progress, StageName, "info", fmt.Sprintf("[search] 完成, 共 %d 条资产", totalHits))
	return out, nil
}

func buildClient(name string, params map[string]string) (onlinesearch.Client, error) {
	// params 里包含 scheduler 注入的 key： search.<provider>.key
	// 但因为 filterParams("search.") 已去除前缀，所以这里读 <provider>.key
	key := strings.TrimSpace(params[name+".key"])
	if key == "" {
		return nil, fmt.Errorf("%s 未配置 API Key 或未启用", name)
	}
	switch name {
	case "fofa":
		return onlinesearch.NewFofa(key), nil
	case "hunter":
		return onlinesearch.NewHunter(key), nil
	case "quake":
		return onlinesearch.NewQuake(key), nil
	case "shodan":
		return onlinesearch.NewShodan(key), nil
	default:
		return nil, fmt.Errorf("未知 provider: %s", name)
	}
}

func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	fmt.Sscanf(s, "%d", &n)
	if n <= 0 {
		return def
	}
	return n
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
