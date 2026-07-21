package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// 通知事件类型
const (
	NotifyEventTaskDone     = "task_done"
	NotifyEventTaskFailed   = "task_failed"
	NotifyEventVulnFound    = "vuln_found"
	NotifyEventAssetChanged = "asset_changed"
	// NotifyEventScanDiff 是一次扫描完成后的新增/变化汇总通知。
	NotifyEventScanDiff = "scan_diff"
)

// NotifyChannel 一个通知渠道的配置（每个渠道一条文档，key 唯一）。
// Config 保存渠道特有字段：
//
//	wecom/slack:  webhook
//	dingtalk:     webhook, secret
//	email:        smtp_host, from, password, to
type NotifyChannel struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	Key       string             `bson:"key"        json:"key"`
	Enabled   bool               `bson:"enabled"    json:"enabled"`
	Events    []string           `bson:"events"     json:"events"`
	Config    map[string]string  `bson:"config"     json:"config"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
}
