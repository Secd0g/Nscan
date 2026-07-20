package repositories

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yourname/nscan/pkg/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// 集合名与 scheduler.resultCollection 保持一致
const (
	collSubdomain = "assets_subdomain"
	collPort      = "assets_port"
	collHTTP      = "assets_http"
	collVuln      = "assets_vuln"
	collDir       = "assets_dir"
	collSensitive = "assets_sensitive"
	collCrawler   = "assets_crawler"
	collChanges   = "asset_changes"
)

type AssetRepo struct {
	db *mongo.Database
}

func NewAssetRepo(db *mongo.Database) *AssetRepo {
	repo := &AssetRepo{db: db}
	repo.ensureIndexes(context.Background())
	return repo
}

func (r *AssetRepo) ensureIndexes(ctx context.Context) {
	type idxDef struct {
		coll string
		keys bson.D
		name string
	}
	indexes := []idxDef{
		{collSubdomain, bson.D{{Key: "project_id", Value: 1}, {Key: "domain", Value: 1}}, "uniq_project_domain"},
		{collPort, bson.D{{Key: "project_id", Value: 1}, {Key: "ip", Value: 1}, {Key: "port", Value: 1}, {Key: "protocol", Value: 1}}, "uniq_project_ip_port"},
		{collHTTP, bson.D{{Key: "project_id", Value: 1}, {Key: "url", Value: 1}}, "uniq_project_url"},
		{collVuln, bson.D{{Key: "project_id", Value: 1}, {Key: "target", Value: 1}, {Key: "template_id", Value: 1}}, "uniq_project_target_tpl"},
		{collSensitive, bson.D{{Key: "project_id", Value: 1}, {Key: "url", Value: 1}, {Key: "rule_id", Value: 1}}, "uniq_project_url_rule"},
		{collDir, bson.D{{Key: "project_id", Value: 1}, {Key: "url", Value: 1}}, "uniq_project_dir_url"},
	}
	for _, idx := range indexes {
		r.deduplicateExisting(ctx, idx.coll, idx.keys) // @check-ignore: internal dedup: targets specific _id set, no user boundary

		// 先尝试删除旧的同名非唯一索引（如果存在），再创建唯一索引
		_, _ = r.db.Collection(idx.coll).Indexes().DropOne(ctx, idx.name)

		model := mongo.IndexModel{
			Keys:    idx.keys,
			Options: options.Index().SetUnique(true).SetName(idx.name),
		}
		name, err := r.db.Collection(idx.coll).Indexes().CreateOne(ctx, model)
		if err != nil {
			fmt.Printf("[asset] WARN: create unique index %s on %s failed: %v\n", idx.name, idx.coll, err)
		} else {
			fmt.Printf("[asset] unique index %s created on %s\n", name, idx.coll)
		}
	}

	// asset_changes 集合非唯一索引：按 asset_id 查列表 + 按 project_id 按时间倒序
	_, _ = r.db.Collection(collChanges).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "asset_id", Value: 1}, {Key: "created_at", Value: -1}}, Options: options.Index().SetName("asset_ts")},
		{Keys: bson.D{{Key: "project_id", Value: 1}, {Key: "created_at", Value: -1}}, Options: options.Index().SetName("project_ts")},
	})

	// 列表排序索引：{project_id, created_at} 支持按项目分页浏览，{created_at} 支持无项目过滤时的全局排序。
	listColls := []string{collSubdomain, collPort, collHTTP, collVuln, collSensitive, collDir}
	for _, coll := range listColls {
		_, _ = r.db.Collection(coll).Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "project_id", Value: 1}, {Key: "created_at", Value: -1}}, Options: options.Index().SetName("list_project_ts")},
			{Keys: bson.D{{Key: "created_at", Value: -1}}, Options: options.Index().SetName("list_ts")},
		})
	}
}

// SaveChangeLog 记录一次资产变更
func (r *AssetRepo) SaveChangeLog(ctx context.Context, log *models.AssetChangeLog) error {
	if len(log.Changes) == 0 {
		return nil
	}
	log.CreatedAt = time.Now()
	_, err := r.db.Collection(collChanges).InsertOne(ctx, log)
	return err
}

// ListChanges 返回某资产的历史变更（按时间倒序）
func (r *AssetRepo) ListChanges(ctx context.Context, assetType string, assetID primitive.ObjectID, limit int64) ([]models.AssetChangeLog, error) {
	filter := bson.M{"asset_id": assetID}
	if assetType != "" {
		filter["asset_type"] = assetType
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	cursor, err := r.db.Collection(collChanges).Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var out []models.AssetChangeLog
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// deduplicateExisting 按去重键聚合，删除多余的文档（保留最新的一条）
func (r *AssetRepo) deduplicateExisting(ctx context.Context, collName string, keys bson.D) {
	coll := r.db.Collection(collName)

	groupID := bson.D{}
	for _, k := range keys {
		groupID = append(groupID, bson.E{Key: k.Key, Value: "$" + k.Key})
	}

	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: groupID},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "keep", Value: bson.D{{Key: "$max", Value: "$_id"}}},
			{Key: "all_ids", Value: bson.D{{Key: "$push", Value: "$_id"}}},
		}}},
		{{Key: "$match", Value: bson.D{{Key: "count", Value: bson.D{{Key: "$gt", Value: 1}}}}}},
	}

	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var result struct {
			Keep   primitive.ObjectID   `bson:"keep"`
			AllIDs []primitive.ObjectID `bson:"all_ids"`
		}
		if err := cursor.Decode(&result); err != nil {
			continue
		}
		deleteIDs := make([]primitive.ObjectID, 0, len(result.AllIDs)-1)
		for _, id := range result.AllIDs {
			if id != result.Keep {
				deleteIDs = append(deleteIDs, id)
			}
		}
		if len(deleteIDs) > 0 {
			coll.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": deleteIDs}}) // @check-ignore: internal dedup: targets specific _id set, no user boundary
		}
	}
}

type AssetFilter struct {
	UserID      string
	TaskID      string
	ProjectID   string
	Q           string // field="value" 语法查询串
	AssetType   string // http, port, subdomain, vuln, sensitive
	Severity    string // 漏洞危险等级筛选
	StatusCodes []int  // 命中的 HTTP 状态码集合（$in）
	SortBy      string // 排序字段，空则默认 created_at
	SortOrder   int    // 1=升序 -1=降序，0 默认降序
	Limit       int64
	Skip        int64
}

