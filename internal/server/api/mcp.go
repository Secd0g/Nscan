package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yourname/nscan/internal/server/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const mcpProtocolVersion = "2025-06-18"

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP handles the Streamable HTTP transport. It intentionally exposes only
// read-only tools; the normal API remains the place for mutations and scans.
func (h *Handler) MCP(c *gin.Context) {
	if !validMCPOrigin(c) {
		errResp(c, http.StatusForbidden, "invalid origin")
		return
	}
	if c.Request.Method == http.MethodGet {
		c.Status(http.StatusMethodNotAllowed)
		return
	}

	var req mcpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		mcpWrite(c, mcpResponse{JSONRPC: "2.0", ID: nil, Error: mcpError{-32600, "invalid JSON-RPC request"}})
		return
	}
	if req.ID == nil || string(req.ID) == "null" {
		// Notifications do not receive a JSON-RPC response.
		c.Status(http.StatusAccepted)
		return
	}

	var result any
	var callErr *mcpError
	switch req.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "nscan", "version": "1.0.0"},
			"instructions":    "通过只读工具查询 nscan 项目、任务和安全资产。不要把结果当作未经验证的安全结论。",
		}
	case "ping":
		result = map[string]any{}
	case "tools/list":
		result = map[string]any{"tools": mcpTools()}
	case "tools/call":
		result, callErr = h.mcpCall(c, req.Params)
	default:
		callErr = &mcpError{-32601, "method not found: " + req.Method}
	}
	resp := mcpResponse{JSONRPC: "2.0", ID: json.RawMessage(req.ID), Result: result}
	if callErr != nil {
		resp.Result = nil
		resp.Error = callErr
	}
	mcpWrite(c, resp)
}

