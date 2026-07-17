package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PluginParam 描述插件的一个配置参数，前端根据此 schema 动态渲染表单
type PluginParam struct {
	Key         string        `bson:"key"                   json:"key"`
	Label       string        `bson:"label"                 json:"label"`
	Type        string        `bson:"type"                  json:"type"` // string | number | select | checkbox-group | textarea | switch
	Default     interface{}   `bson:"default,omitempty"     json:"default,omitempty"`
	Options     []ParamOption `bson:"options,omitempty"     json:"options,omitempty"`
	Placeholder string        `bson:"placeholder,omitempty" json:"placeholder,omitempty"`
	Help        string        `bson:"help,omitempty"        json:"help,omitempty"`
	Min         *float64      `bson:"min,omitempty"         json:"min,omitempty"`
	Max         *float64      `bson:"max,omitempty"         json:"max,omitempty"`
	Step        *float64      `bson:"step,omitempty"        json:"step,omitempty"`
	Multiple    bool          `bson:"multiple,omitempty"    json:"multiple,omitempty"`
	Required    bool          `bson:"required,omitempty"    json:"required,omitempty"`
	Group       string        `bson:"group,omitempty"       json:"group,omitempty"` // 参数分组
	Span        int           `bson:"span,omitempty"        json:"span,omitempty"`  // el-col span (1-24)
	// dict-select 类型: 前端按 (DictCategory + DictService + DictKind) 过滤字典
	DictCategory string `bson:"dict_category,omitempty" json:"dict_category,omitempty"`
	DictService  string `bson:"dict_service,omitempty"  json:"dict_service,omitempty"`
	DictKind     string `bson:"dict_kind,omitempty"     json:"dict_kind,omitempty"`
}

// ParamOption select/checkbox-group 的选项
type ParamOption struct {
	Value interface{} `bson:"value" json:"value"`
	Label string      `bson:"label" json:"label"`
}

// Plugin 插件元数据，存储在 MongoDB
type Plugin struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name        string             `bson:"name"          json:"name"`
	Module      string             `bson:"module"        json:"module"` // subdomain | port | http | vuln | dir_scan | url_scan
	Description string             `bson:"description"   json:"description"`
	Version     string             `bson:"version"       json:"version"`
	Author      string             `bson:"author"        json:"author"`
	Params      []PluginParam      `bson:"params"        json:"params"`
	Builtin     bool               `bson:"builtin"       json:"builtin"`
	Enabled     bool               `bson:"enabled"       json:"enabled"`

	// Dynamic plugin fields
	SourceCode   string `bson:"source_code,omitempty"   json:"source_code,omitempty"`   // .go source for interpreted plugins
	ManifestJSON string `bson:"manifest_json,omitempty" json:"manifest_json,omitempty"` // JSON-encoded pluginsdk.Manifest
	Category     string `bson:"category,omitempty"      json:"category,omitempty"`      // subdomain|port|http|vuln|dir|sensitive
	Icon         string `bson:"icon,omitempty"          json:"icon,omitempty"`          // emoji or URL

	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}
