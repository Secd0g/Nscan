package port

import (
	"bufio"
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

const StageName = "port"

var defaultPorts = []int{
	21, 22, 23, 25, 53, 80, 110, 143, 443, 445, 465, 587,
	993, 995, 1433, 1521, 3306, 3389, 5432, 5900, 6379,
	8080, 8443, 8888, 9200, 9300, 27017,
}

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

	hosts := collectHosts(input)
	if len(hosts) == 0 {
		engine.SendLog(progress, StageName, "warn", "[port] 无可扫描主机, 跳过")
		return nil, nil
	}

	ports := params["ports"]
	options := params["options"]
	var openHosts []string

	engine.SendLog(progress, StageName, "info", fmt.Sprintf("[port] 开始扫描 %d 个主机", len(hosts)))
	if path, err := exec.LookPath("naabu"); err == nil {
		engine.SendLog(progress, StageName, "info", "[port] 使用 naabu 引擎")
		openHosts = s.runNaabu(ctx, path, hosts, ports, options, params, results, progress)
	} else {
		engine.SendLog(progress, StageName, "info", "[port] naabu 未安装, 使用 TCP connect 扫描")
		openHosts = s.tcpScan(ctx, hosts, ports, params, results, progress)
	}
	engine.SendLog(progress, StageName, "info", fmt.Sprintf("[port] 扫描完成, 发现 %d 个开放端口", len(openHosts)))

	return &engine.StageInput{Hosts: openHosts}, nil
}

// naabu JSON line output
type naabuResult struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Service  string `json:"service"`
	Banner   string `json:"banner"`
}

func (s *Stage) runNaabu(
	ctx context.Context,
	path string,
	hosts []string,
	ports string,
	options string,
	params map[string]string,
	results chan<- *engine.ScanResult,
	progress chan<- *engine.Progress,
) []string {
	args := []string{"-silent", "-json"}
	for _, h := range hosts {
		args = append(args, "-host", h)
	}
	if ports != "" {
		args = append(args, "-p", ports)
	} else {
		portList := make([]string, len(defaultPorts))
		for i, p := range defaultPorts {
			portList[i] = strconv.Itoa(p)
		}
		args = append(args, "-p", strings.Join(portList, ","))
	}

	optSet := parseCSV(options)
	wantService := optSet["service"]
	wantBanner := optSet["banner"]
	if optSet["skip_host_discovery"] {
		args = append(args, "-Pn")
	}
	if rate := params["rate"]; rate != "" {
		args = append(args, "-rate", rate)
	}
	if scanType := params["scan_type"]; scanType == "connect" {
		args = append(args, "-scan-type", "c")
	}

	cmd := exec.CommandContext(ctx, path, args...)
	if proxy := params["global_proxy"]; proxy != "" {
		cmd.Env = append(os.Environ(),
			"HTTP_PROXY="+proxy,
			"HTTPS_PROXY="+proxy,
			"ALL_PROXY="+proxy,
		)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.log.Warn("naabu stdout pipe failed", zap.Error(err))
		return nil
	}
	if err := cmd.Start(); err != nil {
		s.log.Warn("naabu start failed", zap.Error(err))
		return nil
	}

	type portEntry struct {
		ip       string
		port     int
		protocol string
	}
	var openHosts []string
	var discovered []portEntry
	sc := bufio.NewScanner(stdout)
	// naabu -json 单行携带 host/port/protocol/service/banner，banner 可能较大；放大缓冲。
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	total := len(hosts)
	done := 0

	for sc.Scan() {
		if ctx.Err() != nil {
			cmd.Process.Kill()
			break
		}
		var nr naabuResult
		if err := json.Unmarshal(sc.Bytes(), &nr); err != nil {
			continue
		}
		discovered = append(discovered, portEntry{ip: nr.IP, port: nr.Port, protocol: nr.Protocol})
		openHosts = append(openHosts, fmt.Sprintf("%s:%d", nr.IP, nr.Port))
		done++
		pct := int32(done * 100 / (total * len(defaultPorts)))
		if pct > 100 {
			pct = 100
		}
		select {
		case progress <- &engine.Progress{Stage: StageName, Percent: pct, Message: fmt.Sprintf("%s:%d", nr.IP, nr.Port)}:
		default:
		}
	}
	_ = cmd.Wait()
	s.log.Info("naabu scan done", zap.Int("open_ports", len(discovered)))

	if (wantService || wantBanner) && len(discovered) > 0 {
		engine.SendLog(progress, StageName, "info", fmt.Sprintf("[port] 开始服务识别, %d 个端口", len(discovered)))
	}

	for _, pe := range discovered {
		service, banner := "", ""
		if wantService || wantBanner {
			service, banner = grabBanner(ctx, pe.ip, pe.port, 3*time.Second)
		}
		asset := &models.PortAsset{
			IP:       pe.ip,
			Port:     pe.port,
			Protocol: pe.protocol,
			State:    "open",
			Service:  service,
			Banner:   banner,
			Sources: []string{"naabu"},
		}
		r, _ := engine.NewResult("port", asset)
		select {
		case results <- r:
		case <-ctx.Done():
		}
	}
	return openHosts
}