func validMCPOrigin(c *gin.Context) bool {
	origin := c.GetHeader("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host != "" && strings.EqualFold(u.Host, c.Request.Host)
}

func mcpWrite(c *gin.Context, resp mcpResponse) {
	c.Header("MCP-Protocol-Version", mcpProtocolVersion)
	c.Header("Content-Type", "application/json")
	c.JSON(http.StatusOK, resp)
}

func mcpTools() []map[string]any {
	readOnly := map[string]any{"readOnlyHint": true, "destructiveHint": false, "openWorldHint": false}
	return []map[string]any{
		{"name": "list_projects", "description": "分页查询 nscan 项目列表。", "inputSchema": objectSchema(map[string]any{"limit": intSchema(100), "skip": intSchema(0)}), "annotations": readOnly},
		{"name": "list_tasks", "description": "查询扫描任务，可按项目、状态或名称关键词过滤。", "inputSchema": objectSchema(map[string]any{"project_id": stringSchema(), "status": stringSchema(), "keyword": stringSchema(), "limit": intSchema(100), "skip": intSchema(0)}), "annotations": readOnly},
		{"name": "get_task", "description": "查询单个扫描任务详情。", "inputSchema": objectSchema(map[string]any{"task_id": requiredStringSchema()}), "annotations": readOnly},
		{"name": "query_assets", "description": "查询子域名、端口、HTTP、漏洞、目录、爬虫或敏感信息资产。支持 q 表达式和分页。", "inputSchema": objectSchema(map[string]any{"asset_type": requiredEnumSchema([]string{"subdomain", "port", "http", "vuln", "dir", "crawler", "sensitive"}), "project_id": stringSchema(), "task_id": stringSchema(), "q": stringSchema(), "severity": stringSchema(), "limit": intSchema(100), "skip": intSchema(0)}), "annotations": readOnly},
		{"name": "asset_stats", "description": "查询全平台或指定项目的资产统计。", "inputSchema": objectSchema(map[string]any{"project_id": stringSchema(), "task_id": stringSchema()}), "annotations": readOnly},
	}
}

func objectSchema(props map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": props, "additionalProperties": false}
}
func stringSchema() map[string]any         { return map[string]any{"type": "string"} }
func requiredStringSchema() map[string]any { return map[string]any{"type": "string", "minLength": 1} }
func requiredEnumSchema(values []string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}
func intSchema(max int) map[string]any {
	return map[string]any{"type": "integer", "minimum": 0, "maximum": max}
}

func (h *Handler) mcpCall(c *gin.Context, raw json.RawMessage) (any, *mcpError) {
	var call struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &call); err != nil || call.Name == "" {
		return nil, &mcpError{-32602, "invalid tools/call arguments"}
	}
	args := call.Arguments
	ctx := c.Request.Context()
	limit := mcpInt(args, "limit", 50)
	if limit < 1 || limit > 100 {
		return nil, &mcpError{-32602, "limit must be between 1 and 100"}
	}
	skip := mcpInt(args, "skip", 0)
	if skip < 0 {
		return nil, &mcpError{-32602, "skip must be non-negative"}
	}

	var data any
	var total int64
	var err error
	switch call.Name {
	case "list_projects":
		data, total, err = h.projects.List(ctx, limit, skip)
	case "list_tasks":
		var projectID *primitive.ObjectID
		if value := mcpString(args, "project_id"); value != "" {
			id, parseErr := primitive.ObjectIDFromHex(value)
			if parseErr != nil {
				return nil, &mcpError{-32602, "invalid project_id"}
			}
			projectID = &id
		}
		data, total, err = h.tasks.List(ctx, projectID, mcpString(args, "status"), mcpString(args, "keyword"), limit, skip)
	case "get_task":
		id, parseErr := primitive.ObjectIDFromHex(mcpString(args, "task_id"))
		if parseErr != nil {
			return nil, &mcpError{-32602, "invalid task_id"}
		}
		data, err = h.tasks.GetByID(ctx, id)
	case "query_assets":
		assetType := mcpString(args, "asset_type")
		if assetType == "" {
			return nil, &mcpError{-32602, "asset_type is required"}
		}
		f := repositories.AssetFilter{ProjectID: mcpString(args, "project_id"), TaskID: mcpString(args, "task_id"), Q: mcpString(args, "q"), AssetType: assetType, Severity: mcpString(args, "severity"), Limit: limit, Skip: skip}
		switch assetType {
		case "subdomain":
			data, total, err = h.assets.ListSubdomains(ctx, f)
		case "port":
			data, total, err = h.assets.ListPorts(ctx, f)
		case "http":
			data, total, err = h.assets.ListHTTP(ctx, f)
		case "vuln":
			data, total, err = h.assets.ListVulns(ctx, f)
		case "dir":
			data, total, err = h.assets.ListDirs(ctx, f)
		case "crawler":
			data, total, err = h.assets.ListCrawler(ctx, f)
		case "sensitive":
			data, total, err = h.assets.ListSensitive(ctx, f)
		default:
			return nil, &mcpError{-32602, "unsupported asset_type"}
		}
	case "asset_stats":
		f := repositories.AssetFilter{ProjectID: mcpString(args, "project_id"), TaskID: mcpString(args, "task_id")}
		data, err = h.assets.Stats(ctx, f)
	default:
		return nil, &mcpError{-32602, "unknown tool: " + call.Name}
	}
	if err != nil {
		return nil, &mcpError{-32000, "query failed: " + err.Error()}
	}
	if call.Name == "get_task" || call.Name == "asset_stats" {
		total = 1
	}
	return map[string]any{"content": []map[string]string{{"type": "text", "text": mcpJSONText(data, total)}}, "structuredContent": map[string]any{"data": data, "total": total}, "isError": false}, nil
}

func mcpJSONText(data any, total int64) string {
	b, err := json.Marshal(map[string]any{"data": data, "total": total})
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(b)
}
func mcpString(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
func mcpInt(args map[string]any, key string, fallback int64) int64 {
	if v, ok := args[key].(float64); ok {
		return int64(v)
	}
	if v, ok := args[key].(json.Number); ok {
		n, _ := strconv.ParseInt(string(v), 10, 64)
		return n
	}
	return fallback
}
