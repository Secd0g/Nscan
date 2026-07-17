package repositories

import (
	"context"
	"time"

	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type NotifyRepo struct {
	coll *mongo.Collection
}

func NewNotifyRepo(db *mongo.Database) *NotifyRepo {
	return &NotifyRepo{coll: db.Collection("notify_channels")}
}

// All 返回所有渠道配置。
func (r *NotifyRepo) All(ctx context.Context) ([]models.NotifyChannel, error) {
	cursor, err := r.coll.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var list []models.NotifyChannel
	if err := cursor.All(ctx, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// Get 按 key 获取单个渠道；不存在返回 mongo.ErrNoDocuments。
func (r *NotifyRepo) Get(ctx context.Context, key string) (*models.NotifyChannel, error) {
	var ch models.NotifyChannel
	if err := r.coll.FindOne(ctx, bson.M{"key": key}).Decode(&ch); err != nil {
		return nil, err
	}
	return &ch, nil
}

// Upsert 按 key 写入/更新渠道配置。
func (r *NotifyRepo) Upsert(ctx context.Context, ch *models.NotifyChannel) error {
	ch.UpdatedAt = time.Now()
	_, err := r.coll.UpdateOne(ctx,
		bson.M{"key": ch.Key},
		bson.M{"$set": bson.M{
			"key":        ch.Key,
			"enabled":    ch.Enabled,
			"events":     ch.Events,
			"config":     ch.Config,
			"updated_at": ch.UpdatedAt,
		}},
		options.Update().SetUpsert(true),
	)
	return err
}

// EnabledForEvent 返回启用且订阅了指定事件的渠道。
func (r *NotifyRepo) EnabledForEvent(ctx context.Context, event string) ([]models.NotifyChannel, error) {
	filter := bson.M{"enabled": true, "events": event}
	cursor, err := r.coll.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var list []models.NotifyChannel
	if err := cursor.All(ctx, &list); err != nil {
		return nil, err
	}
	return list, nil
}
