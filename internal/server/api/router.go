package api

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yourname/nscan/internal/server/hub"
)

var workDir string

func init() {
	if wd, err := os.Getwd(); err == nil {
		workDir = wd
	}
}

func (h *Handler) Register(r *gin.Engine) {
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	// MCP Streamable HTTP endpoint. It reuses the platform JWT and is read-only.
	r.POST("/mcp", AuthMiddleware(h.jwtSecret), h.MCP)
	r.GET("/mcp", AuthMiddleware(h.jwtSecret), h.MCP)

	// 截图静态文件
	r.Static("/images", "./images")

	v1 := r.Group("/api/v1")

	// 公开路由
	auth := v1.Group("/auth")
	auth.GET("/captcha", h.Captcha)
	auth.POST("/login", h.Login)
	// 当前用户信息（需鉴权，放 /auth/me 便于前端统一前缀）

	// 节点下载不使用 JWT，但必须携带当前节点 Key。
	v1.GET("/nodes/docker-compose-worker.yaml", func(c *gin.Context) {
		if !h.validNodeDownloadToken(c) {
			errResp(c, http.StatusUnauthorized, "invalid node token")
			return
		}
		host, grpcPort := splitHostPort(c.Request.Host, h.grpcAddr)
		// 容器内 localhost 指向容器自身，替换为 host.docker.internal
		if host == "localhost" || host == "127.0.0.1" {
			host = "host.docker.internal"
		}
		serverAddr := net.JoinHostPort(host, grpcPort)
		yaml := fmt.Sprintf(`services:
  nscan-scanner:
    image: %q
    restart: unless-stopped
    extra_hosts:
      - "host.docker.internal:host-gateway"
    environment:
      SERVER_ADDR: %q
      TOKEN: %q
      NODE_NAME: "node-1"
      MAX_TASKS: "5"
`, h.scannerImage, serverAddr, h.tokenStore.Get())
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(yaml))
	})

	v1.GET("/nodes/install.sh", func(c *gin.Context) {
		if !h.validNodeDownloadToken(c) {
			errResp(c, http.StatusUnauthorized, "invalid node token")
			return
		}
		scheme := "http"
		if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		} else if c.Request.TLS != nil {
			scheme = "https"
		}
		baseURL := fmt.Sprintf("%s://%s/api/v1/nodes", scheme, c.Request.Host)
		script := fmt.Sprintf(`#!/bin/sh
set -e
echo "[nscan] 正在下载部署文件..."
curl -fsSL -H %s -o docker-compose-worker.yaml %s
echo "[nscan] 正在拉取并启动扫描节点..."
docker compose -f docker-compose-worker.yaml up -d
echo "[nscan] 部署完成！扫描节点已启动。"
`, shellQuote("X-Nscan-Token: "+h.tokenStore.Get()), shellQuote(baseURL+"/docker-compose-worker.yaml"))
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(script))
	})

	// ====== 以下接口全部需要鉴权 ======
	v1.Use(AuthMiddleware(h.jwtSecret))

	// 当前用户信息
	auth.Use(AuthMiddleware(h.jwtSecret))
	auth.GET("/me", h.Me)
	auth.POST("/change-password", h.ChangePassword)

	// 节点
	v1.GET("/nodes", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": h.nm.ListNodes()})
	})
	v1.DELETE("/nodes/:id", func(c *gin.Context) {
		nodeID := c.Param("id")
		h.nm.Unregister(nodeID)
		h.nodeLog.Clear(nodeID)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	v1.POST("/nodes/:id/restart", func(c *gin.Context) {
		nodeID := c.Param("id")
		if !h.nm.Restart(nodeID) {
			errResp(c, http.StatusNotFound, "node not found or offline")
			return
		}
		restartMsg := "收到重启指令，节点即将重启"
		h.nodeLog.Append(nodeID, restartMsg, "warn")
		h.hub.Publish("node:"+nodeID, hub.Event{Kind: "log", Log: restartMsg, Level: "warn"})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	v1.POST("/nodes/:id/install-tool", h.InstallTool)
	v1.POST("/nodes/:id/uninstall-tool", h.UninstallTool)
	v1.GET("/nodes/token", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"token": h.tokenStore.Get()})
	})
	v1.POST("/nodes/token/regenerate", func(c *gin.Context) {
		token, err := h.tokenStore.Regenerate(c.Request.Context())
		if err != nil {
			errResp(c, http.StatusInternalServerError, "regenerate token: "+err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"token": token})
	})

	// 项目
	projects := v1.Group("/projects")
	projects.GET("", h.ListProjects)
	projects.POST("", h.CreateProject)
	projects.GET("/:id", h.GetProject)
	projects.PUT("/:id", h.UpdateProject)
	projects.DELETE("/:id", h.DeleteProject)

	// 任务
	tasks := v1.Group("/tasks")
	tasks.GET("", h.ListTasks)
	tasks.POST("", h.CreateTask)
	tasks.GET("/:id", h.GetTask)
	tasks.PUT("/:id", h.UpdateTask)
	tasks.GET("/:id/logs", h.GetTaskLogs)
	tasks.POST("/:id/cancel", h.CancelTask)
	tasks.POST("/:id/rescan", h.RescanTask)
	tasks.DELETE("/:id", h.DeleteTask)
	tasks.GET("/:id/subtasks", h.ListSubtasks)
	tasks.GET("/:id/dead-letter", h.ListDeadLetterByTask)
	tasks.POST("/:id/ai-analysis", h.AnalyzeTask)
	tasks.POST("/:id/ai-analysis/stop", h.StopAnalyzeTask)
	tasks.POST("/:id/ai-pentest", h.StartAIPentest)
	tasks.POST("/:id/ai-pentest/stop", h.StopAIPentest)

	v1.POST("/dead-letter/:subtaskId/retry", h.RetryDeadLetter)

	// 节点日志：历史（REST）+ 实时（WebSocket）
	v1.GET("/nodes/:id/logs", func(c *gin.Context) {
		limit := 200
		if v := c.Query("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		logs := h.nodeLog.Get(c.Param("id"), limit)
		c.JSON(http.StatusOK, gin.H{"data": logs})
	})

	// WebSocket
	ws := r.Group("/ws")
	ws.Use(AuthWsMiddleware(h.jwtSecret))
	ws.GET("/nodes/:id/logs", h.NodeLogs)
	ws.GET("/tasks/:id/progress", h.TaskProgress)

	// 资产
	assets := v1.Group("/assets")

	assets.GET("/subdomains", h.ListSubdomains)
	assets.GET("/ports", h.ListPorts)
	assets.GET("/ip", h.ListIPAggregated)
	assets.GET("/http", h.ListHTTP)
	assets.GET("/vulns", h.ListVulns)
	assets.GET("/vulns/:id", h.GetVulnDetail)
	assets.PATCH("/vulns/:id/status", h.UpdateVulnStatus)
	assets.GET("/dirs", h.ListDirs)
	assets.GET("/crawler", h.ListCrawlerAssets)
	assets.GET("/sensitive", h.ListSensitiveAssets)
	assets.GET("/sensitive/aggregation", h.SensitiveAggregation)
	assets.GET("/:type/:id/changes", h.ListAssetChanges)
	assets.GET("/stats", h.AssetStats)
	assets.GET("/dashboard-counts", h.DashboardCounts)
	assets.GET("/recent-changes", h.RecentChanges)
	assets.GET("/vuln-severity-stats", h.VulnSeverityStats)
	assets.GET("/daily-trend", h.DailyAssetTrend)
	assets.DELETE("/batch", h.BatchDeleteAssets)

	// 导出
	exp := v1.Group("/export")
	exp.GET("/assets", h.ExportAssets)
	exp.GET("/assets/all", h.ExportAllAssets)
	exp.GET("/task/:id", h.ExportTaskReport)
	exp.GET("/task/:id/ai", h.ExportAIReport)

	// 工具定义
	v1.GET("/tool-defs", h.ListToolDefs)

	// 插件
	plugins := v1.Group("/plugins")
	plugins.GET("", h.ListPlugins)
	plugins.GET("/:id", h.GetPlugin)
	plugins.POST("", h.CreatePlugin)
	plugins.PUT("/:id", h.UpdatePlugin)
	plugins.DELETE("/:id", h.DeletePlugin)
	plugins.POST("/upload", h.UploadPlugin)

	// 扫描模版
	tpl := v1.Group("/scan-templates")
	tpl.GET("", h.ListScanTemplates)
	tpl.POST("", h.CreateScanTemplate)
	tpl.PUT("/:id", h.UpdateScanTemplate)
	tpl.DELETE("/:id", h.DeleteScanTemplate)
	tpl.DELETE("", h.BatchDeleteScanTemplates)

	// 定时扫描
	scheduled := v1.Group("/scheduled")
	scheduled.GET("", h.ListScheduled)
	scheduled.POST("", h.CreateScheduled)
	scheduled.PUT("/:id", h.UpdateScheduled)
	scheduled.DELETE("/:id", h.DeleteScheduled)
	scheduled.POST("/:id/run", h.RunScheduledNow)

	// 通知设置
	notify := v1.Group("/notify")
	notify.GET("", h.ListNotify)
	notify.PUT("/:key", h.SaveNotify)
	notify.POST("/:key/test", h.TestNotify)

	// 系统设置（API 密钥等）
	settings := v1.Group("/settings")
	settings.GET("/providers/:key", h.GetProviderConfig)
	settings.PUT("/providers/:key", h.SaveProviderConfig)
	settings.GET("/ai", h.GetAIConfig)
	settings.PUT("/ai", h.SaveAIConfig)

	// 在线搜索（Fofa/Hunter/Quake/Shodan）
	os := v1.Group("/online-search")
	os.POST("/:provider", h.OnlineSearchQuery)
	os.POST("/:provider/import", h.OnlineSearchImport)

	// 全局黑名单
	bl := v1.Group("/blacklist")
	bl.GET("", h.ListBlacklist)
	bl.POST("", h.AddBlacklist)
	bl.POST("/batch", h.BatchAddBlacklist)
	bl.DELETE("/:id", h.RemoveBlacklist)
	bl.DELETE("", h.ClearBlacklist)

	// POC / Nuclei 模板
	poc := v1.Group("/poc")
	poc.GET("/templates", h.ListNucleiTemplates)
	poc.GET("/templates/stats", h.NucleiTemplateStats)
	poc.GET("/templates/categories", h.NucleiTemplateCategories)
	poc.GET("/templates/:id/content", h.NucleiTemplateContent)
	poc.POST("/templates/sync", h.SyncNucleiTemplates)
	poc.POST("/templates/sync-online", h.SyncNucleiTemplatesOnline)
	poc.DELETE("/templates", h.ClearNucleiTemplates)
	poc.GET("/custom", h.ListCustomPocs)
	poc.POST("/custom", h.CreateCustomPoc)
	poc.PUT("/custom/:id", h.UpdateCustomPoc)
	poc.DELETE("/custom/:id", h.DeleteCustomPoc)
	poc.DELETE("/custom", h.ClearCustomPocs)
	poc.POST("/custom/import", h.ImportCustomPocs)
	poc.GET("/custom/export", h.ExportCustomPocs)

	// 指纹管理
	fp := v1.Group("/fingerprints")
	fp.GET("", h.ListFingerprints)
	fp.GET("/categories", h.FingerprintCategories)
	fp.POST("", h.CreateFingerprint)
	fp.PUT("/:id", h.UpdateFingerprint)
	fp.DELETE("/:id", h.DeleteFingerprint)
	fp.DELETE("", h.ClearFingerprints)
	fp.POST("/import", h.ImportFingerprints)
	fp.POST("/sync-online", h.SyncFingerprintsOnline)

	// 敏感信息规则管理
	sens := v1.Group("/sensitive-rules")
	sens.GET("", h.ListSensitiveRules)
	sens.POST("", h.CreateSensitiveRule)
	sens.PUT("/:id", h.UpdateSensitiveRule)
	sens.DELETE("/:id", h.DeleteSensitiveRule)

	// 字典管理
	dicts := v1.Group("/dicts")
	dicts.GET("", h.ListDicts)
	dicts.POST("", h.CreateDict)
	dicts.PUT("/:id", h.UpdateDict)
	dicts.DELETE("/:id", h.DeleteDict)
	dicts.GET("/:id/preview", h.PreviewDict)
	dicts.GET("/:id/content", h.GetDictContent)
	dicts.PUT("/:id/content", h.UpdateDictContent)
	dicts.DELETE("", h.ClearDicts)
	dicts.POST("/sync-online", h.SyncDictsOnline)

	// 任务批量删除
	tasks.DELETE("", h.BatchDeleteTasks)

	// 项目批量删除
	projects.DELETE("", h.BatchDeleteProjects)
}

// InstallTool 向节点发送工具安装指令
func (h *Handler) InstallTool(c *gin.Context) {
	nodeID := c.Param("id")
	var req struct {
		ToolName   string `json:"tool_name" binding:"required"`
		InstallCmd string `json:"install_cmd" binding:"required"`
		Reinstall  bool   `json:"reinstall"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	if !h.nm.InstallTool(nodeID, req.ToolName, req.InstallCmd, req.Reinstall) {
		errResp(c, http.StatusNotFound, "node not found or offline")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "install command sent"})
}

// UninstallTool 向节点发送工具卸载指令（删除二进制）
func (h *Handler) UninstallTool(c *gin.Context) {
	nodeID := c.Param("id")
	var req struct {
		ToolName string `json:"tool_name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	if !h.nm.UninstallTool(nodeID, req.ToolName) {
		errResp(c, http.StatusNotFound, "node not found or offline")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "uninstall command sent"})
}

func (h *Handler) validNodeDownloadToken(c *gin.Context) bool {
	expected := h.tokenStore.Get()
	provided := c.GetHeader("X-Nscan-Token")
	return expected != "" && len(provided) == len(expected) &&
		subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func splitHostPort(reqHost, cfgAddr string) (host, port string) {
	host = reqHost
	if parsedHost, _, err := net.SplitHostPort(reqHost); err == nil {
		host = parsedHost
	} else {
		host = strings.Trim(reqHost, "[]")
	}
	if _, parsedPort, err := net.SplitHostPort(cfgAddr); err == nil {
		port = parsedPort
	} else if strings.HasPrefix(cfgAddr, ":") {
		port = strings.TrimPrefix(cfgAddr, ":")
	}
	if port == "" {
		port = "9000"
	}
	return
}
