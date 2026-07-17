package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourname/nscan/pkg/models"
)

func (h *Handler) GetProviderConfig(c *gin.Context) {
	key := c.Param("key")
	cfg, err := h.settings.GetProviderConfig(c.Request.Context(), key)
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, cfg)
}

func (h *Handler) SaveProviderConfig(c *gin.Context) {
	key := c.Param("key")
	var body struct {
		Providers map[string][]string `json:"providers"`
		Enabled   map[string]bool     `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	cfg := &models.ProviderConfig{
		Key:       key,
		Providers: body.Providers,
		Enabled:   body.Enabled,
	}
	if err := h.settings.SaveProviderConfig(c.Request.Context(), cfg); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