// ── FOFA 式搜索表达式解析器 ──────────────────────────────────────────────────
//
// 语法：field="value"  field=="value"  field!="value"
// 运算符：&&（AND）、||（OR）、括号
// =  → 正则模糊匹配（不区分大小写）
// == → 精确匹配
// != → 排除匹配

// searchKeyMap 每个资产类型可查询的字段 → MongoDB 字段映射
var searchKeyMap = map[string]map[string]string{
	"http": {
		"app": "tech", "tech": "tech", "body": "body", "header": "banner",
		"title": "title", "statuscode": "status_code", "status_code": "status_code",
		"icon": "favicon_mmh3", "ip": "ip", "domain": "domain", "port": "port",
		"service": "service", "banner": "banner", "url": "url", "server": "server",
	},
	"port": {
		"ip": "ip", "port": "port", "service": "service", "banner": "banner",
		"protocol": "protocol", "state": "state",
	},
	"subdomain": {
		"domain": "domain", "ip": "ips", "dns_type": "dns_type",
	},
	"vuln": {
		"name": "name", "target": "target", "template_id": "template_id",
		"severity": "severity", "tag": "tags",
	},
	"sensitive": {
		"rule_name": "rule_name", "url": "url", "matched": "matched", "severity": "severity",
	},
}

// numericFields 需要数字比较的字段
var numericFields = map[string]bool{
	"port": true, "status_code": true, "statuscode": true, "content_len": true,
}

// parseQ 将搜索表达式解析为 MongoDB 查询条件
// 支持 && || 运算符、括号、= == != 比较
func parseQ(q string, base bson.M) bson.M {
	q = strings.TrimSpace(q)
	if q == "" {
		return base
	}

	tokens := tokenize(q)
	postfix := infixToPostfix(tokens)
	result := evalPostfix(postfix, "http") // 默认用 http 的字段映射

	if result != nil {
		for k, v := range result {
			base[k] = v
		}
	}
	return base
}

// parseQWithType 指定资产类型的搜索解析
func parseQWithType(q string, assetType string, base bson.M) bson.M {
	q = strings.TrimSpace(q)
	if q == "" {
		return base
	}
	tokens := tokenize(q)
	postfix := infixToPostfix(tokens)
	result := evalPostfix(postfix, assetType)
	if result != nil {
		for k, v := range result {
			base[k] = v
		}
	}
	return base
}

func tokenize(expr string) []string {
	var tokens []string
	i := 0
	for i < len(expr) {
		ch := expr[i]
		switch {
		case ch == ' ' || ch == '\t':
			i++
		case ch == '(':
			tokens = append(tokens, "(")
			i++
		case ch == ')':
			tokens = append(tokens, ")")
			i++
		case ch == '&' && i+1 < len(expr) && expr[i+1] == '&':
			tokens = append(tokens, "&&")
			i += 2
		case ch == '|' && i+1 < len(expr) && expr[i+1] == '|':
			tokens = append(tokens, "||")
			i += 2
		default:
			// 读一个 field op "value" 表达式
			j := i
			for j < len(expr) && expr[j] != '(' && expr[j] != ')' &&
				!(expr[j] == '&' && j+1 < len(expr) && expr[j+1] == '&') &&
				!(expr[j] == '|' && j+1 < len(expr) && expr[j+1] == '|') {
				if expr[j] == '"' {
					j++
					for j < len(expr) && expr[j] != '"' {
						j++
					}
					if j < len(expr) {
						j++
					}
				} else {
					j++
				}
			}
			token := strings.TrimSpace(expr[i:j])
			if token != "" {
				tokens = append(tokens, token)
			}
			i = j
		}
	}
	return tokens
}

func infixToPostfix(tokens []string) []string {
	var output, ops []string
	for _, t := range tokens {
		switch t {
		case "(":
			ops = append(ops, t)
		case ")":
			for len(ops) > 0 && ops[len(ops)-1] != "(" {
				output = append(output, ops[len(ops)-1])
				ops = ops[:len(ops)-1]
			}
			if len(ops) > 0 {
				ops = ops[:len(ops)-1] // pop "("
			}
		case "&&":
			for len(ops) > 0 && ops[len(ops)-1] == "&&" {
				output = append(output, ops[len(ops)-1])
				ops = ops[:len(ops)-1]
			}
			ops = append(ops, t)
		case "||":
			for len(ops) > 0 && (ops[len(ops)-1] == "&&" || ops[len(ops)-1] == "||") {
				output = append(output, ops[len(ops)-1])
				ops = ops[:len(ops)-1]
			}
			ops = append(ops, t)
		default:
			output = append(output, t)
		}
	}
	for len(ops) > 0 {
		output = append(output, ops[len(ops)-1])
		ops = ops[:len(ops)-1]
	}
	return output
}

var exprRe = regexp.MustCompile(`^(\w+)\s*(!=|==|=)\s*"([^"]*)"$`)

func evalPostfix(postfix []string, assetType string) bson.M {
	if len(postfix) == 0 {
		return nil
	}

	keyMap := searchKeyMap[assetType]
	if keyMap == nil {
		keyMap = searchKeyMap["http"]
	}

	var stack []bson.M
	for _, token := range postfix {
		switch token {
		case "&&":
			if len(stack) < 2 {
				continue
			}
			right := stack[len(stack)-1]
			left := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, bson.M{"$and": bson.A{left, right}})
		case "||":
			if len(stack) < 2 {
				continue
			}
			right := stack[len(stack)-1]
			left := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, bson.M{"$or": bson.A{left, right}})
		default:
			m := exprRe.FindStringSubmatch(token)
			if m == nil {
				continue
			}
			field, op, value := strings.ToLower(m[1]), m[2], m[3]
			mongoKey, ok := keyMap[field]
			if !ok {
				continue
			}
			isNumeric := numericFields[field]

			var cond bson.M
			switch op {
			case "!=":
				if isNumeric {
					if n, err := strconv.Atoi(value); err == nil {
						cond = bson.M{mongoKey: bson.M{"$ne": n}}
					}
				} else if value == "" {
					cond = bson.M{"$and": bson.A{
						bson.M{mongoKey: bson.M{"$ne": ""}},
						bson.M{mongoKey: bson.M{"$ne": nil}},
					}}
				} else {
					cond = bson.M{mongoKey: bson.M{"$not": primitive.Regex{Pattern: regexp.QuoteMeta(value), Options: "i"}}}
				}
			case "==":
				if isNumeric {
					if n, err := strconv.Atoi(value); err == nil {
						cond = bson.M{mongoKey: n}
					}
				} else {
					cond = bson.M{mongoKey: value}
				}
			case "=":
				if isNumeric {
					if n, err := strconv.Atoi(value); err == nil {
						cond = bson.M{mongoKey: n}
					}
				} else {
					cond = bson.M{mongoKey: primitive.Regex{Pattern: regexp.QuoteMeta(value), Options: "i"}}
				}
			}
			if cond != nil {
				stack = append(stack, cond)
			}
		}
	}
	if len(stack) == 0 {
		return nil
	}
	return stack[0]
}

