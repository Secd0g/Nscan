package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourname/nscan/pkg/tooldef"
)

func (h *Handler) ListToolDefs(c *gin.Context) {
	c.JSON(http.StatusOK, tooldef.All)
}
