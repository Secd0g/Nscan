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

type PocRepo struct {
	templates *mongo.Collection
	custom    *mongo.Collection
}

func NewPocRepo(db *mongo.Database) *PocRepo {
	return &PocRepo{
		templates: db.Collection("nuclei_templates"),
		custom:    db.Collection("custom_pocs"),
	}
}

// ── Nuclei Templates ─────────────────────────────────────────────────────────

func (r *PocRepo) ListTemplates(ctx context.Context, filter bson.M, limit, skip int64) ([]models.NucleiTemplate, int64, error) {
	total, err := r.templates.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "severity_order", Value: 1}, {Key: "template_id", Value: 1}}).
		SetLimit(limit).SetSkip(skip).
		SetProjection(bson.M{"content": 0})
	cursor, err := r.templates.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var list []models.NucleiTemplate
	if err := cursor.All(ctx, &list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *PocRepo) GetTemplateContent(ctx context.Context, templateID string) (*models.NucleiTemplate, error) {
	var tpl models.NucleiTemplate
	err := r.templates.FindOne(ctx, bson.M{"template_id": templateID}).Decode(&tpl) // @check-ignore: system nuclei templates: global, no user scope
	if err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (r *PocRepo) TemplateStats(ctx context.Context) (*models.TemplateStats, error) {
	stats := &models.TemplateStats{}
	var err error
	stats.Total, err = r.templates.CountDocuments(ctx, bson.M{}) // @check-ignore: system nuclei templates: count all
	if err != nil {
		return nil, err
	}
	for _, sev := range []string{"critical", "high", "medium", "low", "info"} {
		count, _ := r.templates.CountDocuments(ctx, bson.M{"severity": sev}) // @check-ignore: system nuclei templates: count by severity
		switch sev {
		case "critical":
			stats.Critical = count
		case "high":
			stats.High = count
		case "medium":
			stats.Medium = count
		case "low":
			stats.Low = count
		case "info":
			stats.Info = count
		}
	}
	return stats, nil
}

func (r *PocRepo) TemplateCategories(ctx context.Context) ([]string, error) {
	vals, err := r.templates.Distinct(ctx, "category", bson.M{})
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

func (r *PocRepo) UpsertTemplate(ctx context.Context, tpl *models.NucleiTemplate) error {
	sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3, "info": 4}
	order, ok := sevOrder[tpl.Severity]
	if !ok {
		order = 5
	}
	_, err := r.templates.UpdateOne(ctx,
		bson.M{"template_id": tpl.TemplateID},
		bson.M{"$set": bson.M{
			"template_id":    tpl.TemplateID,
			"name":           tpl.Name,
			"severity":       tpl.Severity,
			"severity_order": order,
			"category":       tpl.Category,
			"author":         tpl.Author,
			"tags":           tpl.Tags,
			"description":    tpl.Description,
			"content":        tpl.Content,
			"created_at":     tpl.CreatedAt,
		}},
		options.Update().SetUpsert(true),
	)
	return err
}

func (r *PocRepo) ClearTemplates(ctx context.Context) error {
	_, err := r.templates.DeleteMany(ctx, bson.M{}) // @check-ignore: admin clear all: intentional
	return err
}

// ── Custom POCs ──────────────────────────────────────────────────────────────

func (r *PocRepo) ListCustom(ctx context.Context, extraFilter bson.M, limit, skip int64) ([]models.CustomPoc, int64, error) {
	filter := bson.M{}
	for k, v := range extraFilter {
		filter[k] = v
	}
	total, err := r.custom.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(limit).SetSkip(skip).
		SetProjection(bson.M{"content": 0})
	cursor, err := r.custom.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var list []models.CustomPoc
	if err := cursor.All(ctx, &list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *PocRepo) CreateCustom(ctx context.Context, poc *models.CustomPoc) error {
	poc.ID = primitive.NewObjectID()
	poc.CreatedAt = time.Now()
	_, err := r.custom.InsertOne(ctx, poc)
	return err
}

func (r *PocRepo) UpdateCustom(ctx context.Context, id primitive.ObjectID, fields bson.M) error {
	_, err := r.custom.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": fields})
	return err
}

func (r *PocRepo) DeleteCustom(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.custom.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *PocRepo) ClearCustom(ctx context.Context) error {
	_, err := r.custom.DeleteMany(ctx, bson.M{}) // @check-ignore: admin clear all: intentional
	return err
}
