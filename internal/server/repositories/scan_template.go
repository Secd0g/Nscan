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

type ScanTemplateRepo struct {
	coll *mongo.Collection
}

func NewScanTemplateRepo(db *mongo.Database) *ScanTemplateRepo {
	return &ScanTemplateRepo{coll: db.Collection("scan_templates")}
}

func (r *ScanTemplateRepo) Create(ctx context.Context, t *models.ScanTemplate) error {
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	res, err := r.coll.InsertOne(ctx, t)
	if err != nil {
		return err
	}
	t.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *ScanTemplateRepo) List(ctx context.Context, limit, skip int64) ([]models.ScanTemplate, int64, error) {
	filter := bson.M{}
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	opts := options.Find().SetLimit(limit).SetSkip(skip).SetSort(bson.D{{Key: "updated_at", Value: -1}})
	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var list []models.ScanTemplate
	if err := cursor.All(ctx, &list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *ScanTemplateRepo) GetByID(ctx context.Context, id primitive.ObjectID) (*models.ScanTemplate, error) {
	var t models.ScanTemplate
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *ScanTemplateRepo) Update(ctx context.Context, id primitive.ObjectID, fields bson.M) error {
	fields["updated_at"] = time.Now()
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": fields})
	return err
}

func (r *ScanTemplateRepo) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *ScanTemplateRepo) BatchDelete(ctx context.Context, ids []primitive.ObjectID) (int64, error) {
	res, err := r.coll.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}
