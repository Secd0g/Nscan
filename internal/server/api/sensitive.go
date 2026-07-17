package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (h *Handler) ListSensitiveRules(c *gin.Context) {
	limit, skip := paginate(c)
	filter := bson.M{}
	if v := c.Query("keyword"); v != "" {
		filter["$or"] = []bson.M{
			{"name": bson.M{"$regex": v, "$options": "i"}},
			{"description": bson.M{"$regex": v, "$options": "i"}},
		}
	}
	if v := c.Query("severity"); v != "" {
		filter["severity"] = v
	}
	if v := c.Query("active"); v == "true" {
		filter["active"] = true
	} else if v == "false" {
		filter["active"] = false
	}
	list, total, err := h.sensitive.List(c.Request.Context(), filter, limit, skip)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	listResp(c, list, total)
}

func (h *Handler) CreateSensitiveRule(c *gin.Context) {
	var r models.SensitiveRule
	if err := c.ShouldBindJSON(&r); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.sensitive.Create(c.Request.Context(), &r); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, r)
}

func (h *Handler) UpdateSensitiveRule(c *gin.Context) {
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
	if err := h.sensitive.Update(c.Request.Context(), id, body); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) DeleteSensitiveRule(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		errResp(c, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.sensitive.Delete(c.Request.Context(), id); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
