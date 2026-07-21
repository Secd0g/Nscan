package main

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/yourname/nscan/internal/scanner/agent"
	scancfg "github.com/yourname/nscan/internal/scanner/config"
	"github.com/yourname/nscan/internal/scanner/configsync"
	"github.com/yourname/nscan/internal/scanner/engine"
	"github.com/yourname/nscan/internal/scanner/plugins/loader"
	"github.com/yourname/nscan/internal/scanner/scanners/brute"
	"github.com/yourname/nscan/internal/scanner/scanners/crawler"
	"github.com/yourname/nscan/internal/scanner/scanners/dir"
	"github.com/yourname/nscan/internal/scanner/scanners/httpx"
	"github.com/yourname/nscan/internal/scanner/scanners/nuclei"
	"github.com/yourname/nscan/internal/scanner/scanners/port"
	"github.com/yourname/nscan/internal/scanner/scanners/search"
	"github.com/yourname/nscan/internal/scanner/scanners/sensitive"
	"github.com/yourname/nscan/internal/scanner/scanners/subdomain"
	"github.com/yourname/nscan/pkg/tooldef"
	"go.uber.org/zap"
)

func main() {
	ensureGoPathInPATH()

	cfgPath := flag.String("config", "configs/scanner.yaml", "config file path")
	flag.Parse()

	cfg, err := scancfg.Load(*cfgPath)
	if err != nil {
		panic("load config: " + err.Error())
	}

	log := buildLogger(cfg.Log.Level, cfg.Log.Format)
	defer log.Sync()

	dataDir := cfg.Scanner.DataDir
	if dataDir == "" {
		dataDir = "./data/scanner"
	}

	// ── 配置中心化：从服务端同步 POC/字典/指纹 ────────────────────────────────
	var pocDir string
	if cfg.Scanner.ServerHTTP != "" {
		cs := configsync.New(cfg.Scanner.ServerHTTP, cfg.Scanner.Token, dataDir, log)
		if err := cs.SyncAll(context.Background()); err != nil {
			log.Warn("initial config sync failed, continuing with local files", zap.Error(err))
		}
		cs.Start(context.Background(), 30*time.Minute)
		pocDir = cs.POCDir()
	}

	// ── 构建 Pipeline Engine，注册内置 Stage ─────────────────────────────────
	eng := engine.NewPipelineEngine(log)
	defer eng.Shutdown()

	// 崩溃恢复
	if err := eng.InitRecovery(dataDir); err != nil {
		log.Warn("task recovery init failed, continuing without it", zap.Error(err))
	} else if tasks, err := eng.RecoverTasks(); err != nil {
		log.Warn("task recovery load failed", zap.Error(err))
	} else if len(tasks) > 0 {
		log.Info("recovered pending tasks from previous run", zap.Int("count", len(tasks)))
	}

	eng.Register(subdomain.New(log))
	eng.Register(subdomain.NewShufflednsStage(log))
	eng.Register(subdomain.NewBbotStage(log))
	eng.Register(subdomain.NewFindomainStage(log))
	eng.Register(port.New(log))
	eng.Register(httpx.NewWithFingerprints(log, filepath.Join(dataDir, "fingerprints.json")))
	eng.Register(crawler.New(log))
	eng.Register(nuclei.New(log, pocDir))
	eng.Register(brute.New(log))
	eng.Register(dir.New(log))
	eng.Register(search.New(log))
	eng.Register(sensitive.New(log))

	// ── 动态加载用户插件（yaegi 运行时） ────────────────────────────────────
	pluginLoader := loader.New(cfg.Scanner.ServerHTTP, cfg.Scanner.Token, eng, log)
	pluginLoader.Start(context.Background(), 10*time.Minute)

	// ── 检测已安装的外部工具 ────────────────────────────────────────────────
	installedTools := detectTools(log)

	// ── 启动 Agent（自动重连） ────────────────────────────────────────────────
	a := agent.New(&cfg.Scanner, eng, log, installedTools)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("nscan scanner starting",
		zap.String("name", cfg.Scanner.Name),
		zap.String("server", cfg.Scanner.ServerAddr),
	)
	a.Run(ctx) // 阻塞直到 ctx 取消

	log.Info("scanner stopped")
}

var extraToolPaths = []string{
	"/root/.local/bin",
	"/usr/local/bin",
	"/home/ubuntu/.local/bin",
}

func detectTools(log *zap.Logger) []string {
	var installed []string
	for _, name := range tooldef.Names() {
		if detectOne(name) {
			installed = append(installed, name)
		}
	}
	log.Info("detected installed tools", zap.Strings("tools", installed))
	return installed
}

func detectOne(name string) bool {
	if _, err := exec.LookPath(name); err == nil {
		return true
	}
	for _, dir := range extraToolPaths {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	home := os.Getenv("HOME")
	if home == "" {
		home = "/root"
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "pipx", "venvs", name, "bin", name)); err == nil {
		return true
	}
	return false
}

func ensureGoPathInPATH() {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, _ := os.UserHomeDir()
		gopath = home + "/go"
	}
	goBin := gopath + "/bin"
	path := os.Getenv("PATH")
	if !strings.Contains(path, goBin) {
		os.Setenv("PATH", goBin+":"+path)
	}
}

func buildLogger(level, format string) *zap.Logger {
	var cfg zap.Config
	if format == "console" {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}
	_ = cfg.Level.UnmarshalText([]byte(level))
	logger, _ := cfg.Build()
	return logger
}
