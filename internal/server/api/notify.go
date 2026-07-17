package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourname/nscan/pkg/models"
)

var notifyChannelKeys = map[string]bool{"wecom": true, "dingtalk": true, "slack": true, "email": true}

// ListNotify 返回所有渠道配置（前端按 key 归位到表单）。
func (h *Handler) ListNotify(c *gin.Context) {
	list, err := h.notify.All(c.Request.Context())
	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []models.NotifyChannel{}
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

// SaveNotify 保存单个渠道配置。
func (h *Handler) SaveNotify(c *gin.Context) {
	_, ok := RequireUser(c)
	if !ok {
		return
	}
	key := c.Param("key")
	if !notifyChannelKeys[key] {
		errResp(c, http.StatusBadRequest, "unknown channel")
		return
	}
	var req struct {
		Enabled bool              `json:"enabled"`
		Events  []string          `json:"events"`
		Config  map[string]string `json:"config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Config == nil {
		req.Config = map[string]string{}
	}
	if req.Events == nil {
		req.Events = []string{}
	}
	ch := &models.NotifyChannel{Key: key, Enabled: req.Enabled, Events: req.Events, Config: req.Config}
	if err := h.notify.Upsert(c.Request.Context(), ch); err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, ch)
}

// TestNotify 用请求体中的配置立即发送一条测试消息（无需先保存）。
func (h *Handler) TestNotify(c *gin.Context) {
	key := c.Param("key")
	if !notifyChannelKeys[key] {
		errResp(c, http.StatusBadRequest, "unknown channel")
		return
	}
	var req struct {
		Config map[string]string `json:"config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	ch := &models.NotifyChannel{Key: key, Config: req.Config}
	title := "nscan 测试通知"
	body := "这是一条来自 nscan 的测试消息，发送时间 " + time.Now().Format("2006-01-02 15:04:05") + "。若你收到本消息，说明该通知渠道配置正确。"
	if err := h.notifier.SendTo(ch, title, body); err != nil {
		errResp(c, http.StatusBadRequest, "发送失败: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "测试消息已发送"})
}
