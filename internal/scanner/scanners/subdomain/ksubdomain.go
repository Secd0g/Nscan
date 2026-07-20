package subdomain

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"

	dnsresolver "github.com/yourname/nscan/internal/scanner/dns"
)

type ksubdomainCollector struct {
	path           string
	wordlistLines  string
	band           string
	resolverConfig string
}

func (c *ksubdomainCollector) Name() string { return "ksubdomain" }

// detectOutboundInterface 自动探测有外网 IPv4 地址的第一块非回环网卡
func detectOutboundInterface() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil && ip.IsGlobalUnicast() {
				return iface.Name
			}
		}
	}
	return ""
}

func (c *ksubdomainCollector) Collect(ctx context.Context, domain string) ([]string, error) {
	if c.wordlistLines == "" {
		return nil, fmt.Errorf("ksubdomain: wordlist is empty")
	}
	if c.band == "" {
		c.band = "5"
	}

	// 先尝试 ksubdomain 高速引擎
	results, err := c.runKsubdomain(ctx, domain)
	if err != nil {
		// ksubdomain 失败（权限/未安装/网卡问题）→ fallback 到纯 DNS 爆破
		fmt.Fprintf(os.Stderr, "[ksubdomain] 高速引擎失败: %v, 切换到 DNS 爆破模式\n", err)
		return c.fallbackDNSBrute(ctx, domain)
	}
	return results, nil
}

// runKsubdomain 调用 ksubdomain 二进制执行无状态 DNS 爆破
func (c *ksubdomainCollector) runKsubdomain(ctx context.Context, domain string) ([]string, error) {
	// 自动探测外网网卡
	eth := detectOutboundInterface()

	// 1. 临时字典文件
	dictFile, err := os.CreateTemp("", "ksubdomain_dict_*.txt")
	if err != nil {
		return nil, fmt.Errorf("create temp dict: %w", err)
	}
	defer os.Remove(dictFile.Name())
	if _, err := dictFile.WriteString(c.wordlistLines); err != nil {
		return nil, fmt.Errorf("write temp dict: %w", err)
	}
	dictFile.Close()

	// 2. 临时输出文件
	outFile, err := os.CreateTemp("", "ksubdomain_out_*.txt")
	if err != nil {
		return nil, fmt.Errorf("create temp out: %w", err)
	}
	outFile.Close()
	defer os.Remove(outFile.Name())

	// 3. 组装命令
	bandArg := c.band
	if !strings.HasSuffix(strings.ToLower(bandArg), "m") {
		bandArg += "m"
	}
	args := []string{
		"enum",
		"-d", domain,
		"-f", dictFile.Name(),
		"-b", bandArg,
		"--not-print",
		"--oy", "txt",
		"-o", outFile.Name(),
	}
	if eth != "" {
		args = append(args, "-e", eth)
	}

	fmt.Fprintf(os.Stderr, "[ksubdomain] 执行命令: %s %v (eth=%s)\n", c.path, args, eth)
	cmd := exec.CommandContext(ctx, c.path, args...)
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("exit: %w\noutput: %s", err, combined.String())
	}

	// 4. 读取结果
	outData, err := os.ReadFile(outFile.Name())
	if err != nil {
		return nil, fmt.Errorf("read output: %w", err)
	}

	var results []string
	for _, line := range strings.Split(strings.TrimSpace(string(outData)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			results = append(results, line)
		}
	}
	fmt.Fprintf(os.Stderr, "[ksubdomain] 高速引擎完成，发现 %d 个子域名\n", len(results))
	return results, nil
}

// fallbackDNSBrute 纯 Go 并发 DNS 爆破（不需要任何 root 权限）
// 利用 net.DefaultResolver.LookupHost 并发查询，50 个 goroutine 并行跑
func (c *ksubdomainCollector) fallbackDNSBrute(ctx context.Context, domain string) ([]string, error) {
	words := parseWordlines(c.wordlistLines)
	if len(words) == 0 {
		return nil, fmt.Errorf("dns brute: wordlist empty")
	}

	fmt.Fprintf(os.Stderr, "[dns_brute_fallback] 开始 DNS 爆破: %s, 字典 %d 条\n", domain, len(words))

	const concurrency = 100
	wordCh := make(chan string, concurrency)
	var mu sync.Mutex
	var results []string
	var wg sync.WaitGroup

	// 启动 worker
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for word := range wordCh {
				select {
				case <-ctx.Done():
					return
				default:
					candidate := word + "." + domain
					addrs, err := dnsresolver.Resolver(c.resolverConfig).LookupHost(ctx, candidate)
					if err != nil || len(addrs) == 0 {
						continue
					}
					mu.Lock()
					results = append(results, candidate)
					mu.Unlock()
				}
			}
		}()
	}

	// 投递任务
	for _, w := range words {
		select {
		case <-ctx.Done():
			break
		case wordCh <- w:
		}
	}
	close(wordCh)
	wg.Wait()

	fmt.Fprintf(os.Stderr, "[dns_brute_fallback] 完成，发现 %d 个子域名\n", len(results))
	return results, nil
}

// parseWordlines 把字典内容（换行分割）解析成字符串切片
func parseWordlines(content string) []string {
	var words []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(content, "\n") {
		w := strings.TrimSpace(line)
		if w == "" || strings.HasPrefix(w, "#") {
			continue
		}
		if !seen[w] {
			seen[w] = true
			words = append(words, w)
		}
	}
	return words
}
