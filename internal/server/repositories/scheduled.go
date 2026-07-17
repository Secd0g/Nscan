package repositories

import (
	"context"
	"time"

	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ScheduledRepo struct {
	coll *mongo.Collection
}

func NewScheduledRepo(db *mongo.Database) *ScheduledRepo {
	return &ScheduledRepo{coll: db.Collection("scheduled_jobs")}
}

func (r *ScheduledRepo) Create(ctx context.Context, j *models.ScheduledJob) error {
	now := time.Now()
	j.CreatedAt = now
	j.UpdatedAt = now
	res, err := r.coll.InsertOne(ctx, j)
	if err != nil {
		return err
	}
	j.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *ScheduledRepo) List(ctx context.Context, limit, skip int64) ([]models.ScheduledJob, int64, error) {
	filter := bson.M{}
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	opts := options.Find().SetLimit(limit).SetSkip(skip).SetSort(bson.D{{Key: "created_at", Value: -1}})
	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var list []models.ScheduledJob
	if err := cursor.All(ctx, &list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *ScheduledRepo) ListForUser(ctx context.Context, uid primitive.ObjectID, limit, skip int64) ([]models.ScheduledJob, int64, error) {
	filter := bson.M{"user_id": uid}
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil { return nil, 0, err }
	cursor, err := r.coll.Find(ctx, filter, options.Find().SetLimit(limit).SetSkip(skip).SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil { return nil, 0, err }
	defer cursor.Close(ctx)
	var list []models.ScheduledJob
	if err := cursor.All(ctx, &list); err != nil { return nil, 0, err }
	return list, total, nil
}

// ListDue 返回所有启用且下次运行时间已到（<= now）的任务，供调度循环使用。
func (r *ScheduledRepo) ListDue(ctx context.Context, now time.Time) ([]models.ScheduledJob, error) {
	filter := bson.M{
		"enabled":  true,
		"next_run": bson.M{"$ne": nil, "$lte": now},
	}
	cursor, err := r.coll.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var list []models.ScheduledJob
	if err := cursor.All(ctx, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (r *ScheduledRepo) GetByID(ctx context.Context, id primitive.ObjectID) (*models.ScheduledJob, error) {
	var j models.ScheduledJob
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&j); err != nil {
		return nil, err
	}
	return &j, nil
}

func (r *ScheduledRepo) GetByIDForUser(ctx context.Context, id, uid primitive.ObjectID) (*models.ScheduledJob, error) {
	var j models.ScheduledJob
	if err := r.coll.FindOne(ctx, bson.M{"_id": id, "user_id": uid}).Decode(&j); err != nil { return nil, err }
	return &j, nil
}

// GetByIDAdmin is an alias for GetByID (kept for backward compatibility).
func (r *ScheduledRepo) GetByIDAdmin(ctx context.Context, id primitive.ObjectID) (*models.ScheduledJob, error) {
	return r.GetByID(ctx, id)
}

func (r *ScheduledRepo) Update(ctx context.Context, id primitive.ObjectID, fields bson.M) error {
	fields["updated_at"] = time.Now()
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": fields})
	return err
}

func (r *ScheduledRepo) UpdateForUser(ctx context.Context, id, uid primitive.ObjectID, fields bson.M) error {
	fields["updated_at"] = time.Now()
	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": id, "user_id": uid}, bson.M{"$set": fields})
	if err == nil && res.MatchedCount == 0 { return mongo.ErrNoDocuments }
	return err
}

// UpdateAdmin is an alias for Update (kept for backward compatibility).
func (r *ScheduledRepo) UpdateAdmin(ctx context.Context, id primitive.ObjectID, fields bson.M) error {
	return r.Update(ctx, id, fields)
}

// MarkRun 记录一次触发：更新 last_run / next_run，并递增 run_count。
func (r *ScheduledRepo) MarkRun(ctx context.Context, id primitive.ObjectID, lastRun time.Time, nextRun *time.Time) error {
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set": bson.M{"last_run": lastRun, "next_run": nextRun, "updated_at": time.Now()},
		"$inc": bson.M{"run_count": 1},
	})
	return err
}

func (r *ScheduledRepo) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *ScheduledRepo) DeleteForUser(ctx context.Context, id, uid primitive.ObjectID) error {
	res, err := r.coll.DeleteOne(ctx, bson.M{"_id": id, "user_id": uid})
	if err == nil && res.DeletedCount == 0 { return mongo.ErrNoDocuments }
	return err
}
