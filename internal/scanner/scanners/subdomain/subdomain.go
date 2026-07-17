package subdomain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yourname/nscan/internal/scanner/engine"
	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

const StageName = "subdomain"

type Stage struct {
	log *zap.Logger
}

func New(log *zap.Logger) *Stage {
	return &Stage{log: log}
}

func (s *Stage) Name() string { return StageName }

func (s *Stage) Run(
	ctx context.Context,
	input *engine.StageInput,
	params map[string]string,
	results chan<- *engine.ScanResult,
	progress chan<- *engine.Progress,
) (*engine.StageInput, error) {

	var subdomains []string
	total := len(input.Targets)

	for i, target := range input.Targets {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if !isDomain(target) {
			engine.SendLog(progress, StageName, "info", fmt.Sprintf("[subdomain] 跳过非域名目标: %s", target))
			continue
		}

		engine.SendLog(progress, StageName, "info", fmt.Sprintf("[subdomain] 开始枚举: %s", target))

		found := s.collectAll(ctx, target, params, progress)

		engine.SendLog(progress, StageName, "info", fmt.Sprintf("[subdomain] %s 去重后共发现 %d 个子域名", target, len(found)))

		for _, item := range found {
			subdomains = append(subdomains, item.domain)

			dnsCtx, dnsCancel := context.WithTimeout(ctx, 5*time.Second)
			ips, _ := net.DefaultResolver.LookupHost(dnsCtx, item.domain)
			dnsCancel()
			asset := &models.SubdomainAsset{
				Domain:  item.domain,
				IPs:     ips,
				Sources: []string{item.source},
			}
			r, _ := engine.NewResult("subdomain", asset)
			select {
			case results <- r:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		pct := int32((i + 1) * 100 / total)
		select {
		case progress <- &engine.Progress{Stage: StageName, Percent: pct, Message: fmt.Sprintf("processed %s", target)}:
		default:
		}
	}

	return &engine.StageInput{Subdomains: subdomains}, nil
}

type subEntry struct {
	domain string
	source string
}

// collectAll 并行运行所有数据源收集子域名，合并去重，保留首次发现的来源
func (s *Stage) collectAll(ctx context.Context, domain string, params map[string]string, progress chan<- *engine.Progress) []subEntry {
	// 解析启用的数据源（扫描模板中的 checkbox-group）
	enabledSources := make(map[string]bool)
	if src := params["sources"]; src != "" {
		for _, s := range strings.Split(src, ",") {
			enabledSources[strings.TrimSpace(s)] = true
		}
	}
	// dns_brute 由 ksubdomain 插件单独控制，与 sources 无关
	// filterParams 已经裁掉了 "subdomain." 前缀，所以直接用短 key
	if params["wordlist_lines"] != "" {
		enabledSources["dns_brute"] = true
	}

	var collectors []collector

	engine.SendLog(progress, StageName, "info", fmt.Sprintf("[subdomain] 启用的数据源: %v, provider_keys 长度: %d", enabledSources, len(params["provider_keys"])))

	if enabledSources["subfinder"] {
		if path, err := exec.LookPath("subfinder"); err == nil {
			pkJSON := params["provider_keys"]
			proxy := params["global_proxy"]
			engine.SendLog(progress, StageName, "info", fmt.Sprintf("[subfinder] 已找到二进制: %s, provider_keys: %s", path, pkJSON))
			collectors = append(collectors, &subfinderCollector{path: path, providerKeysJSON: pkJSON, globalProxy: proxy})
		} else {
			engine.SendLog(progress, StageName, "warn", "[subfinder] 未安装，跳过 API 聚合")
		}
	} else {
		engine.SendLog(progress, StageName, "info", "[subfinder] 数据源未启用，跳过")
	}

	if enabledSources["crtsh"] {
		engine.SendLog(progress, StageName, "info", "[crt.sh] 数据源已启用")
		collectors = append(collectors, &crtshCollector{})
	}

	if enabledSources["search_engine"] {
		engine.SendLog(progress, StageName, "info", "[search_engine] 数据源已启用（百度+Bing）")
		collectors = append(collectors, &baiduCollector{}, &bingCollector{})
	}

	if enabledSources["dns_record"] {
		engine.SendLog(progress, StageName, "info", "[dns_record] 数据源已启用")
		collectors = append(collectors, &dnsRecordCollector{})
	}

	if enabledSources["dns_brute"] {
		if path, err := exec.LookPath("ksubdomain"); err == nil {
			wl := params["wordlist_lines"]
			band := params["band"]
			if wl == "" {
				engine.SendLog(progress, StageName, "warn", "[ksubdomain] 未配置或未加载子域名字典，跳过 dns_brute")
			} else {
				engine.SendLog(progress, StageName, "info", fmt.Sprintf("[ksubdomain] 已找到二进制: %s, 开启无状态爆破", path))
				collectors = append(collectors, &ksubdomainCollector{
					path:          path,
					wordlistLines: wl,
					band:          band,
				})
			}
		} else {
			engine.SendLog(progress, StageName, "warn", "[ksubdomain] 未安装，跳过 dns_brute")
		}
	}

	// 并行收集
	type result struct {
		name  string
		subs  []string
		err   error
	}

	// per-collector timeout（默认 10 分钟，可通过 collector_timeout 参数覆盖）
	collectorTimeout := 10 * time.Minute
	if v := strings.TrimSpace(params["collector_timeout"]); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			collectorTimeout = d
		} else if n, err := strconv.Atoi(v); err == nil && n > 0 {
			collectorTimeout = time.Duration(n) * time.Second
		}
	}

	ch := make(chan result, len(collectors))
	var wg sync.WaitGroup

	for _, c := range collectors {
		wg.Add(1)
		engine.SendLog(progress, StageName, "info", fmt.Sprintf("[%s] 开始收集: %s", c.Name(), domain))
		go func(c collector) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, collectorTimeout)
			defer cancel()
			subs, err := c.Collect(cctx, domain)
			if cctx.Err() == context.DeadlineExceeded {
				engine.SendLog(progress, StageName, "warn", fmt.Sprintf("[%s] 收集超时(%s)，已中断", c.Name(), collectorTimeout))
			}
			ch <- result{name: c.Name(), subs: subs, err: err}
		}(c)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	// 收集结果并去重，保留首次发现的来源
	seen := make(map[string]struct{})
	var merged []subEntry

	for r := range ch {
		if r.err != nil {
			engine.SendLog(progress, StageName, "warn", fmt.Sprintf("[%s] 收集失败: %v", r.name, r.err))
			s.log.Warn("collector failed", zap.String("source", r.name), zap.Error(r.err))
			continue
		}
		newCount := 0
		for _, sub := range r.subs {
			sub = strings.ToLower(strings.TrimSpace(sub))
			if sub == "" {
				continue
			}
			if _, ok := seen[sub]; !ok {
				seen[sub] = struct{}{}
				merged = append(merged, subEntry{domain: sub, source: r.name})
				newCount++
			}
		}
		engine.SendLog(progress, StageName, "info", fmt.Sprintf("[%s] 发现 %d 个子域名（新增 %d）", r.name, len(r.subs), newCount))
	}

	return merged
}

