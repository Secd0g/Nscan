package brute

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yourname/nscan/internal/scanner/engine"
	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

const StageName = "brute"

type Stage struct {
	log *zap.Logger
}

func New(log *zap.Logger) *Stage {
	return &Stage{log: log}
}

func (s *Stage) Name() string { return StageName }

type bruteResult struct {
	Host     string
	Port     int
	Service  string
	Username string
	Password string
}

var serviceDefaultPorts = map[string]int{
	"ssh": 22, "ftp": 21, "telnet": 23,
	"mysql": 3306, "mssql": 1433, "postgresql": 5432,
	"redis": 6379, "mongodb": 27017, "smb": 445,
	"rdp": 3389, "vnc": 5900,
}

// defaultUsers/defaultPasswords: 当 params 里没提供协议专属字典时的后备。
// 正常路径下 scheduler 会把内置字典行注入 params[<proto>.users]/[<proto>.passwords]。
var defaultUsers = map[string][]string{
	"ssh":        {"root", "admin", "ubuntu", "test", "user", "deploy"},
	"ftp":        {"root", "admin", "ftp", "anonymous", "test", "www"},
	"mysql":      {"root", "admin", "mysql", "test", "dba"},
	"mssql":      {"sa", "admin", "test"},
	"postgresql": {"postgres", "admin", "test"},
	"redis":      {""},
	"mongodb":    {"admin", "root", "test"},
	"smb":        {"administrator", "admin", "guest"},
	"rdp":        {"administrator", "admin", "user", "test"},
	"telnet":     {"root", "admin", "user", "test"},
	"vnc":        {""},
}

var defaultPasswords = []string{
	"", "123456", "admin", "password", "root", "admin123", "123456789",
	"12345678", "1234", "test", "admin@123", "P@ssw0rd", "123123",
	"abc123", "111111", "000000", "qwerty", "letmein", "master",
}

// credential 一条 user:pass 凭据
type credential struct {
	user string
	pass string
}

// protoConfig 单协议的运行时配置
type protoConfig struct {
	creds         []credential
	threads       int
	timeout       time.Duration
	stopOnSuccess bool
}

func loadProtoConfig(params map[string]string, proto string) protoConfig {
	cfg := protoConfig{
		threads:       parseInt(params["threads"], 16),
		timeout:       time.Duration(parseInt(params["timeout"], 10)) * time.Second,
		stopOnSuccess: params["stop_on_success"] == "true",
	}
	// 优先使用 scheduler 注入的 credentials（user:pass 行）
	if v := params[proto+".credentials"]; v != "" {
		cfg.creds = parseCredentialLines(v)
	} else {
		// fallback: 用硬编码 users × passwords 组合
		for _, u := range defaultUsers[proto] {
			for _, p := range defaultPasswords {
				cfg.creds = append(cfg.creds, credential{user: u, pass: p})
			}
		}
	}
	return cfg
}

// parseCredentialLines 每行 "user:pass" 或 ":pass" 或 "user:"
// 允许密码内含冒号（按第一个冒号切）；空行跳过
func parseCredentialLines(s string) []credential {
	raw := strings.Split(s, "\n")
	out := make([]credential, 0, len(raw))
	for _, l := range raw {
		l = strings.TrimRight(l, "\r")
		if l == "" {
			continue
		}
		idx := strings.IndexByte(l, ':')
		if idx < 0 {
			// 没冒号：当作 user, 空密码
			out = append(out, credential{user: l})
			continue
		}
		out = append(out, credential{user: l[:idx], pass: l[idx+1:]})
	}
	return out
}

