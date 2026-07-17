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

type TaskRepo struct {
	coll *mongo.Collection
}

func NewTaskRepo(db *mongo.Database) *TaskRepo {
	return &TaskRepo{coll: db.Collection("tasks")}
}

func (r *TaskRepo) Create(ctx context.Context, t *models.Task) error {
	now := time.Now()
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = models.TaskStatusPending
	}
	res, err := r.coll.InsertOne(ctx, t)
	if err != nil {
		return err
	}
	t.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *TaskRepo) List(ctx context.Context, projectID *primitive.ObjectID, status, keyword string, limit, skip int64) ([]models.Task, int64, error) {
	filter := bson.M{}
	if projectID != nil {
		filter["project_id"] = *projectID
	}
	if status != "" {
		filter["status"] = status
	}
	if keyword != "" {
		filter["name"] = bson.M{"$regex": keyword, "$options": "i"}
	}
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
	var list []models.Task
	if err := cursor.All(ctx, &list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *TaskRepo) ListForUser(ctx context.Context, uid primitive.ObjectID, projectID *primitive.ObjectID, status, keyword string, limit, skip int64) ([]models.Task, int64, error) {
	filter := bson.M{"user_id": uid}
	if projectID != nil { filter["project_id"] = *projectID }
	if status != "" { filter["status"] = status }
	if keyword != "" { filter["name"] = bson.M{"$regex": keyword, "$options": "i"} }
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil { return nil, 0, err }
	cursor, err := r.coll.Find(ctx, filter, options.Find().SetLimit(limit).SetSkip(skip).SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil { return nil, 0, err }
	defer cursor.Close(ctx)
	var list []models.Task
	if err := cursor.All(ctx, &list); err != nil { return nil, 0, err }
	return list, total, nil
}

// ListAdmin is an alias for List (kept for backward compatibility with scheduler code).
func (r *TaskRepo) ListAdmin(ctx context.Context, status string, limit, skip int64) ([]models.Task, int64, error) {
	return r.List(ctx, nil, status, "", limit, skip)
}

func (r *TaskRepo) GetByID(ctx context.Context, id primitive.ObjectID) (*models.Task, error) {
	var t models.Task
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TaskRepo) GetByIDForUser(ctx context.Context, id, uid primitive.ObjectID) (*models.Task, error) {
	var t models.Task
	if err := r.coll.FindOne(ctx, bson.M{"_id": id, "user_id": uid}).Decode(&t); err != nil { return nil, err }
	return &t, nil
}

// GetByIDAdmin is an alias for GetByID (kept for backward compatibility).
func (r *TaskRepo) GetByIDAdmin(ctx context.Context, id primitive.ObjectID) (*models.Task, error) {
	return r.GetByID(ctx, id)
}

func (r *TaskRepo) Update(ctx context.Context, id primitive.ObjectID, update bson.M) error {
	update["updated_at"] = time.Now()
	_, err := r.coll.UpdateByID(ctx, id, bson.M{"$set": update})
	return err
}

func (r *TaskRepo) UpdateForUser(ctx context.Context, id, uid primitive.ObjectID, update bson.M) error {
	update["updated_at"] = time.Now()
	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": id, "user_id": uid}, bson.M{"$set": update})
	if err == nil && res.MatchedCount == 0 { return mongo.ErrNoDocuments }
	return err
}

func (r *TaskRepo) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *TaskRepo) DeleteForUser(ctx context.Context, id, uid primitive.ObjectID) error {
	res, err := r.coll.DeleteOne(ctx, bson.M{"_id": id, "user_id": uid})
	if err == nil && res.DeletedCount == 0 { return mongo.ErrNoDocuments }
	return err
}

// DeleteAdmin is an alias for Delete (kept for backward compatibility).
func (r *TaskRepo) DeleteAdmin(ctx context.Context, id primitive.ObjectID) error {
	return r.Delete(ctx, id)
}
