package server

import (
	"context"

	"github.com/yourname/nscan/internal/server/repositories"
	"github.com/yourname/nscan/pkg/models"
	"go.uber.org/zap"
)

func ptr(f float64) *float64 { return &f }

func SeedBuiltinPlugins(ctx context.Context, repo *repositories.PluginRepo, log *zap.Logger) {
	plugins := []models.Plugin{
		subfinderPlugin(),
		ksubdomainPlugin(),
		shufflednsPlugin(),
		bbotPlugin(),
		findomainPlugin(),
		naabuPlugin(),
		httpxPlugin(),
		fingerprintPlugin(),
		nucleiPlugin(),
		ffufPlugin(),
		brutescanPlugin(),
		onlinesearchPlugin(),
		crawlerPlugin(),
		sensitivePlugin(),
	}

	// 清理已废弃的旧内置插件（含之前拆分的 7 个 brute-* 及更早的 hydra）
	obsolete := []string{
		"hydra",
		"brute-ssh", "brute-ftp", "brute-mysql", "brute-redis",
		"brute-mongodb", "brute-postgresql", "brute-mssql",
		"dirsearch",
		"dirscan", // 改名为 ffuf
	}
	for _, name := range obsolete {
		if err := repo.DeleteBuiltinByName(ctx, name); err != nil {
			log.Warn("delete obsolete plugin failed", zap.String("name", name), zap.Error(err))
		}
	}

	for i := range plugins {
		plugins[i].Builtin = true
		plugins[i].Enabled = true
		if err := repo.UpsertBuiltin(ctx, &plugins[i]); err != nil {
			log.Error("seed plugin failed", zap.String("name", plugins[i].Name), zap.Error(err))
		}
	}

	// Delete amass from db if it exists
	if err := repo.DeleteBuiltinByName(ctx, "amass"); err != nil {
		log.Warn("Failed to delete amass plugin", zap.Error(err))
	}

	log.Info("builtin plugins seeded", zap.Int("count", len(plugins)))
}

func subfinderPlugin() models.Plugin {
	return models.Plugin{
		Name:        "subfinder",
		Module:      "subdomain",
		Description: "多源子域名收集：subfinder API 聚合 + crt.sh 证书透明 + 搜索引擎 + DNS 记录。API 密钥在「插件管理 → API 配置」中设置",
		Version:     "v2.6",
		Author:      "projectdiscovery",
		Params: []models.PluginParam{
			{
				Key: "sources", Label: "启用的数据源", Type: "checkbox-group",
				Default: []string{"subfinder", "crtsh", "search_engine", "dns_record"},
				Options: []models.ParamOption{
					{Value: "subfinder", Label: "Subfinder（API 聚合）"},
					{Value: "crtsh", Label: "crt.sh（证书透明）"},
					{Value: "search_engine", Label: "搜索引擎（百度+Bing）"},
					{Value: "dns_record", Label: "DNS 记录（MX/NS/SRV 等）"},
				},
				Help: "按需勾选，Subfinder 需在「插件管理 → API 配置」中配置 API Key",
				Span: 24,
			},
			{
				Key: "threads", Label: "并发线程", Type: "number",
				Default: float64(30), Min: ptr(1), Max: ptr(200), Step: ptr(5),
				Span: 8,
			},
			{
				Key: "timeout", Label: "超时(秒)", Type: "number",
				Default: float64(30), Min: ptr(5), Max: ptr(120), Step: ptr(5),
				Span: 8,
			},
			{
				Key: "resolvers", Label: "自定义 DNS 解析器", Type: "textarea",
				Placeholder: "8.8.8.8\n1.1.1.1\n223.5.5.5",
				Help:        "每行一个，留空使用系统默认",
				Span:        24,
			},
		},
	}
}

