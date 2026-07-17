package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// FieldChange 单个字段的变更
type FieldChange struct {
	Field string `bson:"field" json:"field"`
	Old   string `bson:"old"   json:"old"`
	New   string `bson:"new"   json:"new"`
}

// AssetChangeLog 资产字段变更记录（跨扫描任务对比）
type AssetChangeLog struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"  json:"id"`
	UserID    primitive.ObjectID `bson:"user_id"       json:"user_id"`
	AssetID   primitive.ObjectID `bson:"asset_id"       json:"asset_id"`
	AssetType string             `bson:"asset_type"     json:"asset_type"` // subdomain | port | http
	ProjectID string             `bson:"project_id"     json:"project_id"`
	TaskID    string             `bson:"task_id"        json:"task_id"`
	Changes   []FieldChange      `bson:"changes"        json:"changes"`
	CreatedAt time.Time          `bson:"created_at"     json:"created_at"`
}
