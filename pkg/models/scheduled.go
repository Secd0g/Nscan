package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ScheduledJob 定时扫描任务：按 cron 表达式周期性地创建扫描任务
//
// Modules 是 fire 时下发到 Task 的完整插件配置（和 ScanTemplate.Modules 同构）；
// 有 Modules 时 Stages/Params 由 Modules 推导，仅当兼容旧数据时才直接读取 Stages/Params。
type ScheduledJob struct {
	ID           primitive.ObjectID           `bson:"_id,omitempty"  json:"id"`
	UserID       primitive.ObjectID           `bson:"user_id"         json:"user_id"`
	Name         string                       `bson:"name"           json:"name"`
	ProjectID    primitive.ObjectID           `bson:"project_id"     json:"project_id"`
	ProjectName  string                       `bson:"project_name"   json:"project_name"`
	Cron         string                       `bson:"cron"           json:"cron"` // 标准 5 段 cron
	Targets      []string                     `bson:"targets"        json:"targets"`
	Stages       []string                     `bson:"stages"         json:"stages"`
	Params       map[string]string            `bson:"params"         json:"params"`
	Modules      map[string][]StagePlugin     `bson:"modules"        json:"modules,omitempty"`
	TemplateID   string                       `bson:"template_id"    json:"template_id"`
	TemplateName string                       `bson:"template_name"  json:"template_name"`
	NodeIDs      []string                     `bson:"node_ids"       json:"node_ids,omitempty"`
	Enabled      bool                         `bson:"enabled"        json:"enabled"`
	LastRun      *time.Time                   `bson:"last_run"       json:"last_run"`
	NextRun      *time.Time                   `bson:"next_run"       json:"next_run"`
	RunCount     int                          `bson:"run_count"      json:"run_count"`
	CreatedAt    time.Time                    `bson:"created_at"     json:"created_at"`
	UpdatedAt    time.Time                    `bson:"updated_at"     json:"updated_at"`
}