// tcpScan 是 naabu 不存在时的 fallback（并发 TCP connect）
func (s *Stage) tcpScan(
	ctx context.Context,
	hosts []string,
	portsParam string,
	params map[string]string,
	results chan<- *engine.ScanResult,
	progress chan<- *engine.Progress,
) []string {
	ports := parsePorts(portsParam)
	rate := parseRate(params["rate"], 200)
	timeout := parseTimeout(params["timeout"], 2*time.Second)

	var openHosts []string
	total := len(hosts) * len(ports)
	done := 0

	sem := make(chan struct{}, rate)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, host := range hosts {
		for _, port := range ports {
			if ctx.Err() != nil {
				return openHosts
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(h string, p int) {
				defer wg.Done()
				defer func() { <-sem }()

				if isOpen(ctx, h, p, timeout) {
					asset := &models.PortAsset{IP: h, Port: p, Protocol: "tcp", State: "open", Sources: []string{"tcp"}}
					r, _ := engine.NewResult("port", asset)
					select {
					case results <- r:
					case <-ctx.Done():
					}
					mu.Lock()
					openHosts = append(openHosts, fmt.Sprintf("%s:%d", h, p))
					mu.Unlock()
				}
				mu.Lock()
				done++
				pct := int32(done * 100 / total)
				mu.Unlock()
				select {
				case progress <- &engine.Progress{Stage: StageName, Percent: pct}:
				default:
				}
			}(host, port)
		}
	}
	wg.Wait()
	return openHosts
}

func isOpen(ctx context.Context, host string, port int, timeout time.Duration) bool {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func collectHosts(input *engine.StageInput) []string {
	seen := make(map[string]struct{})
	var hosts []string
	add := func(h string) {
		if _, ok := seen[h]; !ok {
			seen[h] = struct{}{}
			hosts = append(hosts, h)
		}
	}
	for _, t := range input.Targets {
		if net.ParseIP(t) != nil {
			add(t)
		}
		if strings.Contains(t, "/") {
			ips, _ := expandCIDR(t)
			for _, ip := range ips {
				add(ip)
			}
		}
	}
	for _, sub := range input.Subdomains {
		add(sub)
	}
	return hosts
}

func expandCIDR(cidr string) ([]string, error) {
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	var ips []string
	for ip := ip.Mask(network.Mask); network.Contains(ip); incrementIP(ip) {
		ips = append(ips, ip.String())
		if len(ips) > 65536 {
			break
		}
	}
	return ips, nil
}

func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] != 0 {
			break
		}
	}
}

func parsePorts(s string) []int {
	if s == "" {
		return defaultPorts
	}
	var ports []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			start, _ := strconv.Atoi(bounds[0])
			end, _ := strconv.Atoi(bounds[1])
			for p := start; p <= end && p <= 65535; p++ {
				ports = append(ports, p)
			}
		} else if p, err := strconv.Atoi(part); err == nil {
			ports = append(ports, p)
		}
	}
	return ports
}

func parseCSV(s string) map[string]bool {
	m := make(map[string]bool)
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			m[v] = true
		}
	}
	return m
}

func parseRate(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return def
}

func parseTimeout(s string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return def
}

func grabBanner(ctx context.Context, host string, port int, timeout time.Duration) (service, banner string) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return guessService(port), ""
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(timeout))

	probes := map[int][]byte{
		80: []byte("GET / HTTP/1.0\r\nHost: " + host + "\r\n\r\n"),
		443: []byte("GET / HTTP/1.0\r\nHost: " + host + "\r\n\r\n"),
		8080: []byte("GET / HTTP/1.0\r\nHost: " + host + "\r\n\r\n"),
		8443: []byte("GET / HTTP/1.0\r\nHost: " + host + "\r\n\r\n"),
		8888: []byte("GET / HTTP/1.0\r\nHost: " + host + "\r\n\r\n"),
	}
	if probe, ok := probes[port]; ok {
		conn.Write(probe)
	}

	buf := make([]byte, 1024)
	n, _ := conn.Read(buf)
	if n > 0 {
		banner = strings.TrimSpace(string(buf[:n]))
		if len(banner) > 256 {
			banner = banner[:256]
		}
	}

	if svc := identifyService(banner, port); svc != "" {
		service = svc
	} else {
		service = guessService(port)
	}
	return
}

func identifyService(banner string, port int) string {
	bl := strings.ToLower(banner)
	switch {
	case strings.HasPrefix(banner, "HTTP/"):
		return "http"
	case strings.HasPrefix(banner, "SSH-"):
		return "ssh"
	case strings.HasPrefix(banner, "220") && strings.Contains(bl, "ftp"):
		return "ftp"
	case strings.HasPrefix(banner, "220") && strings.Contains(bl, "smtp"):
		return "smtp"
	case strings.Contains(bl, "mysql"):
		return "mysql"
	case strings.Contains(bl, "postgresql"):
		return "postgresql"
	case strings.Contains(bl, "redis"):
		return "redis"
	case strings.Contains(bl, "mongodb"):
		return "mongodb"
	case strings.HasPrefix(banner, "+OK"):
		return "pop3"
	case strings.HasPrefix(banner, "* OK") && strings.Contains(bl, "imap"):
		return "imap"
	}
	return ""
}

func guessService(port int) string {
	m := map[int]string{
		21: "ftp", 22: "ssh", 23: "telnet", 25: "smtp", 53: "dns",
		80: "http", 110: "pop3", 143: "imap", 443: "https", 445: "smb",
		465: "smtps", 587: "smtp", 993: "imaps", 995: "pop3s",
		1433: "mssql", 1521: "oracle", 3306: "mysql", 3389: "rdp",
		5432: "postgresql", 5900: "vnc", 6379: "redis",
		8080: "http-proxy", 8443: "https-alt", 8888: "http-alt",
		9200: "elasticsearch", 9300: "elasticsearch", 27017: "mongodb",
	}
	if s, ok := m[port]; ok {
		return s
	}
	return ""
}

