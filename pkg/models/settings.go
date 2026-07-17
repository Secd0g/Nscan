package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ProviderConfig 存储第三方 API 服务的密钥配置（如 subfinder 的 Shodan/Censys 等）
type ProviderConfig struct {
	ID        primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	Key       string              `bson:"key"           json:"key"`
	Providers map[string][]string `bson:"providers"     json:"providers"`
	Enabled   map[string]bool     `bson:"enabled"       json:"enabled"`
	UpdatedAt time.Time           `bson:"updated_at"    json:"updated_at"`
}