func ksubdomainPlugin() models.Plugin {
	return models.Plugin{
		Name:        "ksubdomain",
		Module:      "subdomain",
		Description: "高速无状态子域名爆破，基于自研 DNS 发包引擎，字典来源于「子域名配置」页面",
		Version:     "v1.9",
		Author:      "boy-hack",
		Params: []models.PluginParam{
			{
				Key: "wordlist", Label: "爆破字典", Type: "dict-select",
				Default:      []string{},
				Multiple:     true,
				DictCategory: "subdomain",
				Help:         "从「字典管理」中选择子域名字典，可多选叠加",
				Span:         12,
			},
			{
				Key: "band", Label: "发包带宽(Mbps)", Type: "number",
				Default: float64(5), Min: ptr(1), Max: ptr(100), Step: ptr(1),
				Help: "根据网络环境调整，过大可能导致丢包",
				Span: 8,
			},
			{
				Key: "retry", Label: "重试次数", Type: "number",
				Default: float64(3), Min: ptr(1), Max: ptr(10),
				Span: 8,
			},
			{
				Key: "resolvers", Label: "DNS 解析器", Type: "textarea",
				Placeholder: "8.8.8.8\n1.1.1.1",
				Span:        12,
			},
			{
				Key: "verify", Label: "结果验证", Type: "switch",
				Default: true,
				Help:    "对爆破结果二次验证排除泛解析",
				Span:    8,
			},
		},
	}
}
func shufflednsPlugin() models.Plugin {
	return models.Plugin{
		Name:        "shuffledns",
		Module:      "subdomain",
		Description: "基于 massdns 的快速 DNS 解析/爆破，无字典时仅用于验证其他数据源的结果",
		Version:     "v1.0.9",
		Author:      "projectdiscovery",
		Params: []models.PluginParam{
			{
				Key: "wordlist", Label: "爆破字典", Type: "dict-select",
				Default:      []string{},
				Multiple:     true,
				DictCategory: "subdomain",
				Help:         "选择字典即启用爆破模式；留空则仅对已收集到的子域名进行解析验证",
				Span:         12,
			},
			{
				Key: "resolvers", Label: "DNS 解析器", Type: "textarea",
				Placeholder: "8.8.8.8\n1.1.1.1\n223.5.5.5\n114.114.114.114",
				Help:        "每行一个，用于 massdns 解析",
				Span:        12,
			},
		},
	}
}

func bbotPlugin() models.Plugin {
	return models.Plugin{
		Name:        "bbot",
		Module:      "subdomain",
		Description: "BBOT 模块化 OSINT 框架的 subdomain-enum 配置，速度较慢但数据全面。API 密钥在「插件管理 → API 配置」中设置",
		Version:     "v1.0",
		Author:      "blacklanternsecurity",
		Params:      []models.PluginParam{},
	}
}

func findomainPlugin() models.Plugin {
	return models.Plugin{
		Name:        "findomain",
		Module:      "subdomain",
		Description: "跨平台子域名枚举，自动利用多达 15+ 免费 API",
		Version:     "v9.0",
		Author:      "Findomain",
		Params:      []models.PluginParam{},
	}
}

func naabuPlugin() models.Plugin {
	return models.Plugin{
		Name:        "naabu",
		Module:      "port",
		Description: "快速端口扫描器，支持 SYN/CONNECT 扫描模式",
		Version:     "v2.3",
		Author:      "projectdiscovery",
		Params: []models.PluginParam{
			{
				Key: "ports_preset", Label: "端口预设", Type: "select",
				Default: "top100",
				Options: []models.ParamOption{
					{Value: "top100", Label: "Top 100 常用端口"},
					{Value: "top1000", Label: "Top 1000 端口"},
					{Value: "full", Label: "全端口（1-65535）"},
					{Value: "custom", Label: "自定义"},
				},
				Span: 12,
			},
			{
				Key: "ports", Label: "自定义端口", Type: "string",
				Placeholder: "80,443,8080-8090,3306",
				Help:        "选择「自定义」预设时生效",
				Span:        12,
			},
			{
				Key: "rate", Label: "发包速率(包/秒)", Type: "number",
				Default: float64(1000), Min: ptr(50), Max: ptr(50000), Step: ptr(100),
				Span: 8,
			},
			{
				Key: "timeout", Label: "超时(ms)", Type: "number",
				Default: float64(1000), Min: ptr(100), Max: ptr(10000), Step: ptr(100),
				Span: 8,
			},
			{
				Key: "retries", Label: "重试次数", Type: "number",
				Default: float64(2), Min: ptr(0), Max: ptr(5),
				Span: 8,
			},
			{
				Key: "scan_type", Label: "扫描方式", Type: "select",
				Default: "syn",
				Options: []models.ParamOption{
					{Value: "syn", Label: "SYN 扫描（需要 root）"},
					{Value: "connect", Label: "CONNECT 扫描"},
				},
				Span: 12,
			},
			{
				Key: "options", Label: "附加功能", Type: "checkbox-group",
				Default: []string{"service"},
				Options: []models.ParamOption{
					{Value: "service", Label: "服务版本识别"},
					{Value: "banner", Label: "Banner 抓取"},
					{Value: "skip_host_discovery", Label: "跳过主机发现"},
				},
				Span: 12,
			},
		},
	}
}

