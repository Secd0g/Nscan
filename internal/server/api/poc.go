package api

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const nucleiReleasesURL = "https://github.com/projectdiscovery/nuclei-templates/archive/refs/heads/main.zip"

// ── Nuclei 模板 ──────────────────────────────────────────────────────────────

func (h *Handler) ListNucleiTemplates(c *gin.Context) {
	limit, skip := paginate(c)
	filter := bson.M{}
	if v := c.Query("severity"); v != "" {
		filter["severity"] = v
	}
	if v := c.Query("category"); v != "" {
		filter["category"] = v
	}
	if v := c.Query("tag"); v != "" {
		filter["tags"] = v
	}
	if v := c.Query("keyword"); v != "" {
		filter["$or"] = []bson.M{
			{"template_id": bson.M{"$regex": v, "$options": "i"}},
			{"name": bson.M{"$regex": v, "$options": "i"}},
		}
	}
	list, total, err := h.poc.ListTemplates(c.Request.Context(), filter, limit, skip)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	listResp(c, list, total)
}

func (h *Handler) NucleiTemplateStats(c *gin.Context) {
	stats, err := h.poc.TemplateStats(c.Request.Context())
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *Handler) NucleiTemplateCategories(c *gin.Context) {
	cats, err := h.poc.TemplateCategories(c.Request.Context())
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, cats)
}

func (h *Handler) NucleiTemplateContent(c *gin.Context) {
	tpl, err := h.poc.GetTemplateContent(c.Request.Context(), c.Param("id"))
	if err != nil {
		errResp(c, http.StatusNotFound, "template not found")
		return
	}
	c.JSON(http.StatusOK, tpl)
}

func (h *Handler) SyncNucleiTemplates(c *gin.Context) {
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		errResp(c, http.StatusBadRequest, "no file uploaded")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		errResp(c, http.StatusInternalServerError, "read file error")
		return
	}

	count, err := h.importTemplatesFromZip(c.Request.Context(), data)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "count": count})
}

func (h *Handler) SyncNucleiTemplatesOnline(c *gin.Context) {
	ctx := c.Request.Context()

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(nucleiReleasesURL)
	if err != nil {
		errResp(c, http.StatusInternalServerError, "download failed: "+err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		errResp(c, http.StatusInternalServerError, fmt.Sprintf("download returned %d", resp.StatusCode))
		return
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		errResp(c, http.StatusInternalServerError, "read download error: "+err.Error())
		return
	}

	count, err := h.importTemplatesFromZip(ctx, data)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "count": count})
}

func (h *Handler) ClearNucleiTemplates(c *gin.Context) {
	if err := h.poc.ClearTemplates(c.Request.Context()); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) importTemplatesFromZip(ctx context.Context, data []byte) (int, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid zip: %w", err)
	}
	count := 0
	for _, f := range reader.File {
		if f.FileInfo().IsDir() || !strings.HasSuffix(f.Name, ".yaml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		tpl := parseNucleiYAML(string(content), f.Name)
		if tpl == nil {
			continue
		}
		if err := h.poc.UpsertTemplate(ctx, tpl); err != nil {
			continue
		}
		count++
	}
	return count, nil
}

func parseNucleiYAML(content, filename string) *models.NucleiTemplate {
	lines := strings.Split(content, "\n")
	tpl := &models.NucleiTemplate{
		Content:   content,
		CreatedAt: time.Now(),
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "id:") {
			tpl.TemplateID = strings.TrimSpace(strings.TrimPrefix(trimmed, "id:"))
		}
		if strings.HasPrefix(trimmed, "name:") && tpl.Name == "" {
			tpl.Name = strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
		}
		if strings.HasPrefix(trimmed, "severity:") {
			tpl.Severity = strings.TrimSpace(strings.TrimPrefix(trimmed, "severity:"))
		}
		if strings.HasPrefix(trimmed, "author:") {
			tpl.Author = strings.TrimSpace(strings.TrimPrefix(trimmed, "author:"))
		}
		if strings.HasPrefix(trimmed, "tags:") {
			tagStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "tags:"))
			for _, t := range strings.Split(tagStr, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					tpl.Tags = append(tpl.Tags, t)
				}
			}
		}
		if strings.HasPrefix(trimmed, "description:") && tpl.Description == "" {
			tpl.Description = strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
		}
	}
	if tpl.TemplateID == "" {
		return nil
	}
	// derive category from path
	parts := strings.Split(filepath.ToSlash(filename), "/")
	for i, p := range parts {
		if p == "nuclei-templates" || p == "nuclei-templates-main" {
			if i+1 < len(parts)-1 {
				tpl.Category = parts[i+1]
			}
			break
		}
	}
	if tpl.Category == "" && len(parts) >= 2 {
		tpl.Category = parts[len(parts)-2]
	}
	return tpl
}

