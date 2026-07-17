package tooldef

type ToolDef struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Module      string   `json:"module"`
	InstallCmds []string `json:"install_cmds"`
}

var All = []ToolDef{
	{Name: "subfinder", Description: "多源子域名收集", Module: "subdomain", InstallCmds: []string{"go install -v github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest"}},
	{Name: "ksubdomain", Description: "无状态子域名爆破", Module: "subdomain", InstallCmds: []string{"go install -v github.com/boy-hack/ksubdomain/v2/cmd/ksubdomain@latest"}},
	{Name: "shuffledns", Description: "快速DNS解析/爆破", Module: "subdomain", InstallCmds: []string{"go install -v github.com/projectdiscovery/shuffledns/cmd/shuffledns@latest"}},
	{Name: "bbot", Description: "BBOT OSINT框架", Module: "subdomain", InstallCmds: []string{"pipx install bbot --force && pipx ensurepath || pipx install bbot --force || true", "bbot -t init.bbot.local -f subdomain-enum -y --ignore-failed-deps -n bbot_dep_init 2>&1 | head -200 || true"}},
	{Name: "findomain", Description: "Findomain子域名收集", Module: "subdomain", InstallCmds: []string{"curl -sL https://github.com/Findomain/Findomain/releases/latest/download/findomain-linux-i386.zip -o /tmp/findomain.zip && unzip -o /tmp/findomain.zip -d /usr/local/bin && chmod +x /usr/local/bin/findomain && rm /tmp/findomain.zip"}},
	{Name: "naabu", Description: "端口扫描", Module: "port", InstallCmds: []string{"go install -v github.com/projectdiscovery/naabu/v2/cmd/naabu@latest"}},
	{Name: "httpx", Description: "HTTP 探测与指纹识别", Module: "http", InstallCmds: []string{"go install -v github.com/projectdiscovery/httpx/cmd/httpx@latest"}},
	{Name: "nuclei", Description: "漏洞扫描", Module: "vuln", InstallCmds: []string{"go install -v github.com/projectdiscovery/nuclei/v3/cmd/nuclei@latest", "nuclei -update-templates"}},
	{Name: "ffuf", Description: "高速目录/路径扫描", Module: "dir", InstallCmds: []string{"go install -v github.com/ffuf/ffuf/v2@latest"}},
	{Name: "claude", Description: "Claude Code AI 渗透测试代理", Module: "ai-pentest", InstallCmds: []string{"curl -fsSL https://claude.ai/install.sh | bash", "npx --yes skills add yaklang/hack-skills"}},
}

func Names() []string {
	names := make([]string, len(All))
	for i, t := range All {
		names[i] = t.Name
	}
	return names
}