func assetFilter(f AssetFilter) bson.M {
	filter := bson.M{}
	if f.UserID != "" {
		if oid, err := primitive.ObjectIDFromHex(f.UserID); err == nil {
			filter["user_id"] = oid
		}
	}
	if f.TaskID != "" {
		if oid, err := primitive.ObjectIDFromHex(f.TaskID); err == nil {
			filter["$or"] = []bson.M{{"task_id": oid}, {"task_id": f.TaskID}, {"task_ids": oid}, {"task_ids": f.TaskID}}
		} else {
			filter["task_id"] = f.TaskID
		}
	}
	if f.ProjectID != "" {
		if oid, err := primitive.ObjectIDFromHex(f.ProjectID); err == nil {
			filter["project_id"] = oid
		}
	}
	if f.Q != "" {
		at := f.AssetType
		if at == "" {
			at = "http"
		}
		filter = parseQWithType(f.Q, at, filter)
	}
	if f.Severity != "" {
		filter["severity"] = f.Severity
	}
	if len(f.StatusCodes) > 0 {
		filter["status_code"] = bson.M{"$in": f.StatusCodes}
	}
	return filter
}

func assetFindOpts(f AssetFilter) *options.FindOptions {
	sortField := "created_at"
	sortOrder := -1
	if f.SortBy != "" {
		sortField = f.SortBy
		if f.SortOrder != 0 {
			sortOrder = f.SortOrder
		}
	}
	return options.Find().
		SetLimit(f.Limit).
		SetSkip(f.Skip).
		SetSort(bson.D{{Key: sortField, Value: sortOrder}})
}

func (r *AssetRepo) ListSubdomains(ctx context.Context, f AssetFilter) ([]models.SubdomainAsset, int64, error) {
	coll := r.db.Collection(collSubdomain)
	filter := assetFilter(f)
	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cursor, err := coll.Find(ctx, filter, assetFindOpts(f))
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var list []models.SubdomainAsset
	return list, total, cursor.All(ctx, &list)
}

func (r *AssetRepo) ListPorts(ctx context.Context, f AssetFilter) ([]models.PortAsset, int64, error) {
	coll := r.db.Collection(collPort)
	filter := assetFilter(f)
	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: -1}}}},
		{{Key: "$skip", Value: f.Skip}},
		{{Key: "$limit", Value: f.Limit}},
		{{Key: "$lookup", Value: bson.M{
			"from": collHTTP,
			"let":  bson.M{"p_ip": "$ip", "p_port": "$port", "p_project": "$project_id"},
			"pipeline": mongo.Pipeline{
				{{Key: "$match", Value: bson.M{"$expr": bson.M{"$and": bson.A{
					bson.M{"$eq": bson.A{"$ip", "$$p_ip"}},
					bson.M{"$eq": bson.A{"$port", "$$p_port"}},
					bson.M{"$eq": bson.A{"$project_id", "$$p_project"}},
				}}}}},
				{{Key: "$project", Value: bson.M{"tech": 1, "domain": 1, "_id": 0}}},
			},
			"as": "_http_matches",
		}}},
		{{Key: "$addFields", Value: bson.M{
			"products": bson.M{"$reduce": bson.M{
				"input":        "$_http_matches.tech",
				"initialValue": bson.A{},
				"in":           bson.M{"$setUnion": bson.A{"$$value", "$$this"}},
			}},
			"domains": bson.M{"$setUnion": bson.A{"$_http_matches.domain", bson.A{}}},
		}}},
		{{Key: "$project", Value: bson.M{"_http_matches": 0}}},
	}

	type portWithProducts struct {
		models.PortAsset `bson:",inline"`
		Products         []string `bson:"products"`
		Domains          []string `bson:"domains"`
	}

	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var rows []portWithProducts
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, 0, err
	}
	list := make([]models.PortAsset, len(rows))
	for i, row := range rows {
		list[i] = row.PortAsset
		list[i].Products = row.Products
		list[i].Domains = row.Domains
	}
	return list, total, nil
}

func (r *AssetRepo) ListHTTP(ctx context.Context, f AssetFilter) ([]models.HTTPAsset, int64, error) {
	coll := r.db.Collection(collHTTP)
	filter := assetFilter(f)
	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cursor, err := coll.Find(ctx, filter, assetFindOpts(f))
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var list []models.HTTPAsset
	if err := cursor.All(ctx, &list); err != nil {
		return nil, 0, err
	}
	for i := range list {
		seen := make(map[string]struct{}, len(list[i].Tech))
		tech := list[i].Tech[:0]
		for _, value := range list[i].Tech {
			key := strings.ToLower(strings.TrimSpace(value))
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			tech = append(tech, value)
		}
		list[i].Tech = tech
	}
	return list, total, nil
}

func (r *AssetRepo) ListDirs(ctx context.Context, f AssetFilter) ([]models.DirAsset, int64, error) {
	coll := r.db.Collection(collDir)
	filter := assetFilter(f)
	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cursor, err := coll.Find(ctx, filter, assetFindOpts(f))
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var list []models.DirAsset
	return list, total, cursor.All(ctx, &list)
}

func (r *AssetRepo) ListVulns(ctx context.Context, f AssetFilter) ([]models.VulnAsset, int64, error) {
	coll := r.db.Collection(collVuln)
	filter := assetFilter(f)
	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	opts := assetFindOpts(f)
	opts.SetProjection(bson.M{"request": 0, "response": 0}) // 列表不返回大字段
	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var list []models.VulnAsset
	return list, total, cursor.All(ctx, &list)
}