// subfinderCollector 调用 subfinder 外部二进制
type subfinderCollector struct {
	path             string
	providerKeysJSON string
	globalProxy      string
}

func (c *subfinderCollector) Name() string { return "subfinder" }

func (c *subfinderCollector) Collect(ctx context.Context, domain string) ([]string, error) {
	args := []string{"-d", domain, "-silent"}

	// 如果有 provider API keys，写入临时 provider-config.yaml
	if c.providerKeysJSON != "" {
		cfgPath, err := c.writeProviderConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[subfinder] writeProviderConfig 失败: %v\n", err)
		} else {
			defer os.Remove(cfgPath)
			args = append(args, "-pc", cfgPath)
			// 打印生成的配置文件内容
			if data, _ := os.ReadFile(cfgPath); len(data) > 0 {
				fmt.Fprintf(os.Stderr, "[subfinder] provider-config.yaml 内容:\n%s\n", string(data))
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "[subfinder] 没有 provider_keys，将使用默认配置\n")
	}

	fmt.Fprintf(os.Stderr, "[subfinder] 执行命令: %s %v\n", c.path, args)
	cmd := exec.CommandContext(ctx, c.path, args...)
	if c.globalProxy != "" {
		cmd.Env = append(os.Environ(),
			"HTTP_PROXY="+c.globalProxy,
			"HTTPS_PROXY="+c.globalProxy,
			"ALL_PROXY="+c.globalProxy,
		)
	}
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("subfinder: %w, stderr: %s", err, stderr.String())
	}
	if stderr.Len() > 0 {
		fmt.Fprintf(os.Stderr, "[subfinder] stderr 输出: %s\n", stderr.String())
	}
	var results []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			results = append(results, line)
		}
	}
	return results, nil
}

func (c *subfinderCollector) writeProviderConfig() (string, error) {
	var providers map[string][]string
	if err := json.Unmarshal([]byte(c.providerKeysJSON), &providers); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	for name, keys := range providers {
		if len(keys) == 0 {
			continue
		}
		buf.WriteString(name + ":\n")
		for _, key := range keys {
			buf.WriteString("  - " + key + "\n")
		}
	}

	f, err := os.CreateTemp("", "subfinder-provider-*.yaml")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()
	return f.Name(), nil
}

// bruteCollector DNS 字典爆破（subfinder 不存在时的 fallback）
type bruteCollector struct {
	params map[string]string
}

func (c *bruteCollector) Name() string { return "dns-brute" }

func (c *bruteCollector) Collect(ctx context.Context, domain string) ([]string, error) {
	prefixes := []string{
		"www", "mail", "ftp", "api", "admin", "dev", "test", "staging",
		"app", "cdn", "static", "m", "mobile", "blog", "shop", "store",
		"portal", "vpn", "oa", "crm", "erp", "hr", "im", "chat",
		"git", "svn", "ci", "jenkins", "jira", "wiki", "docs",
		"ns1", "ns2", "mx", "smtp", "pop", "imap", "webmail",
		"db", "mysql", "redis", "mongo", "es", "mq", "kafka",
		"stg", "uat", "pre", "beta", "demo", "sandbox",
	}
	if custom := c.params["wordlist"]; custom != "" {
		prefixes = strings.Split(custom, ",")
	}
	var results []string
	for _, prefix := range prefixes {
		if ctx.Err() != nil {
			return results, ctx.Err()
		}
		sub := fmt.Sprintf("%s.%s", prefix, domain)
		if _, err := net.LookupHost(sub); err == nil {
			results = append(results, sub)
		}
	}
	return results, nil
}

func isDomain(target string) bool {
	if net.ParseIP(target) != nil {
		return false
	}
	if strings.Contains(target, "/") {
		return false
	}
	return strings.Contains(target, ".")
}
