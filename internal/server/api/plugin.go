package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	pluginruntime "github.com/yourname/nscan/internal/scanner/plugins/runtime"
	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (h *Handler) ListPlugins(c *gin.Context) {
	module := c.Query("module")
	list, err := h.plugins.List(c.Request.Context(), module)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *Handler) GetPlugin(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	p, err := h.plugins.GetByID(c.Request.Context(), id)
	if err != nil {
		errResp(c, http.StatusNotFound, "plugin not found")
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) CreatePlugin(c *gin.Context) {
	var p models.Plugin
	if err := c.ShouldBindJSON(&p); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	if p.Name == "" || p.Module == "" {
		errResp(c, http.StatusBadRequest, "name and module required")
		return
	}
	p.Enabled = true
	if err := h.plugins.Create(c.Request.Context(), &p); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) UpdatePlugin(c *gin.Context) {
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
	delete(body, "created_at")
	if len(body) == 0 {
		errResp(c, http.StatusBadRequest, "nothing to update")
		return
	}
	if err := h.plugins.Update(c.Request.Context(), id, bson.M(body)); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) DeletePlugin(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.plugins.Delete(c.Request.Context(), id); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// UploadPlugin accepts a .go source file (multipart or raw body), runs sandbox
// pre-check, and stores the plugin in the database.
func (h *Handler) UploadPlugin(c *gin.Context) {
	// Support both multipart and raw body upload
	var src string
	file, _, err := c.Request.FormFile("source")
	if err == nil {
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			errResp(c, http.StatusBadRequest, "failed to read file")
			return
		}
		src = string(data)
	} else {
		data, err := io.ReadAll(c.Request.Body)
		if err != nil || len(data) == 0 {
			errResp(c, http.StatusBadRequest, "source file required")
			return
		}
		src = string(data)
	}

	// Sandbox import check
	if err := pluginruntime.ValidateImports(src); err != nil {
		errResp(c, http.StatusBadRequest, "sandbox violation: "+err.Error())
		return
	}

	// Try to load and obtain manifest
	stage, err := pluginruntime.LoadFromSource(src)
	if err != nil {
		errResp(c, http.StatusBadRequest, "plugin compile error: "+err.Error())
		return
	}
	manifest := stage.GetManifest()
	if manifest.Name == "" {
		errResp(c, http.StatusBadRequest, "plugin manifest must have a Name")
		return
	}
	manifestJSON, _ := json.Marshal(manifest)

	p := &models.Plugin{
		Name:         manifest.Name,
		Version:      manifest.Version,
		Author:       manifest.Author,
		Description:  manifest.Description,
		Module:       manifest.Capability,
		Category:     manifest.Capability,
		SourceCode:   src,
		ManifestJSON: string(manifestJSON),
		Enabled:      true,
		Builtin:      false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := h.plugins.Create(c.Request.Context(), p); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, p)
}
