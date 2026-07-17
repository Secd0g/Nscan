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

type PluginRepo struct {
	coll *mongo.Collection
}

func NewPluginRepo(db *mongo.Database) *PluginRepo {
	return &PluginRepo{coll: db.Collection("plugins")}
}

// DeleteBuiltinByName 删除指定 name 的内置插件（用于清理已废弃的旧内置插件）
func (r *PluginRepo) DeleteBuiltinByName(ctx context.Context, name string) error {
	_, err := r.coll.DeleteMany(ctx, bson.M{"name": name, "builtin": true}) // @check-ignore: builtin plugin cleanup: admin operation
	return err
}

func (r *PluginRepo) Create(ctx context.Context, p *models.Plugin) error {
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()
	res, err := r.coll.InsertOne(ctx, p)
	if err != nil {
		return err
	}
	p.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *PluginRepo) List(ctx context.Context, module string) ([]models.Plugin, error) {
	filter := bson.M{}
	if module != "" {
		filter["module"] = module
	}
	opts := options.Find().SetSort(bson.D{{Key: "module", Value: 1}, {Key: "name", Value: 1}})
	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var list []models.Plugin
	if err := cursor.All(ctx, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (r *PluginRepo) GetByID(ctx context.Context, id primitive.ObjectID) (*models.Plugin, error) {
	var p models.Plugin
	if err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&p); err != nil { // @check-ignore: admin plugin lookup by id
		return nil, err
	}
	return &p, nil
}

func (r *PluginRepo) Update(ctx context.Context, id primitive.ObjectID, fields bson.M) error {
	fields["updated_at"] = time.Now()
	_, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": fields}) // @check-ignore: admin plugin update
	return err
}

func (r *PluginRepo) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id}) // @check-ignore: admin plugin delete
	return err
}

func (r *PluginRepo) UpsertBuiltin(ctx context.Context, p *models.Plugin) error {
	filter := bson.M{"name": p.Name, "module": p.Module, "builtin": true}
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"description": p.Description,
			"version":     p.Version,
			"author":      p.Author,
			"params":      p.Params,
			"builtin":     true,
			"enabled":     p.Enabled,
			"updated_at":  now,
		},
		"$setOnInsert": bson.M{
			"name":       p.Name,
			"module":     p.Module,
			"created_at": now,
		},
	}
	opts := options.Update().SetUpsert(true)
	_, err := r.coll.UpdateOne(ctx, filter, update, opts)
	return err
}
