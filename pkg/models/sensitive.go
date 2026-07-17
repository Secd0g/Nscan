package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SensitiveRule 敏感信息识别规则（正则匹配 http 响应体/头）
type SensitiveRule struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"     json:"id"`
	UserID    primitive.ObjectID `bson:"user_id"       json:"user_id"`
	Name        string             `bson:"name"              json:"name"`
	Pattern     string             `bson:"pattern"           json:"pattern"`     // 正则表达式
	Color       string             `bson:"color,omitempty"   json:"color,omitempty"`
	Description string             `bson:"description"       json:"description"`
	Severity    string             `bson:"severity"          json:"severity"` // low | medium | high | critical
	Builtin     bool               `bson:"builtin"           json:"builtin"`
	Active      bool               `bson:"active"            json:"active"`
	CreatedAt   time.Time          `bson:"created_at"        json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at"        json:"updated_at"`
}

// SensitiveAsset 敏感信息命中结果（作为扫描资产落库）
type SensitiveAsset struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"    json:"id"`
	UserID    primitive.ObjectID `bson:"user_id"       json:"user_id"`
	TaskID      string             `bson:"task_id"          json:"task_id"`
	ProjectID   string             `bson:"project_id"       json:"project_id"`
	URL         string             `bson:"url"              json:"url"`
	RuleID      string             `bson:"rule_id"          json:"rule_id"`
	RuleName    string             `bson:"rule_name"        json:"rule_name"`
	Severity    string             `bson:"severity"         json:"severity"`
	Matched     string             `bson:"matched"          json:"matched"`                   // 命中的字符串（截断）
	Context     string             `bson:"context,omitempty" json:"context,omitempty"`         // 前后文
	Source      string             `bson:"source,omitempty" json:"source,omitempty"`           // "regex" | "trufflehog"
	Verified    *bool              `bson:"verified,omitempty" json:"verified,omitempty"`        // nil=未验证, true/false=验证结果
	DetectorID  string             `bson:"detector_id,omitempty" json:"detector_id,omitempty"` // TruffleHog detector 标识
	CreatedAt   time.Time          `bson:"created_at"       json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at"       json:"updated_at"`
}
