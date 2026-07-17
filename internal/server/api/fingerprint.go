package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	nsserver "github.com/yourname/nscan/internal/server"
	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (h *Handler) ListFingerprints(c *gin.Context) {
	limit, skip := paginate(c)
	filter := bson.M{}
	if v := c.Query("keyword"); v != "" {
		filter["$or"] = []bson.M{
			{"name": bson.M{"$regex": v, "$options": "i"}},
			{"company": bson.M{"$regex": v, "$options": "i"}},
			{"keyword": bson.M{"$regex": v, "$options": "i"}},
		}
	}
	if v := c.Query("parent_category"); v != "" {
		filter["parent_category"] = v
	}
	if v := c.Query("location"); v != "" {
		filter["location"] = v
	}
	if v := c.Query("fp_type"); v != "" {
		filter["fp_type"] = v
	}
	if v := c.Query("enabled"); v == "true" {
		filter["enabled"] = true
	} else if v == "false" {
		filter["enabled"] = false
	}
	list, total, err := h.fingerprint.List(c.Request.Context(), filter, limit, skip)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	listResp(c, list, total)
}

func (h *Handler) FingerprintCategories(c *gin.Context) {
	cats, err := h.fingerprint.Categories(c.Request.Context())
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, cats)
}

func (h *Handler) CreateFingerprint(c *gin.Context) {
	var fp models.Fingerprint
	if err := c.ShouldBindJSON(&fp); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.fingerprint.Create(c.Request.Context(), &fp); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, fp)
}

func (h *Handler) UpdateFingerprint(c *gin.Context) {
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
	if err := h.fingerprint.Update(c.Request.Context(), id, body); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) DeleteFingerprint(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.fingerprint.Delete(c.Request.Context(), id); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) ClearFingerprints(c *gin.Context) {
	if err := h.fingerprint.Clear(c.Request.Context()); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) SyncFingerprintsOnline(c *gin.Context) {
	fps, err := nsserver.LoadEmbeddedFingerprints()
	if err != nil {
		errResp(c, http.StatusInternalServerError, "加载内置指纹数据失败: "+err.Error())
		return
	}
	if err := h.fingerprint.Clear(c.Request.Context()); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	count, err := h.fingerprint.BatchInsert(c.Request.Context(), fps)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "count": count})
}

func (h *Handler) ImportFingerprints(c *gin.Context) {
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

	fpType := c.PostForm("fp_type")
	if fpType == "" {
		fpType = "passive"
	}

	var raw []models.Fingerprint
	if err := json.Unmarshal(data, &raw); err != nil {
		errResp(c, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	for i := range raw {
		raw[i].FpType = fpType
		raw[i].Enabled = true
		raw[i].Builtin = true
	}

	count, err := h.fingerprint.BatchInsert(c.Request.Context(), raw)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "count": count})
}
