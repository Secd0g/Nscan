package repositories

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// EnsureUserIndexes creates (user_id, ...) composite indexes on all user-scoped
// collections. Safe to call on every startup — CreateOne is idempotent when the
// index already exists with the same keys.
func EnsureUserIndexes(ctx context.Context, db *mongo.Database) {
	type idx struct {
		coll string
		keys bson.D
		name string
	}
	indexes := []idx{
		// tasks
		{"tasks", bson.D{{Key: "user_id", Value: 1}, {Key: "project_id", Value: 1}, {Key: "status", Value: 1}}, "tasks_user_project_status"},
		{"tasks", bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}}, "tasks_user_ts"},
		// projects
		{"projects", bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}}, "projects_user_ts"},
		// scan_templates
		{"scan_templates", bson.D{{Key: "user_id", Value: 1}, {Key: "updated_at", Value: -1}}, "templates_user_ts"},
		// scheduled_jobs
		{"scheduled_jobs", bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}}, "scheduled_user_ts"},
		// notify_channels
		{"notify_channels", bson.D{{Key: "user_id", Value: 1}, {Key: "key", Value: 1}}, "notify_user_key"},
		// assets — add user_id prefix to existing project-level indexes
		{collSubdomain, bson.D{{Key: "user_id", Value: 1}, {Key: "project_id", Value: 1}, {Key: "created_at", Value: -1}}, "subdomain_user_project_ts"},
		{collPort, bson.D{{Key: "user_id", Value: 1}, {Key: "project_id", Value: 1}, {Key: "created_at", Value: -1}}, "port_user_project_ts"},
		{collHTTP, bson.D{{Key: "user_id", Value: 1}, {Key: "project_id", Value: 1}, {Key: "created_at", Value: -1}}, "http_user_project_ts"},
		{collVuln, bson.D{{Key: "user_id", Value: 1}, {Key: "project_id", Value: 1}, {Key: "created_at", Value: -1}}, "vuln_user_project_ts"},
		{collDir, bson.D{{Key: "user_id", Value: 1}, {Key: "project_id", Value: 1}, {Key: "created_at", Value: -1}}, "dir_user_project_ts"},
		{collSensitive, bson.D{{Key: "user_id", Value: 1}, {Key: "project_id", Value: 1}, {Key: "created_at", Value: -1}}, "sensitive_user_project_ts"},
		// asset_changes
		{collChanges, bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}}, "changes_user_ts"},
		// custom pocs
		{"custom_pocs", bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}}, "custompoc_user_ts"},
	}

	for _, i := range indexes {
		model := mongo.IndexModel{
			Keys:    i.keys,
			Options: options.Index().SetName(i.name),
		}
		if _, err := db.Collection(i.coll).Indexes().CreateOne(ctx, model); err != nil {
			fmt.Printf("[index] WARN: create index %s on %s: %v\n", i.name, i.coll, err)
		}
	}
}