func httpxPlugin() models.Plugin {
	return models.Plugin{
		Name:        "httpx",
		Module:      "http",
		Description: "HTTP 探测与指纹识别，支持技术栈检测、截图、JARM 指纹等",
		Version:     "v1.6",
		Author:      "projectdiscovery",
		Params: []models.PluginParam{
			{
				Key: "threads", Label: "并发线程", Type: "number",
				Default: float64(50), Min: ptr(1), Max: ptr(500), Step: ptr(10),
				Span: 8,
			},
			{
				Key: "timeout", Label: "超时(秒)", Type: "number",
				Default: float64(10), Min: ptr(1), Max: ptr(60),
				Span: 8,
			},
			{
				Key: "max_redirects", Label: "最大重定向", Type: "number",
				Default: float64(3), Min: ptr(0), Max: ptr(10),
				Span: 8,
			},
			{
				Key: "probes", Label: "探测功能", Type: "checkbox-group",
				Default: []string{"title", "tech", "favicon"},
				Options: []models.ParamOption{
					{Value: "title", Label: "页面标题"},
					{Value: "tech", Label: "技术栈识别"},
					{Value: "screenshot", Label: "页面截图"},
					{Value: "favicon", Label: "Favicon 指纹"},
					{Value: "jarm", Label: "JARM 指纹"},
					{Value: "cdn", Label: "CDN 检测"},
				},
				Span: 12,
			},
			{
				Key: "filter_codes", Label: "过滤状态码", Type: "string",
				Placeholder: "404,403,503",
				Help:        "逗号分隔，匹配的状态码将被过滤",
				Span:        12,
			},
			{
				Key: "user_agent", Label: "自定义 User-Agent", Type: "string",
				Placeholder: "留空使用默认随机 UA",
				Span:        24,
			},
			{
				Key: "headers", Label: "自定义请求头", Type: "textarea",
				Placeholder: "X-Forwarded-For: 127.0.0.1\nX-Real-IP: 127.0.0.1",
				Help:        "每行 Header: Value 格式",
				Span:        24,
			},
		},
	}
}

func fingerprintPlugin() models.Plugin {
	return models.Plugin{
		Name:        "fingerprint",
		Module:      "http",
		Description: "Web 指纹识别，基于「指纹管理」中的被动/主动指纹规则进行匹配，识别目标使用的 CMS、框架、中间件等",
		Version:     "v1.0",
		Author:      "nscan",
		Params: []models.PluginParam{
			{
				Key: "mode", Label: "识别模式", Type: "select",
				Default: "passive",
				Options: []models.ParamOption{
					{Value: "passive", Label: "被动指纹（仅匹配响应）"},
					{Value: "active", Label: "主动指纹（发送探测请求）"},
					{Value: "all", Label: "全部（被动 + 主动）"},
				},
				Span: 12,
			},
			{
				Key: "threads", Label: "并发线程", Type: "number",
				Default: float64(30), Min: ptr(1), Max: ptr(200), Step: ptr(5),
				Span: 8,
			},
			{
				Key: "timeout", Label: "超时(秒)", Type: "number",
				Default: float64(10), Min: ptr(1), Max: ptr(60),
				Span: 8,
			},
			{
				Key: "options", Label: "附加选项", Type: "checkbox-group",
				Default: []string{"favicon", "header"},
				Options: []models.ParamOption{
					{Value: "favicon", Label: "Favicon Hash 匹配"},
					{Value: "header", Label: "Header 指纹匹配"},
					{Value: "body", Label: "Body 关键词匹配"},
					{Value: "cert", Label: "证书信息匹配"},
					{Value: "port", Label: "端口服务指纹"},
				},
				Span: 24,
			},
		},
	}
}