func (r *AssetRepo) UpdateVulnStatus(ctx context.Context, id primitive.ObjectID, status int) error {
	_, err := r.db.Collection(collVuln).UpdateByID(ctx, id, bson.M{
		"$set": bson.M{"status": status, "updated_at": time.Now()},
	})
	return err
}

func (r *AssetRepo) SaveSubdomain(ctx context.Context, a *models.SubdomainAsset) error {
	now := time.Now()
	filter := bson.M{"project_id": a.ProjectID, "domain": a.Domain}
	// 拿旧值用于 diff
	var old models.SubdomainAsset
	hadOld := r.db.Collection(collSubdomain).FindOne(ctx, filter).Decode(&old) == nil
	setOnInsert := bson.M{"project_id": a.ProjectID, "domain": a.Domain, "created_at": now}
	update := bson.M{
		"$set":         bson.M{"task_id": a.TaskID, "ips": a.IPs, "updated_at": now},
		"$setOnInsert": setOnInsert,
	}
	// 多工具来源累积：用 $addToSet 追加，不覆盖
	if len(a.Sources) > 0 {
		update["$addToSet"] = bson.M{"sources": bson.M{"$each": a.Sources}}
	}
	opts := options.Update().SetUpsert(true)
	res, err := r.db.Collection(collSubdomain).UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return err
	}
	if hadOld {
		changes := diffFields(map[string][2]string{
			"ips": {joinStrings(old.IPs), joinStrings(a.IPs)},
		})
		if len(changes) > 0 {
			_ = r.SaveChangeLog(ctx, &models.AssetChangeLog{
				AssetID: old.ID, AssetType: "subdomain",
				ProjectID: a.ProjectID, TaskID: a.TaskID, Changes: changes,
			})
		}
	} else if res.UpsertedID != nil {
		if oid, ok := res.UpsertedID.(primitive.ObjectID); ok {
			_ = r.SaveChangeLog(ctx, &models.AssetChangeLog{
				AssetID: oid, AssetType: "subdomain",
				ProjectID: a.ProjectID, TaskID: a.TaskID,
				Changes: []models.FieldChange{{Field: "status", Old: "", New: "new_discovered"}},
			})
		}
	}
	return nil
}

func (r *AssetRepo) SavePort(ctx context.Context, a *models.PortAsset) error {
	now := time.Now()
	proto := a.Protocol
	if proto == "" {
		proto = "tcp"
	}
	filter := bson.M{"project_id": a.ProjectID, "ip": a.IP, "port": a.Port, "protocol": proto}
	var old models.PortAsset
	hadOld := r.db.Collection(collPort).FindOne(ctx, filter).Decode(&old) == nil
	setOnInsert := bson.M{"project_id": a.ProjectID, "ip": a.IP, "port": a.Port, "protocol": proto, "created_at": now}
	update := bson.M{
		"$set":         bson.M{"task_id": a.TaskID, "state": a.State, "service": a.Service, "updated_at": now},
		"$setOnInsert": setOnInsert,
	}
	// 多工具来源累积：用 $addToSet 追加，不覆盖
	if len(a.Sources) > 0 {
		update["$addToSet"] = bson.M{"sources": bson.M{"$each": a.Sources}}
	}
	opts := options.Update().SetUpsert(true)
	res, err := r.db.Collection(collPort).UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return err
	}
	if hadOld {
		changes := diffFields(map[string][2]string{
			"state":   {old.State, a.State},
			"service": {old.Service, a.Service},
		})
		if len(changes) > 0 {
			_ = r.SaveChangeLog(ctx, &models.AssetChangeLog{
				AssetID: old.ID, AssetType: "port",
				ProjectID: a.ProjectID, TaskID: a.TaskID, Changes: changes,
			})
		}
	} else if res.UpsertedID != nil {
		if oid, ok := res.UpsertedID.(primitive.ObjectID); ok {
			_ = r.SaveChangeLog(ctx, &models.AssetChangeLog{
				AssetID: oid, AssetType: "port",
				ProjectID: a.ProjectID, TaskID: a.TaskID,
				Changes: []models.FieldChange{{Field: "status", Old: "", New: "new_discovered"}},
			})
		}
	}
	return nil
}

func (r *AssetRepo) ListSensitive(ctx context.Context, f AssetFilter) ([]models.SensitiveAsset, int64, error) {
	coll := r.db.Collection(collSensitive)
	filter := assetFilter(f)
	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cursor, err := coll.Find(ctx, filter, assetFindOpts(f))
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var list []models.SensitiveAsset
	return list, total, cursor.All(ctx, &list)
}

func (r *AssetRepo) SensitiveAggByRule(ctx context.Context, f AssetFilter) ([]map[string]interface{}, error) {
	coll := r.db.Collection(collSensitive)
	match := assetFilter(f)
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{{Key: "rule_name", Value: "$rule_name"}, {Key: "severity", Value: "$severity"}}},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
	}
	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var raw []struct {
		ID struct {
			RuleName string `bson:"rule_name"`
			Severity string `bson:"severity"`
		} `bson:"_id"`
		Count int `bson:"count"`
	}
	if err := cursor.All(ctx, &raw); err != nil {
		return nil, err
	}
	result := make([]map[string]interface{}, len(raw))
	for i, r := range raw {
		result[i] = map[string]interface{}{
			"rule_name": r.ID.RuleName,
			"severity":  r.ID.Severity,
			"count":     r.Count,
		}
	}
	return result, nil
}

func (r *AssetRepo) SaveSensitive(ctx context.Context, a *models.SensitiveAsset) error {
	now := time.Now()
	filter := bson.M{"project_id": a.ProjectID, "url": a.URL, "rule_id": a.RuleID}
	update := bson.M{
		"$set": bson.M{
			"task_id": a.TaskID, "rule_name": a.RuleName, "severity": a.Severity,
			"matched": a.Matched, "context": a.Context, "updated_at": now,
		},
		"$setOnInsert": bson.M{
			"project_id": a.ProjectID, "url": a.URL, "rule_id": a.RuleID, "created_at": now,
		},
	}
	opts := options.Update().SetUpsert(true)
	_, err := r.db.Collection(collSensitive).UpdateOne(ctx, filter, update, opts)
	return err
}

func (r *AssetRepo) ListCrawler(ctx context.Context, f AssetFilter) ([]models.CrawlerAsset, int64, error) {
	coll := r.db.Collection(collCrawler)
	filter := assetFilter(f)
	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cursor, err := coll.Find(ctx, filter, assetFindOpts(f))
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var list []models.CrawlerAsset
	return list, total, cursor.All(ctx, &list)
}

