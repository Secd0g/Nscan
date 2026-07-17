package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yourname/nscan/internal/server/ai"
	"github.com/yourname/nscan/internal/server/repositories"
	"github.com/yourname/nscan/pkg/models"
	"github.com/yourname/nscan/pkg/proto/scanv1"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (h *Handler) GetAIConfig(c *gin.Context) {
	var cfg ai.Config
	if raw, err := h.settings.GetValue(c.Request.Context(), "ai"); err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	c.JSON(http.StatusOK, cfg)
}

func (h *Handler) SaveAIConfig(c *gin.Context) {
	var cfg ai.Config
	if err := c.ShouldBindJSON(&cfg); err != nil {
		errResp(c, http.StatusBadRequest, "AI 配置格式无效")
		return
	}
	if cfg.Type == "anthropic" {
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://api.anthropic.com"
		}
		if cfg.Model == "" {
			cfg.Model = "claude-sonnet-4-20250514"
		}
	}
	if cfg.BaseURL == "" || cfg.Token == "" || cfg.Model == "" {
		errResp(c, http.StatusBadRequest, "接口地址、Token 和模型不能为空")
		return
	}
	raw, _ := json.Marshal(cfg)
	if err := h.settings.SetValue(c.Request.Context(), "ai", string(raw)); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) ExportAIReport(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok { return }
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, 400, "invalid task id")
		return
	}
	task, err := h.tasks.GetByIDForUser(c.Request.Context(), id, uid)
	if err != nil {
		errResp(c, 404, "task not found")
		return
	}
	if task.AIAnalysisStatus != "done" || task.AIAnalysis == "" {
		errResp(c, 400, "AI 分析尚未完成")
		return
	}
	filename := fmt.Sprintf("nscan_ai_report_%s.md", id.Hex()[:8])
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(task.AIAnalysis))
}

func (h *Handler) AnalyzeTask(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok { return }
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, 400, "invalid task id")
		return
	}
	task, err := h.tasks.GetByIDForUser(c.Request.Context(), id, uid)
	if err != nil {
		errResp(c, 404, "task not found")
		return
	}
	if task.Status != models.TaskStatusDone {
		errResp(c, 400, "任务完成后才能进行 AI 分析")
		return
	}
	h.aiJobsMu.Lock()
	if _, ok := h.aiJobs[id.Hex()]; ok {
		h.aiJobsMu.Unlock()
		errResp(c, http.StatusConflict, "AI 分析正在进行中")
		return
	}
	raw, _ := h.settings.GetValue(c.Request.Context(), "ai")
	var cfg ai.Config
	if raw == "" || json.Unmarshal([]byte(raw), &cfg) != nil || cfg.BaseURL == "" || cfg.Token == "" || cfg.Model == "" {
		h.aiJobsMu.Unlock()
		errResp(c, 400, "请先在 AI 配置中填写接口地址、Token 和模型")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.aiJobs[id.Hex()] = cancel
	h.aiJobsMu.Unlock()
	_ = h.tasks.UpdateForUser(c.Request.Context(), id, uid, bson.M{"ai_analysis_status": "running", "ai_analysis_error": "", "ai_analysis": "", "ai_analysis_log": []string{"已启动 AI 分析"}})
	go func() {
		appendLog := func(line string) {
			now, _ := h.tasks.GetByIDForUser(context.Background(), id, uid)
			if now == nil { return }
			logs := append(now.AIAnalysisLog, line)
			_ = h.tasks.UpdateForUser(context.Background(), id, uid, bson.M{"ai_analysis_log": logs})
		}
		result, callErr := ai.Analyze(ctx, cfg, task, h.assets, appendLog)
		status, msg := "done", ""
		if callErr != nil {
			status = "failed"
			msg = callErr.Error()
			if ctx.Err() != nil {
				status = "cancelled"
				msg = "用户已停止 AI 分析"
			}
		}
		update := bson.M{"ai_analysis_status": status, "ai_analysis_error": msg}
		if status == "done" {
			update["ai_analysis"] = result
			appendLog("AI 分析完成")
		} else {
			appendLog(msg)
		}
		_ = h.tasks.UpdateForUser(context.Background(), id, uid, update)
		h.aiJobsMu.Lock()
		delete(h.aiJobs, id.Hex())
		h.aiJobsMu.Unlock()
	}()
	task.AIAnalysisStatus = "running"
	task.AIAnalysisLog = []string{"已启动 AI 分析"}
	c.JSON(http.StatusAccepted, task)
}

