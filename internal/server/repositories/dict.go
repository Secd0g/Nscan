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

type DictRepo struct {
	meta  *mongo.Collection
	lines *mongo.Collection
}

type DictLine struct {
	ID     primitive.ObjectID `bson:"_id,omitempty"`
	DictID primitive.ObjectID `bson:"dict_id"`
	Line   string             `bson:"line"`
}

func NewDictRepo(db *mongo.Database) *DictRepo {
	return &DictRepo{
		meta:  db.Collection("dicts"),
		lines: db.Collection("dict_lines"),
	}
}

// ListFilter dict 查询过滤条件
type ListFilter struct {
	Category   string // 精确匹配
	Service    string // 精确匹配 service；"*" 表示无过滤（含通用）
	Kind       string // password 类下 users|passwords
	ActiveOnly bool
}

func (r *DictRepo) List(ctx context.Context, category string) ([]models.Dict, error) {
	return r.Query(ctx, ListFilter{Category: category})
}

func (r *DictRepo) Get(ctx context.Context, id primitive.ObjectID) (*models.Dict, error) {
	var d models.Dict
	if err := r.meta.FindOne(ctx, bson.M{"_id": id}).Decode(&d); err != nil { // @check-ignore: dict is global admin config, no user scope
		return nil, err
	}
	return &d, nil
}

func (r *DictRepo) Query(ctx context.Context, f ListFilter) ([]models.Dict, error) {
	filter := bson.M{}
	if f.Category != "" {
		filter["category"] = f.Category
	}
	if f.Service != "" && f.Service != "*" {
		filter["service"] = f.Service
	}
	if f.Kind != "" {
		filter["kind"] = f.Kind
	}
	if f.ActiveOnly {
		filter["active"] = true
	}
	opts := options.Find().SetSort(bson.D{{Key: "builtin", Value: -1}, {Key: "name", Value: 1}})
	cursor, err := r.meta.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var list []models.Dict
	if err := cursor.All(ctx, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (r *DictRepo) Create(ctx context.Context, d *models.Dict, lines []string) error {
	d.ID = primitive.NewObjectID()
	d.CreatedAt = time.Now()
	d.Count = len(lines)
	if _, err := r.meta.InsertOne(ctx, d); err != nil {
		return err
	}
	if len(lines) > 0 {
		docs := make([]interface{}, len(lines))
		for i, l := range lines {
			docs[i] = DictLine{ID: primitive.NewObjectID(), DictID: d.ID, Line: l}
		}
		_, err := r.lines.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
		return err
	}
	return nil
}

func (r *DictRepo) Delete(ctx context.Context, id primitive.ObjectID) error {
	r.lines.DeleteMany(ctx, bson.M{"dict_id": id})     // @check-ignore: dict is global admin config, no user scope
	_, err := r.meta.DeleteOne(ctx, bson.M{"_id": id}) // @check-ignore: dict is global admin config, no user scope
	return err
}

func (r *DictRepo) Update(ctx context.Context, id primitive.ObjectID, fields bson.M) error {
	result, err := r.meta.UpdateOne(ctx, bson.M{
		"_id": id,
		"$or": bson.A{bson.M{"builtin": false}, bson.M{"category": "password"}},
	}, bson.M{"$set": fields}) // @check-ignore: dict is global admin config, no user scope
	if err == nil && result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return err
}

func (r *DictRepo) GetLines(ctx context.Context, dictID primitive.ObjectID, limit, skip int64) ([]string, int64, error) {
	total, err := r.lines.CountDocuments(ctx, bson.M{"dict_id": dictID}) // @check-ignore: dict is global admin config, no user scope
	if err != nil {
		return nil, 0, err
	}
	opts := options.Find().SetLimit(limit).SetSkip(skip).SetProjection(bson.M{"line": 1}) // @check-ignore: dict is global admin config, no user scope
	cursor, err := r.lines.Find(ctx, bson.M{"dict_id": dictID}, opts)                     // @check-ignore: dict is global admin config, no user scope
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var results []DictLine
	if err := cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}
	lines := make([]string, len(results))
	for i, dl := range results {
		lines[i] = dl.Line
	}
	return lines, total, nil
}

// GetContent 返回字典的全部行内容（换行拼接）
func (r *DictRepo) GetContent(ctx context.Context, dictID primitive.ObjectID) (string, error) {
	cursor, err := r.lines.Find(ctx, bson.M{"dict_id": dictID}, options.Find().SetProjection(bson.M{"line": 1})) // @check-ignore: dict is global admin config, no user scope
	if err != nil {
		return "", err
	}
	defer cursor.Close(ctx)
	var docs []DictLine
	if err := cursor.All(ctx, &docs); err != nil {
		return "", err
	}
	parts := make([]string, len(docs))
	for i, d := range docs {
		parts[i] = d.Line
	}
	return joinLines(parts), nil
}

// SetContent 替换字典的行内容（原子替换）
func (r *DictRepo) SetContent(ctx context.Context, dictID primitive.ObjectID, lines []string) error {
	var meta models.Dict
	if err := r.meta.FindOne(ctx, bson.M{"_id": dictID}).Decode(&meta); err != nil { // @check-ignore: dict is global admin config, no user scope
		return err
	}
	if meta.Builtin && meta.Category != "password" {
		return mongo.ErrNoDocuments
	}
	if _, err := r.lines.DeleteMany(ctx, bson.M{"dict_id": dictID}); err != nil { // @check-ignore: dict is global admin config, no user scope
		return err
	}
	if len(lines) > 0 {
		docs := make([]interface{}, len(lines))
		for i, l := range lines {
			docs[i] = DictLine{ID: primitive.NewObjectID(), DictID: dictID, Line: l}
		}
		if _, err := r.lines.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false)); err != nil {
			return err
		}
	}
	_, err := r.meta.UpdateByID(ctx, dictID, bson.M{"$set": bson.M{"count": len(lines)}}) // @check-ignore: dict is global admin config, no user scope
	return err
}

