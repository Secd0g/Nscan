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

type ProjectRepo struct {
	coll *mongo.Collection
}

func NewProjectRepo(db *mongo.Database) *ProjectRepo {
	return &ProjectRepo{coll: db.Collection("projects")}
}

func (r *ProjectRepo) Create(ctx context.Context, p *models.Project) error {
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()
	res, err := r.coll.InsertOne(ctx, p)
	if err != nil {
		return err
	}
	p.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *ProjectRepo) List(ctx context.Context, limit, skip int64) ([]models.Project, int64, error) {
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
	var list []models.Project
	if err := cursor.All(ctx, &list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *ProjectRepo) ListForUser(ctx context.Context, uid primitive.ObjectID, limit, skip int64) ([]models.Project, int64, error) {
	filter := bson.M{"user_id": uid}
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil { return nil, 0, err }
	cursor, err := r.coll.Find(ctx, filter, options.Find().SetLimit(limit).SetSkip(skip).SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil { return nil, 0, err }
	defer cursor.Close(ctx)
	var list []models.Project
	if err := cursor.All(ctx, &list); err != nil { return nil, 0, err }
	return list, total, nil
}

func (r *ProjectRepo) GetByID(ctx context.Context, id primitive.ObjectID) (*models.Project, error) {
	var p models.Project
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *ProjectRepo) GetByIDForUser(ctx context.Context, id, uid primitive.ObjectID) (*models.Project, error) {
	var p models.Project
	if err := r.coll.FindOne(ctx, bson.M{"_id": id, "user_id": uid}).Decode(&p); err != nil { return nil, err }
	return &p, nil
}

func (r *ProjectRepo) Update(ctx context.Context, id primitive.ObjectID, fields bson.M) error {
	fields["updated_at"] = time.Now()
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": fields})
	return err
}

func (r *ProjectRepo) UpdateForUser(ctx context.Context, id, uid primitive.ObjectID, fields bson.M) error {
	fields["updated_at"] = time.Now()
	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": id, "user_id": uid}, bson.M{"$set": fields})
	if err == nil && res.MatchedCount == 0 { return mongo.ErrNoDocuments }
	return err
}

func (r *ProjectRepo) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *ProjectRepo) DeleteForUser(ctx context.Context, id, uid primitive.ObjectID) error {
	res, err := r.coll.DeleteOne(ctx, bson.M{"_id": id, "user_id": uid})
	if err == nil && res.DeletedCount == 0 { return mongo.ErrNoDocuments }
	return err
}