func (r *AssetRepo) SaveHTTP(ctx context.Context, a *models.HTTPAsset) error {
	now := time.Now()
	filter := bson.M{"project_id": a.ProjectID, "url": a.URL}
	var old models.HTTPAsset
	hadOld := r.db.Collection(collHTTP).FindOne(ctx, filter).Decode(&old) == nil
	setOnInsert := bson.M{"project_id": a.ProjectID, "url": a.URL, "created_at": now}
	if a.Source != "" {
		setOnInsert["source"] = a.Source // 首次发现者胜出
	}
	update := bson.M{
		"$set": bson.M{
			"task_id": a.TaskID, "domain": a.Domain, "ip": a.IP, "port": a.Port,
			"status_code": a.StatusCode, "title": a.Title, "tech": a.Tech,
			"banner": a.Banner, "content_len": a.ContentLen, "screenshot": a.ScreenshotPath,
			"updated_at": now,
		},
		"$setOnInsert": setOnInsert,
	}
	opts := options.Update().SetUpsert(true)
	res, err := r.db.Collection(collHTTP).UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return err
	}
	if hadOld {
		changes := diffFields(map[string][2]string{
			"status_code": {intToStr(old.StatusCode), intToStr(a.StatusCode)},
			"title":       {old.Title, a.Title},
			"tech":        {joinStrings(old.Tech), joinStrings(a.Tech)},
			"banner":      {old.Banner, a.Banner},
			"ip":          {old.IP, a.IP},
			"port":        {intToStr(old.Port), intToStr(a.Port)},
		})
		if len(changes) > 0 {
			_ = r.SaveChangeLog(ctx, &models.AssetChangeLog{
				AssetID: old.ID, AssetType: "http",
				ProjectID: a.ProjectID, TaskID: a.TaskID, Changes: changes,
			})
		}
	} else if res.UpsertedID != nil {
		if oid, ok := res.UpsertedID.(primitive.ObjectID); ok {
			_ = r.SaveChangeLog(ctx, &models.AssetChangeLog{
				AssetID: oid, AssetType: "http",
				ProjectID: a.ProjectID, TaskID: a.TaskID,
				Changes: []models.FieldChange{{Field: "status", Old: "", New: "new_discovered"}},
			})
		}
	}
	return nil
}

// ── diff 助手 ─────────────────────────────────────────────────────────────

// diffFields 对比 old/new 字段对，返回有差异的 FieldChange 列表
// 输入 map: field -> [old, new]；old==new 或 (old=="" && new=="") 都跳过
func diffFields(pairs map[string][2]string) []models.FieldChange {
	if len(pairs) == 0 {
		return nil
	}
	out := make([]models.FieldChange, 0, len(pairs))
	// 稳定顺序遍历
	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	sortStrings(keys)
	for _, k := range keys {
		v := pairs[k]
		if v[0] == v[1] {
			continue
		}
		out = append(out, models.FieldChange{Field: k, Old: v[0], New: v[1]})
	}
	return out
}

func joinStrings(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	// 排序后拼接，保证 slice 顺序变化不算 diff
	sorted := make([]string, len(ss))
	copy(sorted, ss)
	sortStrings(sorted)
	out := ""
	for i, s := range sorted {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}

func intToStr(n int) string {
	return strconv.Itoa(n)
}

func sortStrings(ss []string) {
	// 手写小排序避免引入 sort 包（其实已经有），用 sort.Strings 也可
	// 为简洁改用 sort.Strings
	sort.Strings(ss)
}

// ── IP 聚合视图 ──────────────────────────────────────────────────────────────

// ListIPAggregated 实时按 IP 聚合端口+HTTP 数据，返回拍平的行列表。
func (r *AssetRepo) ListIPAggregated(ctx context.Context, f AssetFilter) ([]models.IPAssetFlat, int64, error) {
	filter := assetFilter(f)

	// 1) 按 IP 分组取端口列表，分页在 IP 维度
	groupPipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$ip"},
			{Key: "ports", Value: bson.M{"$addToSet": bson.M{"port": "$port", "protocol": "$protocol", "service": "$service", "banner": "$banner"}}},
			{Key: "time", Value: bson.M{"$max": "$created_at"}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "time", Value: -1}}}},
	}

	// 计数（IP 数）
	countPipeline := append(mongo.Pipeline{}, groupPipeline...)
	countPipeline = append(countPipeline, bson.D{{Key: "$count", Value: "total"}})
	countCur, err := r.db.Collection(collPort).Aggregate(ctx, countPipeline)
	if err != nil {
		return nil, 0, err
	}
	var countResult []struct{ Total int64 }
	_ = countCur.All(ctx, &countResult)
	countCur.Close(ctx)
	var total int64
	if len(countResult) > 0 {
		total = countResult[0].Total
	}

	// 分页
	dataPipeline := append(mongo.Pipeline{}, groupPipeline...)
	dataPipeline = append(dataPipeline,
		bson.D{{Key: "$skip", Value: f.Skip}},
		bson.D{{Key: "$limit", Value: f.Limit}},
	)

	cursor, err := r.db.Collection(collPort).Aggregate(ctx, dataPipeline)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	type portEntry struct {
		Port     int    `bson:"port"`
		Protocol string `bson:"protocol"`
		Service  string `bson:"service"`
		Banner   string `bson:"banner"`
	}
	type ipGroup struct {
		IP    string      `bson:"_id"`
		Ports []portEntry `bson:"ports"`
		Time  time.Time   `bson:"time"`
	}

	var groups []ipGroup
	if err := cursor.All(ctx, &groups); err != nil {
		return nil, 0, err
	}

	// 2) 批量查 HTTP 资产的 tech, domain, webServer（按 ip+port 关联）
	allIPs := make([]string, len(groups))
	for i, g := range groups {
		allIPs[i] = g.IP
	}
	httpMap := r.batchHTTPLookup(ctx, allIPs, f)

	// 3) 拍平
	var rows []models.IPAssetFlat
	for _, g := range groups {
		sort.Slice(g.Ports, func(i, j int) bool { return g.Ports[i].Port < g.Ports[j].Port })
		timeStr := g.Time.Format("2006-01-02 15:04:05")

		// 计算每个端口关联的 HTTP 服务数，算出总行数
		type portServers struct {
			port    portEntry
			servers []models.IPServer
		}
		var psSlice []portServers
		totalRows := 0
		for _, p := range g.Ports {
			key := fmt.Sprintf("%s:%d", g.IP, p.Port)
			servers := httpMap[key]
			if len(servers) == 0 {
				servers = []models.IPServer{{Service: p.Service}}
			}
			psSlice = append(psSlice, portServers{port: p, servers: servers})
			totalRows += len(servers)
		}
		if totalRows == 0 {
			totalRows = 1
		}

		ipAssigned := false
		for _, ps := range psSlice {
			portAssigned := false
			for _, srv := range ps.servers {
				row := models.IPAssetFlat{
					IP:        g.IP,
					Port:      ps.port.Port,
					Domain:    srv.Domain,
					Service:   firstNonEmpty(srv.Service, ps.port.Service),
					WebServer: srv.WebServer,
					Products:  srv.Products,
					Time:      timeStr,
				}
				if !ipAssigned {
					row.IPRowSpan = totalRows
					ipAssigned = true
				}
				if !portAssigned {
					row.PortRowSpan = len(ps.servers)
					portAssigned = true
				}
				rows = append(rows, row)
			}
		}
	}

	return rows, total, nil
}