func nucleiPlugin() models.Plugin {
	return models.Plugin{
		Name:        "nuclei",
		Module:      "vuln",
		Description: "基于模板的漏洞扫描器，支持 OOB 检测、workflow 编排",
		Version:     "v3.3",
		Author:      "projectdiscovery",
		Params: []models.PluginParam{
			{
				Key: "severity", Label: "危险等级", Type: "checkbox-group",
				Default: []string{"critical", "high", "medium"},
				Options: []models.ParamOption{
					{Value: "critical", Label: "Critical"},
					{Value: "high", Label: "High"},
					{Value: "medium", Label: "Medium"},
					{Value: "low", Label: "Low"},
					{Value: "info", Label: "Info"},
				},
				Span: 12,
			},
			{
				Key: "tags", Label: "模板 Tags", Type: "string",
				Placeholder: "cve,rce,sqli,xss,lfi",
				Help:        "逗号分隔，留空使用全部模板",
				Span:        12,
			},
			{
				Key: "concurrency", Label: "并发数", Type: "number",
				Default: float64(25), Min: ptr(1), Max: ptr(200), Step: ptr(5),
				Span: 8,
			},
			{
				Key: "rate_limit", Label: "速率限制(请求/秒)", Type: "number",
				Default: float64(150), Min: ptr(10), Max: ptr(1000), Step: ptr(10),
				Span: 8,
			},
			{
				Key: "timeout", Label: "超时(秒)", Type: "number",
				Default: float64(10), Min: ptr(1), Max: ptr(60),
				Span: 8,
			},
			{
				Key: "retries", Label: "重试次数", Type: "number",
				Default: float64(1), Min: ptr(0), Max: ptr(5),
				Span: 8,
			},
			{
				Key: "templates_dir", Label: "自定义模板目录", Type: "string",
				Placeholder: "/path/to/nuclei-templates",
				Help:        "留空使用 ~/.nuclei-templates",
				Span:        16,
			},
			{
				Key: "options", Label: "附加选项", Type: "checkbox-group",
				Default: []string{"follow_redirects"},
				Options: []models.ParamOption{
					{Value: "follow_redirects", Label: "跟随重定向"},
					{Value: "stop_at_first_match", Label: "首次匹配后停止"},
					{Value: "no_interactsh", Label: "禁用 interactsh（无 OOB 检测）"},
					{Value: "headless", Label: "Headless 浏览器模式"},
				},
				Span: 24,
			},
		},
	}
}

// brutescanPlugin 单个整体弱口令扫描插件，用户在扫描模板里勾选要扫的协议。
// 字典自动使用「字典管理」里对应协议、已启用的所有字典（合并去重），无需在模板里挑选。
func brutescanPlugin() models.Plugin {
	return models.Plugin{
		Name:        "brutescan",
		Module:      "brute",
		Description: "网络协议弱口令爆破：SSH/FTP/MySQL/Redis/MongoDB/PostgreSQL/MSSQL，字典自动匹配「字典管理」中对应协议的启用字典",
		Version:     "v1.0",
		Author:      "nscan",
		Params: []models.PluginParam{
			{
				Key: "services", Label: "扫描服务", Type: "checkbox-group",
				Default: []string{"ssh", "mysql", "redis", "ftp"},
				Options: []models.ParamOption{
					{Value: "ssh", Label: "SSH"},
					{Value: "ftp", Label: "FTP"},
					{Value: "mysql", Label: "MySQL"},
					{Value: "redis", Label: "Redis"},
					{Value: "mongodb", Label: "MongoDB"},
					{Value: "postgresql", Label: "PostgreSQL"},
					{Value: "mssql", Label: "MSSQL"},
				},
				Help: "勾选要爆破的协议，字典自动从「字典管理」中按协议匹配已启用的所有字典",
				Span: 24,
			},
			{
				Key: "threads", Label: "并发线程", Type: "number",
				Default: float64(16), Min: ptr(1), Max: ptr(64), Step: ptr(1),
				Span: 8,
			},
			{
				Key: "timeout", Label: "超时(秒)", Type: "number",
				Default: float64(10), Min: ptr(1), Max: ptr(60),
				Span: 8,
			},
			{
				Key: "stop_on_success", Label: "发现即停止", Type: "switch",
				Default: false,
				Span:    8,
			},
		},
	}
}

