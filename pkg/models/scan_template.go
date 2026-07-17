package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// StagePlugin 模版中某阶段选用的插件及其参数值
type StagePlugin struct {
	PluginID string                 `bson:"plugin_id" json:"plugin_id"`
	Name     string                 `bson:"name"      json:"name"`
	Enabled  bool                   `bson:"enabled"   json:"enabled"`
	Params   map[string]interface{} `bson:"params"    json:"params"`
}

// ScanTemplate 扫描模版，stages 改为 map[module] → []StagePlugin
type ScanTemplate struct {
	ID          primitive.ObjectID        `bson:"_id,omitempty" json:"id"`
	Name        string                    `bson:"name"          json:"name"`
	Description string                    `bson:"description"   json:"description"`
	Modules     map[string][]StagePlugin  `bson:"modules"       json:"modules"`
	CreatedAt   time.Time                 `bson:"created_at"    json:"created_at"`
	UpdatedAt   time.Time                 `bson:"updated_at"    json:"updated_at"`
}