// batchHTTPLookup 批量查询 HTTP 资产的 tech/domain/webServer，按 "ip:port" 分组
func (r *AssetRepo) batchHTTPLookup(ctx context.Context, ips []string, f AssetFilter) map[string][]models.IPServer {
	if len(ips) == 0 {
		return nil
	}
	httpFilter := bson.M{"ip": bson.M{"$in": ips}}
	if f.ProjectID != "" {
		if oid, err := primitive.ObjectIDFromHex(f.ProjectID); err == nil {
			httpFilter["project_id"] = oid
		}
	}
	cursor, err := r.db.Collection(collHTTP).Find(ctx, httpFilter,
		options.Find().SetProjection(bson.M{"ip": 1, "port": 1, "domain": 1, "tech": 1, "banner": 1}))
	if err != nil {
		return nil
	}
	defer cursor.Close(ctx)

	result := make(map[string][]models.IPServer)
	for cursor.Next(ctx) {
		var h struct {
			IP     string   `bson:"ip"`
			Port   int      `bson:"port"`
			Domain string   `bson:"domain"`
			Tech   []string `bson:"tech"`
			Banner string   `bson:"banner"`
		}
		if cursor.Decode(&h) != nil {
			continue
		}
		key := fmt.Sprintf("%s:%d", h.IP, h.Port)
		webServer := ""
		if h.Banner != "" {
			for _, line := range strings.Split(h.Banner, "\r\n") {
				if strings.HasPrefix(strings.ToLower(line), "server:") {
					webServer = strings.TrimSpace(line[7:])
					break
				}
			}
		}
		result[key] = append(result[key], models.IPServer{
			Domain:    h.Domain,
			Products:  h.Tech,
			WebServer: webServer,
		})
	}
	return result
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// StatItem 聚合计数结果
type StatItem struct {
	Value string `bson:"_id" json:"value"`
	Count int    `bson:"count" json:"count"`
}

// AssetStats 资产统计（基于 assets_http 集合，保证与主表计数一致）
type AssetStats struct {
	Ports []StatItem `json:"ports"`
	Techs []StatItem `json:"techs"`
}

func (r *AssetRepo) Stats(ctx context.Context, f AssetFilter) (*AssetStats, error) {
	matchStage := bson.D{{Key: "$match", Value: assetFilter(f)}}

	var stats AssetStats

	// 端口统计：从 assets_http 聚合，与主表计数一致
	portPipeline := mongo.Pipeline{
		matchStage,
		{{Key: "$match", Value: bson.D{{Key: "port", Value: bson.D{{Key: "$gt", Value: 0}}}}}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$port"}, {Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
		{{Key: "$limit", Value: 20}},
	}
	if intCur, err := r.db.Collection(collHTTP).Aggregate(ctx, portPipeline); err == nil {
		type intItem struct {
			Value int `bson:"_id"`
			Count int `bson:"count"`
		}
		var items []intItem
		_ = intCur.All(ctx, &items)
		stats.Ports = make([]StatItem, len(items))
		for i, it := range items {
			stats.Ports[i] = StatItem{Value: strconv.Itoa(it.Value), Count: it.Count}
		}
	}

	// tech 在 HTTP 资产里，展开数组
	techPipeline := mongo.Pipeline{
		matchStage,
		{{Key: "$unwind", Value: "$tech"}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$tech"}, {Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
		{{Key: "$limit", Value: 20}},
	}
	if cur, err := r.db.Collection(collHTTP).Aggregate(ctx, techPipeline); err == nil {
		_ = cur.All(ctx, &stats.Techs)
	}

	return &stats, nil
}

// BatchDelete 按 ID 列表批量删除指定类型资产
func (r *AssetRepo) BatchDelete(ctx context.Context, userID primitive.ObjectID, assetType string, ids []string) error {
	collMap := map[string]string{
		"http": collHTTP, "port": collPort, "subdomain": collSubdomain, "vuln": collVuln, "dir": collDir, "crawler": collCrawler, "sensitive": collSensitive,
	}
	coll, ok := collMap[assetType]
	if !ok {
		return fmt.Errorf("unknown asset type: %s", assetType)
	}
	oids := make([]primitive.ObjectID, 0, len(ids))
	for _, id := range ids {
		if oid, err := primitive.ObjectIDFromHex(id); err == nil {
			oids = append(oids, oid)
		}
	}
	filter := bson.M{"_id": bson.M{"$in": oids}}
	if !userID.IsZero() {
		filter["user_id"] = userID
	}
	_, err := r.db.Collection(coll).DeleteMany(ctx, filter)
	return err
}

// DeleteByTaskID 删除指定任务的所有资产。
// 资产写入路径不统一：gRPC 结果（scheduler.OnResult）存 task_id 为 ObjectID，
// 而部分 Save* 存为字符串，因此两种形式都要匹配。
func (r *AssetRepo) DeleteByTaskID(ctx context.Context, taskID string) error {
	conds := []interface{}{bson.M{"task_id": taskID}}
	if oid, err := primitive.ObjectIDFromHex(taskID); err == nil {
		conds = append(conds, bson.M{"task_id": oid})
	}
	filter := bson.M{"$or": conds}
	for _, coll := range []string{collSubdomain, collPort, collHTTP, collVuln, collDir, collCrawler, collSensitive} {
		if _, err := r.db.Collection(coll).DeleteMany(ctx, filter); err != nil {
			return err
		}
	}
	return nil
}

// DeleteOrphansByProject 清理指定项目里"无主"资产：task_id 为空 / nil / 零值 / 已被删除的任务 ID。
// 触发场景：批量删除任务后调用，防止历史任务已删但资产因 task_id 不一致而残留；
// 手动导入（online_search）路径不写 task_id 也会被这里兜底清掉。
// project_id 支持 ObjectID 与字符串两种存储形式（历史遗留）。
func (r *AssetRepo) DeleteOrphansByProject(ctx context.Context, projectID string) error {
	if projectID == "" {
		return nil
	}
	// 收集该项目现存的 task_id 集合（用于识别"指向已删任务"的资产）
	projectClauses := []bson.M{{"project_id": projectID}}
	if pOID, err := primitive.ObjectIDFromHex(projectID); err == nil {
		projectClauses = append(projectClauses, bson.M{"project_id": pOID})
	}
	projectFilter := bson.M{"$or": projectClauses}

	cur, err := r.db.Collection("tasks").Find(ctx, projectFilter, options.Find().SetProjection(bson.M{"_id": 1})) // @check-ignore: cross-collection join: reading task IDs for orphan cleanup
	if err != nil {
		return err
	}
	liveOIDs := []primitive.ObjectID{}
	liveStrs := []string{}
	for cur.Next(ctx) {
		var d struct {
			ID primitive.ObjectID `bson:"_id"`
		}
		if err := cur.Decode(&d); err == nil {
			liveOIDs = append(liveOIDs, d.ID)
			liveStrs = append(liveStrs, d.ID.Hex())
		}
	}
	cur.Close(ctx)

	// task_id 满足下列任一 → 视为孤儿
	//   缺失 / nil / "" / 零值 OID / 不在 live 集合中
	orphanTaskConds := []bson.M{
		{"task_id": bson.M{"$exists": false}},
		{"task_id": nil},
		{"task_id": ""},
		{"task_id": primitive.NilObjectID},
	}
	// "既不是当前存活的字符串 ID，也不是当前存活的 ObjectID"
	notLive := bson.M{
		"$and": []bson.M{
			{"task_id": bson.M{"$nin": liveStrs}},
			{"task_id": bson.M{"$nin": liveOIDs}},
		},
	}
	orphanTaskConds = append(orphanTaskConds, notLive)

	filter := bson.M{
		"$and": []bson.M{
			projectFilter,
			{"$or": orphanTaskConds},
		},
	}
	for _, coll := range []string{collSubdomain, collPort, collHTTP, collVuln, collDir, collCrawler, collSensitive} {
		if _, err := r.db.Collection(coll).DeleteMany(ctx, filter); err != nil {
			return err
		}
	}
	return nil
}

func (r *AssetRepo) SaveVuln(ctx context.Context, a *models.VulnAsset) error {
	now := time.Now()
	filter := bson.M{"project_id": a.ProjectID, "target": a.Target, "template_id": a.TemplateID}
	set := bson.M{
		"task_id": a.TaskID, "name": a.Name, "severity": a.Severity,
		"matched_at": a.MatchedAt, "updated_at": now,
	}
	if a.Request != "" {
		set["request"] = a.Request
	}
	if a.Response != "" {
		set["response"] = a.Response
	}
	update := bson.M{
		"$set": set,
		"$setOnInsert": bson.M{
			"project_id": a.ProjectID, "target": a.Target, "template_id": a.TemplateID,
			"created_at": now,
		},
	}
	opts := options.Update().SetUpsert(true)
	_, err := r.db.Collection(collVuln).UpdateOne(ctx, filter, update, opts)
	return err
}

func (r *AssetRepo) GetVuln(ctx context.Context, id primitive.ObjectID) (*models.VulnAsset, error) {
	var v models.VulnAsset
	err := r.db.Collection(collVuln).FindOne(ctx, bson.M{"_id": id}).Decode(&v)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// DiffOfflineAssets 找出项目中在本次任务中未更新（未扫到）的资产，并标记为下线
// DiffOfflineAssets marks assets not updated in the current task as offline.
// stages is the list of stages that ran in this task; only the relevant
// collections are diffed so that a port-only scan doesn't wipe HTTP titles.
func (r *AssetRepo) DiffOfflineAssets(ctx context.Context, projectID string, currentTaskID string, stages []string) error {
	pID, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return err
	}

	stageSet := make(map[string]bool, len(stages))
	for _, s := range stages {
		stageSet[s] = true
	}
	hasSubdomain := stageSet["subdomain"] || stageSet["bbot"] || stageSet["amass"] || len(stages) == 0
	hasPort := stageSet["port"] || stageSet["portscan"] || len(stages) == 0
	hasHTTP := stageSet["http"] || stageSet["httpx"] || len(stages) == 0

	var colls []string
	if hasSubdomain {
		colls = append(colls, collSubdomain)
	}
	if hasPort {
		colls = append(colls, collPort)
	}
	if hasHTTP {
		colls = append(colls, collHTTP)
	}
	// task_id 存储形式不统一：gRPC 路径（OnResult）存 ObjectID，Save* 路径存字符串。
	// 用两个独立类型的 $nin 条件取 $and，避免 []interface{} 混合类型序列化歧义。
	taskIDStr := currentTaskID
	taskIDOID, taskIDOIDErr := primitive.ObjectIDFromHex(currentTaskID)

	for _, coll := range colls {
		// 构造"task_id 不属于当前任务"的筛选条件
		// 需要同时排除字符串形式和 ObjectID 形式
		var taskFilter bson.M
		if taskIDOIDErr == nil {
			taskFilter = bson.M{
				"$and": bson.A{
					bson.M{"task_id": bson.M{"$ne": taskIDStr}},
					bson.M{"task_id": bson.M{"$ne": taskIDOID}},
				},
			}
		} else {
			taskFilter = bson.M{"task_id": bson.M{"$ne": taskIDStr}}
		}
		filter := bson.M{
			"$and": bson.A{
				bson.M{"project_id": pID},
				taskFilter,
			},
		}

		cursor, err := r.db.Collection(coll).Find(ctx, filter)
		if err != nil {
			continue
		}

		var offlineIDs []primitive.ObjectID
		for cursor.Next(ctx) {
			var asset struct {
				ID         primitive.ObjectID `bson:"_id"`
				State      string             `bson:"state"`       // port
				StatusCode int                `bson:"status_code"` // http
			}
			if err := cursor.Decode(&asset); err != nil {
				continue
			}

			assetType := "subdomain"
			if coll == collPort {
				assetType = "port"
				if asset.State == "closed" {
					continue
				}
			} else if coll == collHTTP {
				assetType = "http"
				if asset.StatusCode == 0 {
					continue
				}
			}

			_ = r.SaveChangeLog(ctx, &models.AssetChangeLog{
				AssetID: asset.ID, AssetType: assetType,
				ProjectID: projectID, TaskID: currentTaskID,
				Changes: []models.FieldChange{{Field: "status", Old: "online", New: "offline"}},
			})
			offlineIDs = append(offlineIDs, asset.ID)
		}
		cursor.Close(ctx)

		if len(offlineIDs) > 0 {
			update := bson.M{}
			if coll == collPort {
				update = bson.M{"$set": bson.M{"state": "closed", "updated_at": time.Now()}}
			} else if coll == collHTTP {
				update = bson.M{"$set": bson.M{"status_code": 0, "title": "Offline", "updated_at": time.Now()}}
			} else {
				update = bson.M{"$set": bson.M{"updated_at": time.Now()}}
			}
			_, _ = r.db.Collection(coll).UpdateMany(ctx, bson.M{"_id": bson.M{"$in": offlineIDs}}, update) // @check-ignore: offline diff: scoped by task/project, not user
		}
	}
	return nil
}

// DashboardCounts 返回各资产集合的文档总数，用于 Dashboard 统计卡片。
type DashboardCounts struct {
	Subdomains int64 `json:"subdomains"`
	Ports      int64 `json:"ports"`
	HTTP       int64 `json:"http"`
	Vulns      int64 `json:"vulns"`
	Dirs       int64 `json:"dirs"`
	Crawler    int64 `json:"crawler"`
	Sensitive  int64 `json:"sensitive"`
}

func (r *AssetRepo) DashboardCounts(ctx context.Context) (*DashboardCounts, error) {
	var c DashboardCounts
	var err error
	filter := bson.M{}
	count := func(coll string) int64 {
		n, e := r.db.Collection(coll).CountDocuments(ctx, filter)
		if e != nil {
			err = e
		}
		return n
	}
	c.Subdomains = count(collSubdomain)
	c.Ports = count(collPort)
	c.HTTP = count(collHTTP)
	c.Vulns = count(collVuln)
	c.Dirs = count(collDir)
	c.Crawler = count(collCrawler)
	c.Sensitive = count(collSensitive)
	return &c, err
}

// VulnSeverityStats 返回漏洞按危险等级的数量分布。
type VulnSeverityStats struct {
	Severity string `json:"severity"`
	Count    int    `json:"count"`
}

func (r *AssetRepo) VulnSeverityStats(ctx context.Context) ([]VulnSeverityStats, error) {
	match := bson.M{}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$severity"},
			{Key: "count", Value: bson.M{"$sum": 1}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
	}
	cursor, err := r.db.Collection(collVuln).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var raw []struct {
		Severity string `bson:"_id"`
		Count    int    `bson:"count"`
	}
	if err := cursor.All(ctx, &raw); err != nil {
		return nil, err
	}
	out := make([]VulnSeverityStats, 0, len(raw))
	for _, r := range raw {
		out = append(out, VulnSeverityStats{Severity: r.Severity, Count: r.Count})
	}
	return out, nil
}

// DailyAssetTrend 返回过去 days 天每天新增的各类资产数量。
type DailyTrendItem struct {
	Date      string `json:"date"`
	Subdomain int    `json:"subdomain"`
	Port      int    `json:"port"`
	HTTP      int    `json:"http"`
	Vuln      int    `json:"vuln"`
}

func (r *AssetRepo) DailyAssetTrend(ctx context.Context, days int) ([]DailyTrendItem, error) {
	if days <= 0 || days > 30 {
		days = 7
	}
	since := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -(days - 1))

	userMatch := bson.M{}

	type collResult struct {
		name string
		col  string
	}
	colls := []collResult{
		{"subdomain", collSubdomain},
		{"port", collPort},
		{"http", collHTTP},
		{"vuln", collVuln},
	}

	// date → item map
	dateMap := make(map[string]*DailyTrendItem)
	for i := 0; i < days; i++ {
		d := since.AddDate(0, 0, i).Format("2006-01-02")
		dateMap[d] = &DailyTrendItem{Date: d}
	}

	for _, c := range colls {
		match := bson.M{"created_at": bson.M{"$gte": since}}
		for k, v := range userMatch {
			match[k] = v
		}
		pipeline := mongo.Pipeline{
			{{Key: "$match", Value: match}},
			{{Key: "$group", Value: bson.D{
				{Key: "_id", Value: bson.D{{Key: "$dateToString", Value: bson.M{"format": "%Y-%m-%d", "date": "$created_at"}}}},
				{Key: "count", Value: bson.M{"$sum": 1}},
			}}},
		}
		cursor, err := r.db.Collection(c.col).Aggregate(ctx, pipeline)
		if err != nil {
			return nil, err
		}
		var rows []struct {
			Date  string `bson:"_id"`
			Count int    `bson:"count"`
		}
		if err := cursor.All(ctx, &rows); err != nil {
			cursor.Close(ctx)
			return nil, err
		}
		cursor.Close(ctx)
		for _, row := range rows {
			item, ok := dateMap[row.Date]
			if !ok {
				continue
			}
			switch c.name {
			case "subdomain":
				item.Subdomain = row.Count
			case "port":
				item.Port = row.Count
			case "http":
				item.HTTP = row.Count
			case "vuln":
				item.Vuln = row.Count
			}
		}
	}

	out := make([]DailyTrendItem, 0, days)
	for i := 0; i < days; i++ {
		d := since.AddDate(0, 0, i).Format("2006-01-02")
		out = append(out, *dateMap[d])
	}
	return out, nil
}

// RecentChanges 返回最近 N 条资产变更记录（跨���目），按时���倒序。
func (r *AssetRepo) RecentChanges(ctx context.Context, limit int64) ([]models.AssetChangeLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	filter := bson.M{}
	cursor, err := r.db.Collection(collChanges).Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var out []models.AssetChangeLog
	if err := cursor.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}
