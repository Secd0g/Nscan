package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type NucleiTemplate struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TemplateID  string             `bson:"template_id"   json:"template_id"`
	Name        string             `bson:"name"          json:"name"`
	Severity    string             `bson:"severity"      json:"severity"`
	Category    string             `bson:"category"      json:"category"`
	Author      string             `bson:"author"        json:"author"`
	Tags        []string           `bson:"tags"          json:"tags"`
	Description string             `bson:"description"   json:"description"`
	Content     string             `bson:"content"       json:"content"`
	CreatedAt   time.Time          `bson:"created_at"    json:"created_at"`
}

type CustomPoc struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TemplateID string             `bson:"template_id"   json:"template_id"`
	Name       string             `bson:"name"          json:"name"`
	Severity   string             `bson:"severity"      json:"severity"`
	Author     string             `bson:"author"        json:"author"`
	Tags       []string           `bson:"tags"          json:"tags"`
	Description string            `bson:"description"   json:"description"`
	Content    string             `bson:"content"       json:"content"`
	Enabled    bool               `bson:"enabled"       json:"enabled"`
	CreatedAt  time.Time          `bson:"created_at"    json:"created_at"`
}

type TemplateStats struct {
	Total    int64 `bson:"total"    json:"total"`
	Critical int64 `bson:"critical" json:"critical"`
	High     int64 `bson:"high"     json:"high"`
	Medium   int64 `bson:"medium"   json:"medium"`
	Low      int64 `bson:"low"      json:"low"`
	Info     int64 `bson:"info"     json:"info"`
}