func (h *Handler) StopAnalyzeTask(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok { return }
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, 400, "invalid task id")
		return
	}
	h.aiJobsMu.Lock()
	cancel, ok := h.aiJobs[id.Hex()]
	h.aiJobsMu.Unlock()
	if ok {
		cancel()
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	_ = h.tasks.UpdateForUser(c.Request.Context(), id, uid, bson.M{"ai_analysis_status": "cancelled", "ai_analysis_error": "用户已停止 AI 分析"})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// StartAIPentest starts a Claude Code run on an online node after the scan is
// complete. It deliberately requires an explicit request and never accepts a
// shell command from the client.
func (h *Handler) StartAIPentest(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok { return }
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid task id")
		return
	}
	task, err := h.tasks.GetByIDForUser(c.Request.Context(), id, uid)
	if err != nil {
		errResp(c, http.StatusNotFound, "task not found")
		return
	}
	if task.Status != models.TaskStatusDone {
		errResp(c, http.StatusBadRequest, "任务完成后才能启动 AI 渗透")
		return
	}
	if task.AIPentestStatus == "running" {
		errResp(c, http.StatusConflict, "AI 渗透正在进行中")
		return
	}
	var cfg ai.Config
	raw, _ := h.settings.GetValue(c.Request.Context(), "ai")
	if raw == "" || json.Unmarshal([]byte(raw), &cfg) != nil || cfg.Token == "" {
		errResp(c, http.StatusBadRequest, "请先在 AI 配置中填写 Anthropic API Key")
		return
	}
	if cfg.Type != "anthropic" {
		errResp(c, http.StatusBadRequest, "请将 AI 配置的接口类型切换为 Anthropic / Claude")
		return
	}
	var req struct {
		NodeID         string `json:"node_id"`
		Prompt         string `json:"prompt"`
		TimeoutSeconds int32  `json:"timeout_seconds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	node := h.nm.PickNode([]string{"ai-pentest"})
	if req.NodeID != "" {
		node = h.nm.PickNodeFrom([]string{req.NodeID})
	}
	if node == nil {
		errResp(c, http.StatusConflict, "没有可用的 AI 渗透节点")
		return
	}
	if !contains(node.InstalledTools, "claude") {
		errResp(c, http.StatusConflict, "所选节点尚未安装 Claude Code")
		return
	}
	if req.TimeoutSeconds <= 0 || req.TimeoutSeconds > 3600 {
		req.TimeoutSeconds = 1800
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = "请基于 nscan 扫描结果进行授权范围内的验证性渗透测试。"
	}
	prompt += "\n\n硬性边界：只能测试以下已授权目标，不得访问或攻击其他目标：" + strings.Join(task.Targets, ", ") +
		"。不得进行破坏性操作、持久化、提权、横向移动、数据外传或真实凭据利用；发现高风险动作时只给出建议并停止。所有结论必须保留证据、请求/响应摘要和复现步骤。任务 ID：" + id.Hex()
	// Give Claude the completed scan evidence directly. This keeps the node
	// independent from a user JWT/MCP session and avoids an overly broad API key.
	f := repositories.AssetFilter{TaskID: id.Hex(), Limit: 500}
	sub, _, _ := h.assets.ListSubdomains(c.Request.Context(), f)
	ports, _, _ := h.assets.ListPorts(c.Request.Context(), f)
	httpAssets, _, _ := h.assets.ListHTTP(c.Request.Context(), f)
	vulns, _, _ := h.assets.ListVulns(c.Request.Context(), f)
	evidence, _ := json.Marshal(map[string]any{"subdomains": sub, "ports": ports, "http": httpAssets, "vulnerabilities": vulns})
	prompt += "\n\nnscan 已完成的扫描证据（仅作线索，必须自行验证）：\n" + string(evidence)
	if !h.nm.RunAIPentest(node.ID, id.Hex(), prompt, task.Targets, req.TimeoutSeconds, cfg.Token) {
		errResp(c, http.StatusConflict, "节点已离线")
		return
	}
	_ = h.tasks.UpdateForUser(c.Request.Context(), id, uid, bson.M{"ai_pentest_status": "running", "ai_pentest_error": "", "ai_pentest_output": "", "ai_pentest_node_id": node.ID, "ai_pentest_log": []string{"已授权并发送到节点 " + node.Name}})
	task.AIPentestStatus = "running"
	task.AIPentestNodeID = node.ID
	task.AIPentestLog = []string{"已授权并发送到节点 " + node.Name}
	c.JSON(http.StatusAccepted, task)
}

func (h *Handler) StopAIPentest(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok { return }
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, 400, "invalid task id")
		return
	}
	task, err := h.tasks.GetByIDForUser(c.Request.Context(), id, uid)
	if err != nil {
		errResp(c, 404, "task not found")
		return
	}
	if task.AIPentestNodeID != "" {
		_ = h.nm.CancelAIPentest(task.AIPentestNodeID, id.Hex())
	}
	_ = h.tasks.UpdateForUser(c.Request.Context(), id, uid, bson.M{"ai_pentest_status": "cancelled", "ai_pentest_error": "用户已停止 AI 渗透"})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) OnAIPentestResult(result *scanv1.AIPentestResult) {
	id, err := primitive.ObjectIDFromHex(result.TaskId)
	if err != nil {
		return
	}
	update := bson.M{"ai_pentest_status": result.Status, "ai_pentest_error": result.Error, "ai_pentest_output": result.Output, "ai_pentest_node_id": result.NodeId}
	if result.Log != "" {
		update["ai_pentest_log"] = []string{result.Log}
	}
	_ = h.tasks.Update(context.Background(), id, update)
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
