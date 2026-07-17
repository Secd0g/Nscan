package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Dict struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"     json:"id"`
	UserID    primitive.ObjectID `bson:"user_id"       json:"user_id"`
	Category    string             `bson:"category"          json:"category"`
	Service     string             `bson:"service,omitempty" json:"service,omitempty"` // 协议关联，password 类下用（ssh/ftp/...），空表示通用
	Kind        string             `bson:"kind,omitempty"    json:"kind,omitempty"`    // password 类下: users | passwords
	Name        string             `bson:"name"              json:"name"`
	Description string             `bson:"description"       json:"description"`
	Count       int                `bson:"count"             json:"count"`
	Builtin     bool               `bson:"builtin"           json:"builtin"`
	Active      bool               `bson:"active"            json:"active"`
	CreatedAt   time.Time          `bson:"created_at"        json:"created_at"`
}
