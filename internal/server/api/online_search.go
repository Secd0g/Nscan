package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourname/nscan/pkg/models"
	"github.com/yourname/nscan/pkg/onlinesearch"
)

type onlineSearchReq struct {
	Query string `json:"query" binding:"required"`
	Page  int    `json:"page"`
	Size  int    `json:"size"`
}

type onlineSearchResp struct {
	Provider string                      `json:"provider"`
	Total    int                         `json:"total"`
	Page     int                         `json:"page"`
	Size     int                         `json:"size"`
	Results  []onlinesearch.SearchResult `json:"results"`
}

func (h *Handler) OnlineSearchQuery(c *gin.Context) {
	provider := c.Param("provider")
	var req onlineSearchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Size <= 0 {
		req.Size = 20
	}
	client, err := h.buildOnlineClient(c.Request.Context(), provider)
	if err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
	defer cancel()
	results, total, err := client.Search(ctx, onlinesearch.SearchOptions{
		Query: req.Query, Page: req.Page, Size: req.Size,
	})
	if err != nil {
		errResp(c, http.StatusBadGateway, err.Error())
		return
	}
	c.JSON(http.StatusOK, onlineSearchResp{
		Provider: provider, Total: total, Page: req.Page, Size: req.Size, Results: results,
	})
}

// buildOnlineClient 根据 provider 名字构造客户端，从 settings.online_search 读取 key/version 等
func (h *Handler) buildOnlineClient(ctx context.Context, provider string) (onlinesearch.Client, error) {
	cfg, err := h.settings.GetProviderConfig(ctx, "online_search")
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}
	keys := cfg.Providers[provider]
	if len(keys) == 0 || strings.TrimSpace(keys[0]) == "" {
		return nil, fmt.Errorf("%s 未配置 API Key，请在「插件与节点 → onlinesearch → API 配置」中设置", provider)
	}
	enabled := cfg.Enabled[provider]
	if !enabled {
		return nil, fmt.Errorf("%s 未启用，请在「插件与节点 → onlinesearch → API 配置」中启用", provider)
	}
	switch provider {
	case "fofa":
		return onlinesearch.NewFofa(keys[0]), nil
	case "hunter":
		return onlinesearch.NewHunter(keys[0]), nil
	case "quake":
		return onlinesearch.NewQuake(keys[0]), nil
	case "shodan":
		return onlinesearch.NewShodan(keys[0]), nil
	default:
		return nil, fmt.Errorf("未知 provider: %s", provider)
	}
}

// OnlineSearchImport 将选中的搜索结果导入到项目的 http 资产库
type onlineImportReq struct {
	ProjectID string                      `json:"project_id" binding:"required"`
	Results   []onlinesearch.SearchResult `json:"results"    binding:"required,min=1"`
}

func (h *Handler) OnlineSearchImport(c *gin.Context) {
	provider := c.Param("provider")
	var req onlineImportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errResp(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.ProjectID == "" {
		errResp(c, http.StatusBadRequest, "invalid project_id")
		return
	}
	imported := 0
	skipped := 0
	for _, r := range req.Results {
		if r.URL == "" && r.Host == "" && r.IP == "" {
			skipped++
			continue
		}
		hit := false
		// 1. HTTP 资产（仅当有 URL）
		if r.URL != "" {
			asset := &models.HTTPAsset{
				ProjectID: req.ProjectID,
				URL:       r.URL,
				Domain:    r.Host,
				IP:        r.IP,
				Port:      r.Port,
				Title:     r.Title,
				Banner:    r.Server,
				Source:    provider,
			}
			if err := h.assets.SaveHTTP(c.Request.Context(), asset); err == nil {
				hit = true
			}
		}
		// 2. 端口资产
		if r.IP != "" && r.Port > 0 {
			portAsset := &models.PortAsset{
				ProjectID: req.ProjectID,
				IP:        r.IP,
				Port:      r.Port,
				Protocol:  "tcp",
				State:     "open",
				Service:   r.Protocol,
				Banner:    r.Server,
				Sources:   []string{provider},
			}
			if err := h.assets.SavePort(c.Request.Context(), portAsset); err == nil {
				hit = true
			}
		}
		// 3. 子域名资产（先剥协议/端口/路径再判断）
		if domain := normalizeImportHost(r.Host); isImportDomain(domain) {
			var ips []string
			if r.IP != "" {
				ips = []string{r.IP}
			}
			subAsset := &models.SubdomainAsset{
				ProjectID: req.ProjectID,
				Domain:    domain,
				IPs:       ips,
				Sources:   []string{provider},
			}
			if err := h.assets.SaveSubdomain(c.Request.Context(), subAsset); err == nil {
				hit = true
			}
		}
		if hit {
			imported++
		} else {
			skipped++
		}
	}
	c.JSON(http.StatusOK, gin.H{"imported": imported, "skipped": skipped, "provider": provider})
}

// normalizeImportHost 把 fofa/quake 等返回的 host 字段规整成裸域名，
// 逻辑与 scanner 端 search.normalizeHost 保持一致：剥 scheme / path / 端口。
func normalizeImportHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	if strings.Count(h, ":") == 1 {
		h = h[:strings.Index(h, ":")]
	}
	return h
}

// isImportDomain 判断规整后的 host 是否可入库为子域名（不是 IP、含点、字符集合法）。
func isImportDomain(s string) bool {
	if s == "" || net.ParseIP(s) != nil {
		return false
	}
	if !strings.Contains(s, ".") || strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") {
		return false
	}
	for _, r := range s {
		if !(r == '-' || r == '.' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z')) {
			return false
		}
	}
	return true
}