func ffufPlugin() models.Plugin {
	return models.Plugin{
		Name:        "ffuf",
		Module:      "dir",
		Description: "高速 Web 目录与文件扫描器，基于字典进行路径爆破",
		Version:     "v2",
		Author:      "ffuf",
		Params: []models.PluginParam{
			{
				Key: "threads", Label: "并发线程", Type: "number",
				Default: float64(50), Min: ptr(1), Max: ptr(200), Step: ptr(5),
				Span: 8,
			},
			{
				Key: "timeout", Label: "超时(秒)", Type: "number",
				Default: float64(10), Min: ptr(1), Max: ptr(60),
				Span: 8,
			},
			{
				Key: "extensions", Label: "扩展名", Type: "string",
				Default:     "php,asp,aspx,jsp,html,js",
				Placeholder: "php,asp,aspx,jsp,html",
				Help:        "逗号分隔，留空使用默认扩展名",
				Span:        12,
			},
			{
				Key: "exclude_codes", Label: "排除状态码", Type: "string",
				Default:     "404,403,500,503",
				Placeholder: "404,403,500,503",
				Help:        "逗号分隔",
				Span:        12,
			},
			{
				Key: "options", Label: "附加选项", Type: "checkbox-group",
				Default: []string{},
				Options: []models.ParamOption{
					{Value: "recursive", Label: "递归扫描"},
					{Value: "follow_redirects", Label: "跟随重定向"},
					{Value: "random_agent", Label: "随机 User-Agent"},
				},
				Span: 24,
			},
			{
				Key: "wordlist", Label: "目录字典", Type: "dict-select",
				Default:      []string{},
				Multiple:     true,
				DictCategory: "directory",
				Help:         "从「字典管理」中选择目录字典，留空使用内置字典",
				Span:         12,
			},
		},
	}
}

func onlinesearchPlugin() models.Plugin {
	return models.Plugin{
		Name:        "onlinesearch",
		Module:      "search",
		Description: "在线资产搜索：任务执行时按扫描目标自动向 Fofa / Hunter / Quake / Shodan 查询（每个 provider 用各自语法自动拼装 domain/ip/cidr 条件），结果作为 http 资产整合到项目。API Key 在「API 配置」中维护",
		Version:     "v1.1",
		Author:      "nscan",
		Params: []models.PluginParam{
			{
				Key: "providers", Label: "启用的数据源", Type: "checkbox-group",
				Default: []string{"fofa"},
				Options: []models.ParamOption{
					{Value: "fofa", Label: "Fofa"},
					{Value: "hunter", Label: "Hunter"},
					{Value: "quake", Label: "Quake"},
					{Value: "shodan", Label: "Shodan"},
				},
				Help: "勾选要调用的搜索平台，需先在「API 配置」中配置 Key 并启用；查询语句按扫描目标（域名/IP/CIDR）自动生成 provider 各自的语法，无需手写",
				Span: 18,
			},
			{
				Key: "size", Label: "每源结果条数", Type: "number",
				Default: float64(50), Min: ptr(10), Max: ptr(500), Step: ptr(10),
				Span: 6,
			},
			// 注：不再暴露 "query" 参数。手动查询请去「在线搜索」页面。
			// scanner 端仍读取 params["query"] 作为向后兼容 override（老模板/任务残留数据）。
		},
	}
}

