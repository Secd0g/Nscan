package repositories

import (
	"context"
	"time"

	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type SettingsRepo struct {
	coll *mongo.Collection
}

func NewSettingsRepo(db *mongo.Database) *SettingsRepo {
	return &SettingsRepo{coll: db.Collection("settings")}
}

func (r *SettingsRepo) GetValue(ctx context.Context, key string) (string, error) {
	var doc struct {
		Value string `bson:"value"`
	}
	err := r.coll.FindOne(ctx, bson.M{"key": key}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return "", nil
	}
	return doc.Value, err
}

func (r *SettingsRepo) SetValue(ctx context.Context, key, value string) error {
	filter := bson.M{"key": key}
	update := bson.M{
		"$set":         bson.M{"value": value, "updated_at": time.Now()},
		"$setOnInsert": bson.M{"key": key},
	}
	opts := options.Update().SetUpsert(true)
	_, err := r.coll.UpdateOne(ctx, filter, update, opts)
	return err
}

func (r *SettingsRepo) GetProviderConfig(ctx context.Context, key string) (*models.ProviderConfig, error) {
	var cfg models.ProviderConfig
	err := r.coll.FindOne(ctx, bson.M{"key": key}).Decode(&cfg) // @check-ignore: global settings: no user scope
	if err == mongo.ErrNoDocuments {
		return &models.ProviderConfig{Key: key, Providers: map[string][]string{}, Enabled: map[string]bool{}}, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *SettingsRepo) SaveProviderConfig(ctx context.Context, cfg *models.ProviderConfig) error {
	cfg.UpdatedAt = time.Now()
	filter := bson.M{"key": cfg.Key}
	update := bson.M{
		"$set": bson.M{
			"providers":  cfg.Providers,
			"enabled":    cfg.Enabled,
			"updated_at": cfg.UpdatedAt,
		},
		"$setOnInsert": bson.M{
			"key": cfg.Key,
		},
	}
	opts := options.Update().SetUpsert(true)
	_, err := r.coll.UpdateOne(ctx, filter, update, opts)
	return err
}
