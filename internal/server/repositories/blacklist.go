package repositories

import (
	"context"
	"time"

	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type BlacklistRepo struct {
	coll *mongo.Collection
}

func NewBlacklistRepo(db *mongo.Database) *BlacklistRepo {
	return &BlacklistRepo{coll: db.Collection("blacklist")}
}

func (r *BlacklistRepo) List(ctx context.Context) ([]models.BlacklistEntry, error) {
	cursor, err := r.coll.Find(ctx, bson.M{}) // @check-ignore: global blacklist: no user scope
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var list []models.BlacklistEntry
	if err := cursor.All(ctx, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (r *BlacklistRepo) Add(ctx context.Context, e *models.BlacklistEntry) error {
	e.ID = primitive.NewObjectID()
	e.CreatedAt = time.Now()
	_, err := r.coll.InsertOne(ctx, e)
	return err
}

func (r *BlacklistRepo) BatchAdd(ctx context.Context, entries []models.BlacklistEntry) error {
	if len(entries) == 0 {
		return nil
	}
	docs := make([]interface{}, len(entries))
	now := time.Now()
	for i := range entries {
		entries[i].ID = primitive.NewObjectID()
		entries[i].CreatedAt = now
		docs[i] = entries[i]
	}
	_, err := r.coll.InsertMany(ctx, docs)
	return err
}

func (r *BlacklistRepo) Remove(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id}) // @check-ignore: blacklist is global admin config, no user scope
	return err
}

func (r *BlacklistRepo) Clear(ctx context.Context) error {
	_, err := r.coll.DeleteMany(ctx, bson.M{}) // @check-ignore: blacklist is global admin config, no user scope
	return err
}

func (r *BlacklistRepo) Contains(ctx context.Context, value string) (bool, error) {
	count, err := r.coll.CountDocuments(ctx, bson.M{"value": value}) // @check-ignore: blacklist is global admin config, no user scope
	return count > 0, err
}

func (r *BlacklistRepo) AllValues(ctx context.Context) ([]string, error) {
	cursor, err := r.coll.Find(ctx, bson.M{}, nil) // @check-ignore: blacklist is global admin config, no user scope
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var entries []models.BlacklistEntry
	if err := cursor.All(ctx, &entries); err != nil {
		return nil, err
	}
	values := make([]string, len(entries))
	for i, e := range entries {
		values[i] = e.Value
	}
	return values, nil
}
