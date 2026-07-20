package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SubdomainAsset 子域名资产
type SubdomainAsset struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TaskID    string             `bson:"task_id"       json:"task_id"`
	ProjectID string             `bson:"project_id"    json:"project_id"`
	Domain    string             `bson:"domain"        json:"domain"`
	IPs       []string           `bson:"ips"           json:"ips"`
	Sources   []string           `bson:"sources"       json:"sources"` // 所有发现来源，多工具累积
	CreatedAt time.Time          `bson:"created_at"    json:"created_at"`
}

// PortAsset 端口资产
type PortAsset struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TaskID    string             `bson:"task_id"       json:"task_id"`
	ProjectID string             `bson:"project_id"    json:"project_id"`
	IP        string             `bson:"ip"            json:"ip"`
	Port      int                `bson:"port"          json:"port"`
	Protocol  string             `bson:"protocol"      json:"protocol"` // "tcp" | "udp"
	State     string             `bson:"state"         json:"state"`    // "open"
	Service   string             `bson:"service"       json:"service"`
	Banner    string             `bson:"banner"        json:"banner"`
	Products  []string           `bson:"-"             json:"products"` // 来自 HTTP 资产的 tech，$lookup 填充
	Domains   []string           `bson:"-"             json:"domains"`  // 同一 IP:port 上的全部 HTTP 域名
	Sources   []string           `bson:"sources"       json:"sources"`  // 所有发现来源，多工具累积
	CreatedAt time.Time          `bson:"created_at"    json:"created_at"`
}

// ── IP 聚合模型 ───────────────────────────────────────────────────────────────

// IPAsset 按 IP 聚合的资产视图（预聚合集合 assets_ip）
type IPAsset struct {
	ID    primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	IP    string             `bson:"ip"            json:"ip"`
	Ports []IPPort           `bson:"ports"         json:"ports"`
	Time  time.Time          `bson:"time"          json:"time"`
}

// IPPort 单个端口下挂的服务列表
type IPPort struct {
	Port   int        `bson:"port"   json:"port"`
	Server []IPServer `bson:"server" json:"server"`
}

// IPServer 一个端口下的单个服务记录
type IPServer struct {
	Domain    string   `bson:"domain"    json:"domain"`
	Service   string   `bson:"service"   json:"service"`
	WebServer string   `bson:"webServer" json:"webServer"`
	Products  []string `bson:"products"  json:"products"`
}

// IPAssetFlat 拍平后的行（API 返回给前端，支持合并单元格）
type IPAssetFlat struct {
	IP          string   `json:"ip"`
	IPRowSpan   int      `json:"ipRowSpan"`
	Port        int      `json:"port"`
	PortRowSpan int      `json:"portRowSpan"`
	Domain      string   `json:"domain"`
	Service     string   `json:"service"`
	WebServer   string   `json:"webServer"`
	Products    []string `json:"products"`
	Time        string   `json:"time"`
}

// HTTPAsset HTTP 服务资产
type HTTPAsset struct {
	ID             primitive.ObjectID `bson:"_id,omitempty"  json:"id"`
	TaskID         string             `bson:"task_id"        json:"task_id"`
	ProjectID      string             `bson:"project_id"     json:"project_id"`
	URL            string             `bson:"url"            json:"url"`
	Domain         string             `bson:"domain"         json:"domain"`
	IP             string             `bson:"ip"             json:"ip"`
	Port           int                `bson:"port"           json:"port"`
	StatusCode     int                `bson:"status_code"    json:"status_code"`
	Title          string             `bson:"title"          json:"title"`
	Tech           []string           `bson:"tech"           json:"tech"`
	Banner         string             `bson:"banner"         json:"banner"`
	ContentLen     int64              `bson:"content_len"    json:"content_len"`
	ScreenshotPath string             `bson:"screenshot"     json:"screenshot"`
	ScreenshotPNG  []byte             `bson:"-"              json:"screenshot_png,omitempty"`
	Source         string             `bson:"source,omitempty" json:"source,omitempty"` // 首次发现工具: httpx / fofa / hunter ...
	CreatedAt      time.Time          `bson:"created_at"     json:"created_at"`
}

// CrawlerAsset 爬虫抓取页面资产
type CrawlerAsset struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"    json:"id"`
	TaskID      string             `bson:"task_id"          json:"task_id"`
	ProjectID   string             `bson:"project_id"       json:"project_id"`
	URL         string             `bson:"url"              json:"url"`
	StatusCode  int                `bson:"status_code"      json:"status_code"`
	ContentType string             `bson:"content_type"     json:"content_type"`
	ContentLen  int                `bson:"content_len"      json:"content_len"`
	Title       string             `bson:"title,omitempty"  json:"title,omitempty"`
	Depth       int                `bson:"depth"            json:"depth"`
	Source      string             `bson:"source,omitempty" json:"source,omitempty"` // "static" | "headless" | "pdf"
	CreatedAt   time.Time          `bson:"created_at"       json:"created_at"`
}

// DirAsset 目录扫描资产
type DirAsset struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TaskID      string             `bson:"task_id"       json:"task_id"`
	ProjectID   string             `bson:"project_id"    json:"project_id"`
	URL         string             `bson:"url"           json:"url"`
	Path        string             `bson:"path"          json:"path"`
	StatusCode  int                `bson:"status_code"   json:"status_code"`
	ContentLen  int                `bson:"content_len"   json:"content_len"`
	ContentType string             `bson:"content_type"  json:"content_type"`
	RedirectURL string             `bson:"redirect_url"  json:"redirect_url,omitempty"`
	CreatedAt   time.Time          `bson:"created_at"    json:"created_at"`
}

// VulnAsset 漏洞资产
type VulnAsset struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TaskID     string             `bson:"task_id"       json:"task_id"`
	ProjectID  string             `bson:"project_id"    json:"project_id"`
	Target     string             `bson:"target"        json:"target"`
	TemplateID string             `bson:"template_id"   json:"template_id"`
	Name       string             `bson:"name"          json:"name"`
	Severity   string             `bson:"severity"      json:"severity"` // "critical"|"high"|"medium"|"low"|"info"
	MatchedAt  string             `bson:"matched_at"    json:"matched_at"`
	Status     int                `bson:"status"        json:"status"`
	Request    string             `bson:"request,omitempty"  json:"request,omitempty"`
	Response   string             `bson:"response,omitempty" json:"response,omitempty"`
	CreatedAt  time.Time          `bson:"created_at"    json:"created_at"`
	UpdatedAt  time.Time          `bson:"updated_at"    json:"updated_at"`
}

// PassiveAsset 被动扫描发现的信息泄露资产
type PassiveAsset struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TaskID    string             `bson:"task_id"       json:"task_id"`
	ProjectID string             `bson:"project_id"    json:"project_id"`
	URL       string             `bson:"url"           json:"url"`
	RuleName  string             `bson:"rule_name"     json:"rule_name"`
	Severity  string             `bson:"severity"      json:"severity"`
	Detail    string             `bson:"detail"        json:"detail"`
	Match     string             `bson:"match"         json:"match"`
	CreatedAt time.Time          `bson:"created_at"    json:"created_at"`
}