// ── 自定义 POC ───────────────────────────────────────────────────────────────

func (h *Handler) ListCustomPocs(c *gin.Context) {
	limit, skip := paginate(c)
	filter := bson.M{}
	if v := c.Query("name"); v != "" {
		filter["name"] = bson.M{"$regex": v, "$options": "i"}
	}
	if v := c.Query("template_id"); v != "" {
		filter["template_id"] = bson.M{"$regex": v, "$options": "i"}
	}
	if v := c.Query("severity"); v != "" {
		filter["severity"] = v
	}
	if v := c.Query("enabled"); v == "true" {
		filter["enabled"] = true
	} else if v == "false" {
		filter["enabled"] = false
	}
	list, total, err := h.poc.ListCustom(c.Request.Context(), filter, limit, skip)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	listResp(c, list, total)
}

func (h *Handler) CreateCustomPoc(c *gin.Context) {
	_, ok := RequireUser(c)
	if !ok {
		return
	}
	var poc models.CustomPoc
	if err := c.ShouldBindJSON(&poc); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.poc.CreateCustom(c.Request.Context(), &poc); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, poc)
}

func (h *Handler) UpdateCustomPoc(c *gin.Context) {
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
	if err := h.poc.UpdateCustom(c.Request.Context(), id, bson.M(body)); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) DeleteCustomPoc(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.poc.DeleteCustom(c.Request.Context(), id); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) ClearCustomPocs(c *gin.Context) {
	if err := h.poc.ClearCustom(c.Request.Context()); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) ImportCustomPocs(c *gin.Context) {
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		errResp(c, http.StatusBadRequest, "no file")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		errResp(c, http.StatusInternalServerError, "read error")
		return
	}

	enabledStr := c.PostForm("enabled")
	enabled := enabledStr == "true"

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid zip")
		return
	}

	count := 0
	for _, f := range reader.File {
		if f.FileInfo().IsDir() || !strings.HasSuffix(f.Name, ".yaml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}

		tplInfo := parseNucleiYAML(string(content), f.Name)
		if tplInfo == nil {
			continue
		}

		poc := &models.CustomPoc{
			TemplateID:  tplInfo.TemplateID,
			Name:        tplInfo.Name,
			Severity:    tplInfo.Severity,
			Author:      tplInfo.Author,
			Tags:        tplInfo.Tags,
			Description: tplInfo.Description,
			Content:     tplInfo.Content,
			Enabled:     enabled,
		}
		if err := h.poc.CreateCustom(c.Request.Context(), poc); err != nil {
			continue
		}
		count++
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "count": count})
}

func (h *Handler) ExportCustomPocs(c *gin.Context) {
	list, _, err := h.poc.ListCustom(c.Request.Context(), bson.M{}, 10000, 0)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for _, poc := range list {
		if poc.Content == "" {
			continue
		}
		name := poc.TemplateID
		if name == "" {
			name = poc.ID.Hex()
		}
		fw, err := zw.Create(name + ".yaml")
		if err != nil {
			continue
		}
		fw.Write([]byte(poc.Content))
	}
	zw.Close()

	c.Header("Content-Disposition", "attachment; filename=custom-pocs.zip")
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

