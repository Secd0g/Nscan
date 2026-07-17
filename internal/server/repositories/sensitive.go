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

type SensitiveRuleRepo struct {
	col *mongo.Collection
}

func NewSensitiveRuleRepo(db *mongo.Database) *SensitiveRuleRepo {
	return &SensitiveRuleRepo{col: db.Collection("sensitive_rules")}
}

func (r *SensitiveRuleRepo) List(ctx context.Context, filter bson.M, limit, skip int64) ([]models.SensitiveRule, int64, error) {
	total, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	opts := options.Find().SetSort(bson.D{{Key: "builtin", Value: -1}, {Key: "name", Value: 1}}).
		SetLimit(limit).SetSkip(skip)
	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var out []models.SensitiveRule
	if err := cursor.All(ctx, &out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// ListActive 返回所有 active=true 的规则（供 scheduler 注入到扫描任务）
func (r *SensitiveRuleRepo) ListActive(ctx context.Context) ([]models.SensitiveRule, error) {
	cursor, err := r.col.Find(ctx, bson.M{"active": true}) // @check-ignore: global sensitive rules: system-wide config
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var out []models.SensitiveRule
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *SensitiveRuleRepo) Count(ctx context.Context) (int64, error) {
	return r.col.CountDocuments(ctx, bson.M{}) // @check-ignore: global sensitive rules: count all
}

func (r *SensitiveRuleRepo) Create(ctx context.Context, s *models.SensitiveRule) error {
	s.ID = primitive.NewObjectID()
	s.CreatedAt = time.Now()
	s.UpdatedAt = s.CreatedAt
	_, err := r.col.InsertOne(ctx, s)
	return err
}

func (r *SensitiveRuleRepo) Update(ctx context.Context, id primitive.ObjectID, fields bson.M) error {
	fields["updated_at"] = time.Now()
	_, err := r.col.UpdateByID(ctx, id, bson.M{"$set": fields}) // @check-ignore: admin sensitive rule update
	return err
}

func (r *SensitiveRuleRepo) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.col.DeleteOne(ctx, bson.M{"_id": id}) // @check-ignore: admin sensitive rule delete
	return err
}

func (r *SensitiveRuleRepo) BatchInsert(ctx context.Context, rules []models.SensitiveRule) (int, error) {
	if len(rules) == 0 {
		return 0, nil
	}
	docs := make([]interface{}, len(rules))
	now := time.Now()
	for i := range rules {
		if rules[i].ID.IsZero() {
			rules[i].ID = primitive.NewObjectID()
		}
		rules[i].CreatedAt = now
		rules[i].UpdatedAt = now
		docs[i] = rules[i]
	}
	res, err := r.col.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
	if err != nil {
		return 0, err
	}
	return len(res.InsertedIDs), nil
}
