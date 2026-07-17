package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yourname/nscan/internal/server/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// BatchDeleteAssets 批量删除资产，body: {"type":"http"|"port"|"subdomain"|"vuln","ids":["id1","id2",...]}
func (h *Handler) BatchDeleteAssets(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok { return }
	var req struct {
		Type string   `json:"type"`
		IDs  []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		errResp(c, http.StatusBadRequest, "type and ids required")
		return
	}
	if checkBatchLimit(c, req.IDs) {
		return
	}
	if err := h.assets.BatchDelete(c.Request.Context(), uid, req.Type, req.IDs); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": len(req.IDs)})
}

func assetFilter(c *gin.Context, limit, skip int64) repositories.AssetFilter {
	sortOrder := 0
	if v := c.Query("sort_order"); v == "ascending" {
		sortOrder = 1
	} else if v == "descending" {
		sortOrder = -1
	}
	return repositories.AssetFilter{
		UserID:      UserID(c).Hex(),
		TaskID:      c.Query("task_id"),
		ProjectID:   c.Query("project_id"),
		Q:           c.Query("q"),
		AssetType:   c.Query("asset_type"),
		Severity:    c.Query("severity"),
		StatusCodes: parseStatusCodes(c.Query("status_codes")),
		SortBy:      c.Query("sort_by"),
		SortOrder:   sortOrder,
		Limit:       limit,
		Skip:        skip,
	}
}

// parseStatusCodes 解析 "200,301,403" 形式为 []int，无效项跳过。
func parseStatusCodes(s string) []int {
	if s == "" {
		return nil
	}
	out := make([]int, 0, 4)
	for _, p := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}

func (h *Handler) ListSubdomains(c *gin.Context) {
	limit, skip := paginate(c)
	f := assetFilter(c, limit, skip)
	if f.AssetType == "" {
		f.AssetType = "subdomain"
	}
	list, total, err := h.assets.ListSubdomains(c.Request.Context(), f)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	listResp(c, list, total)
}

func (h *Handler) ListPorts(c *gin.Context) {
	limit, skip := paginate(c)
	f := assetFilter(c, limit, skip)
	if f.AssetType == "" {
		f.AssetType = "port"
	}
	list, total, err := h.assets.ListPorts(c.Request.Context(), f)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	listResp(c, list, total)
}

func (h *Handler) ListIPAggregated(c *gin.Context) {
	limit, skip := paginate(c)
	f := assetFilter(c, limit, skip)
	if f.AssetType == "" {
		f.AssetType = "port"
	}
	rows, total, err := h.assets.ListIPAggregated(c.Request.Context(), f)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows, "total": total})
}

func (h *Handler) ListHTTP(c *gin.Context) {
	limit, skip := paginate(c)
	f := assetFilter(c, limit, skip)
	if f.AssetType == "" {
		f.AssetType = "http"
	}
	list, total, err := h.assets.ListHTTP(c.Request.Context(), f)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	listResp(c, list, total)
}

func (h *Handler) ListDirs(c *gin.Context) {
	limit, skip := paginate(c)
	list, total, err := h.assets.ListDirs(c.Request.Context(), assetFilter(c, limit, skip))
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	listResp(c, list, total)
}

func (h *Handler) ListVulns(c *gin.Context) {
	limit, skip := paginate(c)
	f := assetFilter(c, limit, skip)
	if f.AssetType == "" {
		f.AssetType = "vuln"
	}
	list, total, err := h.assets.ListVulns(c.Request.Context(), f)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	listResp(c, list, total)
}

func (h *Handler) GetVulnDetail(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	vuln, err := h.assets.GetVuln(c.Request.Context(), id)
	if err != nil {
		errResp(c, http.StatusNotFound, "not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": vuln})
}

func (h *Handler) UpdateVulnStatus(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		Status int `json:"status"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Status < 1 || body.Status > 6 {
		errResp(c, http.StatusBadRequest, "invalid status")
		return
	}
	if err := h.assets.UpdateVulnStatus(c.Request.Context(), id, body.Status); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) ListAssetChanges(c *gin.Context) {
	assetType := c.Param("type") // subdomain|port|http
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	list, err := h.assets.ListChanges(c.Request.Context(), assetType, id, 100)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *Handler) ListSensitiveAssets(c *gin.Context) {
	limit, skip := paginate(c)
	f := assetFilter(c, limit, skip)
	if f.AssetType == "" {
		f.AssetType = "sensitive"
	}
	list, total, err := h.assets.ListSensitive(c.Request.Context(), f)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	listResp(c, list, total)
}

func (h *Handler) ListCrawlerAssets(c *gin.Context) {
	limit, skip := paginate(c)
	f := assetFilter(c, limit, skip)
	list, total, err := h.assets.ListCrawler(c.Request.Context(), f)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	listResp(c, list, total)
}

func (h *Handler) SensitiveAggregation(c *gin.Context) {
	f := repositories.AssetFilter{
		TaskID:    c.Query("task_id"),
		ProjectID: c.Query("project_id"),
		Q:         c.Query("q"),
		AssetType: "sensitive",
	}
	data, err := h.assets.SensitiveAggByRule(c.Request.Context(), f)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

func (h *Handler) AssetStats(c *gin.Context) {
	f := repositories.AssetFilter{
		TaskID:    c.Query("task_id"),
		ProjectID: c.Query("project_id"),
	}
	stats, err := h.assets.Stats(c.Request.Context(), f)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *Handler) DashboardCounts(c *gin.Context) {
	counts, err := h.assets.DashboardCounts(c.Request.Context())
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, counts)
}

func (h *Handler) VulnSeverityStats(c *gin.Context) {
	stats, err := h.assets.VulnSeverityStats(c.Request.Context())
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": stats})
}

func (h *Handler) DailyAssetTrend(c *gin.Context) {
	days := 7
	if d := c.Query("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 && n <= 30 {
			days = n
		}
	}
	trend, err := h.assets.DailyAssetTrend(c.Request.Context(), days)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": trend})
}

func (h *Handler) RecentChanges(c *gin.Context) {
	limit, _ := paginate(c)
	list, err := h.assets.RecentChanges(c.Request.Context(), limit)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}