func crawlerPlugin() models.Plugin {
	return models.Plugin{
		Name:        "crawler",
		Module:      "crawler",
		Description: "爬虫：对 HTTP 资产做 BFS 深度爬取，抓取页面内容供敏感信息等下游模块消费",
		Version:     "v1.0",
		Author:      "nscan",
		Params: []models.PluginParam{
			{
				Key: "max_pages", Label: "最大页面数", Type: "number",
				Default: float64(5000), Min: ptr(100), Max: ptr(50000), Step: ptr(100),
				Help: "单次任务最多爬取的页面总数",
				Span: 8,
			},
			{
				Key: "max_depth", Label: "最大深度", Type: "number",
				Default: float64(3), Min: ptr(1), Max: ptr(10), Step: ptr(1),
				Help: "从种子 URL 开始的最大链接跟踪深度",
				Span: 8,
			},
			{
				Key: "threads", Label: "并发线程", Type: "number",
				Default: float64(20), Min: ptr(1), Max: ptr(100), Step: ptr(1),
				Span: 8,
			},
			{
				Key: "timeout", Label: "超时(秒)", Type: "number",
				Default: float64(10), Min: ptr(1), Max: ptr(60),
				Span: 8,
			},
			{
				Key: "max_body_kb", Label: "响应体上限 (KB)", Type: "number",
				Default: float64(1024), Min: ptr(64), Max: ptr(4096), Step: ptr(64),
				Help: "单个页面最多读取多少 KB",
				Span: 8,
			},
			{
				Key: "headless", Label: "Headless 渲染", Type: "switch",
				Default: false,
				Help: "启用 Headless Chrome 渲染 SPA 页面（需要系统安装 Chrome/Chromium）",
				Span: 8,
			},
		},
	}
}

// sensitivePlugin 双引擎敏感信息扫描：正则规则 + TruffleHog 700+ 密钥检测器。
// 支持分块匹配避免大 body 截断漏检，支持密钥在线验证。
func sensitivePlugin() models.Plugin {
	return models.Plugin{
		Name:        "sensitive",
		Module:      "sensitive",
		Description: "敏感信息扫描：正则 + TruffleHog 双引擎，检测 700+ 种密钥/凭据泄漏，支持分块匹配和在线验证",
		Version:     "v2.0",
		Author:      "nscan",
		Params: []models.PluginParam{
			{
				Key: "trufflehog", Label: "TruffleHog 引擎", Type: "switch",
				Default: true,
				Help: "启用 TruffleHog 700+ 内置密钥检测器（AWS/GitHub/Stripe 等）",
				Span: 8,
			},
			{
				Key: "verify", Label: "密钥在线验证", Type: "switch",
				Default: false,
				Help: "对 TruffleHog 检测到的密钥发起在线验证，确认是否仍有效（会产生外部请求）",
				Span: 8,
			},
			{
				Key: "chunk_size", Label: "分块大小 (B)", Type: "number",
				Default: float64(4096), Min: ptr(1024), Max: ptr(16384), Step: ptr(512),
				Help: "正则匹配时的分块大小，避免大 body 截断导致漏检",
				Span: 8,
			},
			{
				Key: "chunk_overlap", Label: "分块重叠 (B)", Type: "number",
				Default: float64(128), Min: ptr(32), Max: ptr(512), Step: ptr(32),
				Help: "相邻分块的重叠字节数，确保跨块边界的密钥不被遗漏",
				Span: 8,
			},
			{
				Key: "max_body_kb", Label: "响应体上限 (KB)", Type: "number",
				Default: float64(512), Min: ptr(32), Max: ptr(2048), Step: ptr(32),
				Help: "回退模式下单次响应最多读取多少 KB",
				Span: 8,
			},
			{
				Key: "threads", Label: "并发线程", Type: "number",
				Default: float64(20), Min: ptr(1), Max: ptr(100), Step: ptr(1),
				Span: 8,
			},
			{
				Key: "timeout", Label: "超时(秒)", Type: "number",
				Default: float64(10), Min: ptr(1), Max: ptr(60),
				Span: 8,
			},
		},
	}
}
