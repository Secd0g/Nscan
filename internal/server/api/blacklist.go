package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (h *Handler) ListBlacklist(c *gin.Context) {
	list, err := h.blacklist.List(c.Request.Context())
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []models.BlacklistEntry{}
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *Handler) AddBlacklist(c *gin.Context) {
	var body struct {
		Type   string `json:"type"`
		Value  string `json:"value"`
		Remark string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	body.Value = strings.TrimSpace(body.Value)
	if body.Value == "" {
		errResp(c, http.StatusBadRequest, "value is required")
		return
	}
	if body.Type == "" {
		body.Type = "domain"
	}
	entry := &models.BlacklistEntry{Type: body.Type, Value: body.Value, Remark: body.Remark}
	if err := h.blacklist.Add(c.Request.Context(), entry); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": entry})
}

func (h *Handler) BatchAddBlacklist(c *gin.Context) {
	var body struct {
		Items []struct {
			Type   string `json:"type"`
			Value  string `json:"value"`
			Remark string `json:"remark"`
		} `json:"items"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	entries := make([]models.BlacklistEntry, 0, len(body.Items))
	for _, item := range body.Items {
		v := strings.TrimSpace(item.Value)
		if v == "" {
			continue
		}
		t := item.Type
		if t == "" {
			t = "domain"
		}
		entries = append(entries, models.BlacklistEntry{Type: t, Value: v, Remark: item.Remark})
	}
	if err := h.blacklist.BatchAdd(c.Request.Context(), entries); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "count": len(entries)})
}

func (h *Handler) RemoveBlacklist(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.blacklist.Remove(c.Request.Context(), id); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) ClearBlacklist(c *gin.Context) {
	if err := h.blacklist.Clear(c.Request.Context()); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