func joinLines(parts []string) string {
	total := 0
	for _, p := range parts {
		total += len(p) + 1
	}
	if total == 0 {
		return ""
	}
	buf := make([]byte, 0, total)
	for i, p := range parts {
		if i > 0 {
			buf = append(buf, '\n')
		}
		buf = append(buf, p...)
	}
	return string(buf)
}

// DeleteBuiltinByCategory 删除某个 category 的所有内置字典（含 lines）
// 用于 sync-online 前的清理，避免产生重复内置项。
func (r *DictRepo) DeleteBuiltinByCategory(ctx context.Context, category string) error {
	filter := bson.M{"category": category, "builtin": true}
	cursor, err := r.meta.Find(ctx, filter, options.Find().SetProjection(bson.M{"_id": 1})) // @check-ignore: dict is global admin config, no user scope
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	var metas []models.Dict
	if err := cursor.All(ctx, &metas); err != nil {
		return err
	}
	ids := make([]primitive.ObjectID, len(metas))
	for i, m := range metas {
		ids[i] = m.ID
	}
	if len(ids) > 0 {
		r.lines.DeleteMany(ctx, bson.M{"dict_id": bson.M{"$in": ids}}) // @check-ignore: dict is global admin config, no user scope
	}
	_, err = r.meta.DeleteMany(ctx, filter)
	return err
}

func (r *DictRepo) Clear(ctx context.Context, category string) error {
	filter := bson.M{"builtin": false}
	if category != "" {
		filter["category"] = category
	}
	cursor, err := r.meta.Find(ctx, filter, options.Find().SetProjection(bson.M{"_id": 1})) // @check-ignore: dict is global admin config, no user scope
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	var metas []models.Dict
	if err := cursor.All(ctx, &metas); err != nil {
		return err
	}
	ids := make([]primitive.ObjectID, len(metas))
	for i, m := range metas {
		ids[i] = m.ID
	}
	if len(ids) > 0 {
		r.lines.DeleteMany(ctx, bson.M{"dict_id": bson.M{"$in": ids}}) // @check-ignore: dict is global admin config, no user scope
	}
	_, err = r.meta.DeleteMany(ctx, filter)
	return err
}
