package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Project struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID      primitive.ObjectID `bson:"user_id"       json:"user_id"`
	Name        string             `bson:"name"          json:"name"`
	Description string             `bson:"description"   json:"description"`
	Scope       []string           `bson:"scope"         json:"scope"` // 目标域名/IP 范围
	CreatedAt   time.Time          `bson:"created_at"    json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at"    json:"updated_at"`
}
