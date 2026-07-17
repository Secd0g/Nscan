package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Fingerprint struct {
	ID             primitive.ObjectID `bson:"_id,omitempty"     json:"id"`
	UserID    primitive.ObjectID `bson:"user_id"       json:"user_id"`
	Name           string             `bson:"name"              json:"name"`
	Category       string             `bson:"category"          json:"category"`
	ParentCategory string             `bson:"parent_category"   json:"parent_category"`
	Company        string             `bson:"company"           json:"company"`
	MatchType      string             `bson:"match_type"        json:"match_type"`
	Location       string             `bson:"location"          json:"location"`
	Keyword        string             `bson:"keyword"           json:"keyword"`
	FpType         string             `bson:"fp_type"           json:"fp_type"`
	Enabled        bool               `bson:"enabled"           json:"enabled"`
	Builtin        bool               `bson:"builtin"           json:"builtin"`
	CreatedAt      time.Time          `bson:"created_at"        json:"created_at"`
}
