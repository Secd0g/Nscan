package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (h *Handler) ListScanTemplates(c *gin.Context) {
	_, ok := RequireUser(c)
	if !ok {
		return
	}
	limit, skip := paginate(c)
	list, total, err := h.scanTpl.List(c.Request.Context(), limit, skip)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	listResp(c, list, total)
}

func (h *Handler) CreateScanTemplate(c *gin.Context) {
	_, ok := RequireUser(c)
	if !ok {
		return
	}
	var t models.ScanTemplate
	if err := c.ShouldBindJSON(&t); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	if t.Name == "" {
		errResp(c, http.StatusBadRequest, "name required")
		return
	}
	if err := h.scanTpl.Create(c.Request.Context(), &t); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, t)
}

func (h *Handler) UpdateScanTemplate(c *gin.Context) {
	_, ok := RequireUser(c)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	var body struct {
		Name        *string                         `json:"name"`
		Description *string                         `json:"description"`
		Modules     map[string][]models.StagePlugin `json:"modules"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	fields := bson.M{}
	if body.Name != nil {
		fields["name"] = *body.Name
	}
	if body.Description != nil {
		fields["description"] = *body.Description
	}
	if body.Modules != nil {
		fields["modules"] = body.Modules
	}
	if len(fields) == 0 {
		errResp(c, http.StatusBadRequest, "nothing to update")
		return
	}
	if err := h.scanTpl.Update(c.Request.Context(), id, fields); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) DeleteScanTemplate(c *gin.Context) {
	_, ok := RequireUser(c)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.scanTpl.Delete(c.Request.Context(), id); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) BatchDeleteScanTemplates(c *gin.Context) {
	_, ok := RequireUser(c)
	if !ok {
		return
	}
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		errResp(c, http.StatusBadRequest, "ids required")
		return
	}
	if checkBatchLimit(c, req.IDs) {
		return
	}
	oids := make([]primitive.ObjectID, 0, len(req.IDs))
	for _, s := range req.IDs {
		oid, err := primitive.ObjectIDFromHex(s)
		if err != nil {
			continue
		}
		oids = append(oids, oid)
	}
	n, err := h.scanTpl.BatchDelete(c.Request.Context(), oids)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": n})
}
