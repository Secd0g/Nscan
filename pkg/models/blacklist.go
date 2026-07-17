package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type BlacklistEntry struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    primitive.ObjectID `bson:"user_id"       json:"user_id"`
	Type      string             `bson:"type"          json:"type"`
	Value     string             `bson:"value"         json:"value"`
	Remark    string             `bson:"remark"        json:"remark"`
	CreatedAt time.Time          `bson:"created_at"    json:"created_at"`
}
