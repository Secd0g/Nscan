package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	nsserver "github.com/yourname/nscan/internal/server"
	"github.com/yourname/nscan/internal/server/repositories"
	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (h *Handler) ListDicts(c *gin.Context) {
	f := repositories.ListFilter{
		Category: c.Query("category"),
		Service:  c.Query("service"),
		Kind:     c.Query("kind"),
	}
	list, err := h.dict.Query(c.Request.Context(), f)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *Handler) CreateDict(c *gin.Context) {
	var body struct {
		Category    string `json:"category"`
		Service     string `json:"service"`
		Kind        string `json:"kind"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	lines := splitLines(body.Content)
	d := &models.Dict{
		Category:    body.Category,
		Service:     body.Service,
		Kind:        body.Kind,
		Name:        body.Name,
		Description: body.Description,
		Builtin:     false,
		Active:      false,
	}
	if err := h.dict.Create(c.Request.Context(), d, lines); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *Handler) UpdateDict(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	delete(body, "id")
	delete(body, "_id")
	dict, err := h.dict.Get(c.Request.Context(), id)
	if err != nil {
		errResp(c, http.StatusNotFound, "字典不存在")
		return
	}
	if dict.Builtin && dict.Category != "password" {
		errResp(c, http.StatusForbidden, "内置字典不可修改")
		return
	}
	if err := h.dict.Update(c.Request.Context(), id, body); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) DeleteDict(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	dict, err := h.dict.Get(c.Request.Context(), id)
	if err != nil {
		errResp(c, http.StatusNotFound, "字典不存在")
		return
	}
	if dict.Builtin && dict.Category != "password" {
		errResp(c, http.StatusForbidden, "内置字典不可修改")
		return
	}
	if err := h.dict.Delete(c.Request.Context(), id); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) PreviewDict(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	limit, skip := paginate(c)
	if limit > 500 {
		limit = 500
	}
	lines, total, err := h.dict.GetLines(c.Request.Context(), id, limit, skip)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"lines": lines, "total": total})
}

// GetDictContent 返回字典完整内容为字符串（用于前端编辑）
func (h *Handler) GetDictContent(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	content, err := h.dict.GetContent(c.Request.Context(), id)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": content})
}

// UpdateDictContent 用完整字符串替换字典内容
func (h *Handler) UpdateDictContent(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	dict, err := h.dict.Get(c.Request.Context(), id)
	if err != nil {
		errResp(c, http.StatusNotFound, "字典不存在")
		return
	}
	if dict.Builtin && dict.Category != "password" {
		errResp(c, http.StatusForbidden, "内置字典不可修改")
		return
	}
	lines := splitLines(body.Content)
	if err := h.dict.SetContent(c.Request.Context(), id, lines); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "count": len(lines)})
}

func (h *Handler) ClearDicts(c *gin.Context) {
	category := c.Query("category")
	if err := h.dict.Clear(c.Request.Context(), category); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) SyncDictsOnline(c *gin.Context) {
	type dictDef struct {
		Category    string
		Name        string
		Description string
	}
	defs := []dictDef{
		{"subdomain", "子域名爆破字典", ""},
		{"directory", "目录扫描字典", ""},
	}
	totalCount := 0
	for _, def := range defs {
		lines, err := nsserver.LoadEmbeddedDict(def.Category)
		if err != nil || len(lines) == 0 {
			continue
		}
		// 先删除该分类的旧内置字典，避免与新 seed 重复
		if err := h.dict.DeleteBuiltinByCategory(c.Request.Context(), def.Category); err != nil {
			continue
		}
		d := &models.Dict{
			Category:    def.Category,
			Name:        def.Name,
			Description: def.Description,
			Builtin:     true,
			Active:      true,
		}
		if err := h.dict.Create(c.Request.Context(), d, lines); err != nil {
			continue
		}
		totalCount++
	}
	if totalCount == 0 {
		errResp(c, http.StatusInternalServerError, "重置内置字典失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "count": totalCount})
}

func splitLines(s string) []string {
	raw := strings.Split(s, "\n")
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
