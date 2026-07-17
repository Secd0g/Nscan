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

type FingerprintRepo struct {
	col *mongo.Collection
}

func NewFingerprintRepo(db *mongo.Database) *FingerprintRepo {
	return &FingerprintRepo{col: db.Collection("fingerprints")}
}

func (r *FingerprintRepo) List(ctx context.Context, filter bson.M, limit, skip int64) ([]models.Fingerprint, int64, error) {
	total, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "parent_category", Value: 1}, {Key: "name", Value: 1}}).
		SetLimit(limit).SetSkip(skip)
	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var list []models.Fingerprint
	if err := cursor.All(ctx, &list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *FingerprintRepo) Create(ctx context.Context, fp *models.Fingerprint) error {
	fp.ID = primitive.NewObjectID()
	fp.CreatedAt = time.Now()
	_, err := r.col.InsertOne(ctx, fp)
	return err
}

func (r *FingerprintRepo) Update(ctx context.Context, id primitive.ObjectID, fields bson.M) error {
	_, err := r.col.UpdateByID(ctx, id, bson.M{"$set": fields}) // @check-ignore: global system config: fingerprints are admin-managed
	return err
}

func (r *FingerprintRepo) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.col.DeleteOne(ctx, bson.M{"_id": id}) // @check-ignore: global system config: fingerprints are admin-managed
	return err
}

func (r *FingerprintRepo) Clear(ctx context.Context) error {
	_, err := r.col.DeleteMany(ctx, bson.M{}) // @check-ignore: global system config: fingerprints are admin-managed
	return err
}

func (r *FingerprintRepo) Count(ctx context.Context) (int64, error) {
	return r.col.CountDocuments(ctx, bson.M{}) // @check-ignore: global system config: count all
}

func (r *FingerprintRepo) Categories(ctx context.Context) ([]string, error) {
	vals, err := r.col.Distinct(ctx, "parent_category", bson.M{})
	if err != nil {
		return nil, err
	}
	cats := make([]string, 0, len(vals))
	for _, v := range vals {
		if s, ok := v.(string); ok && s != "" {
			cats = append(cats, s)
		}
	}
	return cats, nil
}

func (r *FingerprintRepo) BatchInsert(ctx context.Context, fps []models.Fingerprint) (int, error) {
	if len(fps) == 0 {
		return 0, nil
	}
	docs := make([]interface{}, len(fps))
	now := time.Now()
	for i := range fps {
		fps[i].ID = primitive.NewObjectID()
		fps[i].CreatedAt = now
		docs[i] = fps[i]
	}
	res, err := r.col.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
	if err != nil {
		if res != nil {
			return len(res.InsertedIDs), nil
		}
		return 0, err
	}
	return len(res.InsertedIDs), nil
}
