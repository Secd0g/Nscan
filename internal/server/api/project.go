package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func (h *Handler) ListProjects(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	limit, skip := paginate(c)
	list, total, err := h.projects.ListForUser(c.Request.Context(), uid, limit, skip)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	listResp(c, list, total)
}

func (h *Handler) CreateProject(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	var req struct {
		Name        string   `json:"name"        binding:"required"`
		Description string   `json:"description"`
		Scope       []string `json:"scope"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	p := &models.Project{
		UserID:      uid,
		Name:        req.Name,
		Description: req.Description,
		Scope:       req.Scope,
	}
	if err := h.projects.Create(c.Request.Context(), p); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusCreated, p)
}

func (h *Handler) GetProject(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	p, err := h.projects.GetByIDForUser(c.Request.Context(), id, uid)
	if err == mongo.ErrNoDocuments {
		errResp(c, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) UpdateProject(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Name        *string  `json:"name"`
		Description *string  `json:"description"`
		Scope       []string `json:"scope"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	fields := bson.M{}
	if req.Name != nil {
		fields["name"] = *req.Name
	}
	if req.Description != nil {
		fields["description"] = *req.Description
	}
	if req.Scope != nil {
		fields["scope"] = req.Scope
	}
	if len(fields) == 0 {
		errResp(c, http.StatusBadRequest, "nothing to update")
		return
	}
	if err := h.projects.UpdateForUser(c.Request.Context(), id, uid, fields); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

func (h *Handler) DeleteProject(c *gin.Context) {
	uid, ok := RequireUser(c)
	if !ok {
		return
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.projects.DeleteForUser(c.Request.Context(), id, uid); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (h *Handler) BatchDeleteProjects(c *gin.Context) {
	uid, ok := RequireUser(c)
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
	ctx := c.Request.Context()
	for _, idStr := range req.IDs {
		id, err := primitive.ObjectIDFromHex(idStr)
		if err != nil {
			continue
		}
		_ = h.projects.DeleteForUser(ctx, id, uid)
	}
	c.JSON(http.StatusOK, gin.H{"deleted": len(req.IDs)})
}