func (s *Stage) Run(
	ctx context.Context,
	input *engine.StageInput,
	params map[string]string,
	results chan<- *engine.ScanResult,
	progress chan<- *engine.Progress,
) (*engine.StageInput, error) {

	// 新的参数命名空间：brute.protocols + brute.<proto>.*
	// 兼容旧的："protocols" (合并版 hydra 插件)
	protocols := parseCSV(params["protocols"])
	if len(protocols) == 0 {
		protocols = []string{"ssh", "ftp", "mysql", "redis"}
	}

	// 每协议独立配置
	configs := make(map[string]protoConfig, len(protocols))
	maxThreads := 1
	for _, p := range protocols {
		c := loadProtoConfig(params, p)
		configs[p] = c
		if c.threads > maxThreads {
			maxThreads = c.threads
		}
	}

	targets := collectBruteTargets(input, protocols)
	if len(targets) == 0 {
		engine.SendLog(progress, StageName, "warn", "[brute] 无可爆破目标, 跳过")
		return nil, nil
	}

	engine.SendLog(progress, StageName, "info", fmt.Sprintf("[brute] 开始弱口令扫描, %d 个目标, 协议: %v", len(targets), protocols))

	sem := make(chan struct{}, maxThreads)
	var mu sync.Mutex
	var found []bruteResult
	total := len(targets)
	done := 0

	for _, t := range targets {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		go func(target bruteTarget) {
			defer func() { <-sem }()

			cfg := configs[target.service]
			checker := getChecker(target.service)
			if checker == nil {
				return
			}

			for _, cred := range cfg.creds {
				if ctx.Err() != nil {
					return
				}
				ok := checker(ctx, target.host, target.port, cred.user, cred.pass, cfg.timeout)
				if !ok {
					continue
				}
				mu.Lock()
				found = append(found, bruteResult{
					Host: target.host, Port: target.port,
					Service: target.service, Username: cred.user, Password: cred.pass,
				})
				mu.Unlock()

				engine.SendLog(progress, StageName, "info",
					fmt.Sprintf("[brute] 发现弱口令 %s://%s:%s@%s:%d", target.service, cred.user, cred.pass, target.host, target.port))

				vuln := &models.VulnAsset{
					Target:     fmt.Sprintf("%s:%d", target.host, target.port),
					TemplateID: fmt.Sprintf("brute-%s", target.service),
					Name:       fmt.Sprintf("%s 弱口令: %s/%s", strings.ToUpper(target.service), cred.user, cred.pass),
					Severity:   "high",
					MatchedAt:  fmt.Sprintf("%s://%s:%s@%s:%d", target.service, cred.user, cred.pass, target.host, target.port),
					Request:    fmt.Sprintf("Protocol: %s\nHost: %s\nPort: %d\nUsername: %s\nPassword: %s", strings.ToUpper(target.service), target.host, target.port, cred.user, cred.pass),
					Response:   fmt.Sprintf("Authentication SUCCESS\nProtocol: %s\nHost: %s:%d\nCredential: %s:%s", strings.ToUpper(target.service), target.host, target.port, cred.user, cred.pass),
				}
				r, _ := engine.NewResult("vuln", vuln)
				select {
				case results <- r:
				case <-ctx.Done():
				}

				if cfg.stopOnSuccess {
					return
				}
			}

			mu.Lock()
			done++
			pct := int32(done * 100 / total)
			mu.Unlock()
			select {
			case progress <- &engine.Progress{Stage: StageName, Percent: pct, Message: fmt.Sprintf("%s:%d", target.host, target.port)}:
			default:
			}
		}(t)
	}

	for i := 0; i < maxThreads; i++ {
		sem <- struct{}{}
	}

	engine.SendLog(progress, StageName, "info", fmt.Sprintf("[brute] 扫描完成, 发现 %d 个弱口令", len(found)))
	return nil, nil
}

type bruteTarget struct {
	host    string
	port    int
	service string
}

// collectBruteTargets 只对已经确认端口开放的 host:port 展开爆破目标。
// input.Hosts 由 port stage 填充为 "ip:port"（open）；直接匹配到协议默认端口即可。
// 原来还会遍历 input.Targets 里的 IP 对全部协议默认端口拨号 —— 那些端口大概率没开，
// 只会造成大量超时和噪声日志，已移除。
func collectBruteTargets(input *engine.StageInput, protocols []string) []bruteTarget {
	protocolSet := make(map[string]bool)
	for _, p := range protocols {
		protocolSet[p] = true
	}

	var targets []bruteTarget
	seen := make(map[string]bool)

	for _, hp := range input.Hosts {
		host, portStr, err := net.SplitHostPort(hp)
		if err != nil {
			continue
		}
		port, _ := strconv.Atoi(portStr)
		for svc, defaultPort := range serviceDefaultPorts {
			if !protocolSet[svc] {
				continue
			}
			if port == defaultPort {
				key := fmt.Sprintf("%s:%d:%s", host, port, svc)
				if !seen[key] {
					seen[key] = true
					targets = append(targets, bruteTarget{host: host, port: port, service: svc})
				}
			}
		}
	}

	return targets
}

type checkerFunc func(ctx context.Context, host string, port int, user, pass string, timeout time.Duration) bool

func getChecker(service string) checkerFunc {
	switch service {
	case "ssh":
		return checkSSH
	case "ftp":
		return checkFTP
	case "mysql":
		return checkMySQL
	case "redis":
		return checkRedis
	case "mongodb":
		return checkMongoDB
	case "postgresql":
		return checkPostgreSQL
	case "mssql":
		return checkMSSQL
	case "smb", "rdp", "telnet", "vnc":
		return checkTCP
	default:
		return nil
	}
}

func checkTCP(ctx context.Context, host string, port int, user, pass string, timeout time.Duration) bool {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return false
	}
	conn.Close()
	return false
}

func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func parseCSVSet(s string) map[string]bool {
	m := make(map[string]bool)
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			m[v] = true
		}
	}
	return m
}

func parseInt(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return def
}
