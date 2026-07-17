package scheduler

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/yourname/nscan/internal/server/dedup"
	grpcsvr "github.com/yourname/nscan/internal/server/grpc"
	"github.com/yourname/nscan/internal/server/hub"
	"github.com/yourname/nscan/internal/server/notify"
	"github.com/yourname/nscan/internal/server/queue"
	"github.com/yourname/nscan/internal/server/repositories"
	"github.com/yourname/nscan/internal/server/taskprogress"
	"github.com/yourname/nscan/pkg/models"
	"github.com/yourname/nscan/pkg/proto/scanv1"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// Scheduler 负责将待执行任务分发到合适的扫描节点
type Scheduler struct {
	db              *mongo.Database
	rdb             *redis.Client
	nm              *grpcsvr.NodeManager
	hub             *hub.Hub
	taskProg        *taskprogress.Store
	notifier        *notify.Notifier
	log             *zap.Logger
	blRepo          *repositories.BlacklistRepo
	settings        *repositories.SettingsRepo
	aiResultHandler func(*scanv1.AIPentestResult)

	projCache sync.Map // taskID(ObjectID) -> project_id(ObjectID)，避免每条结果都查库
	progressPersistMu sync.Mutex
	progressPersist  map[string]time.Time

	// Phase 3: subtask queue
	q         *queue.Queue
	queueMode bool // true = split+enqueue; false = legacy PickNode+gRPC

	// Phase 4: distributed dedup
	dedup *dedup.Dedup
}

func (s *Scheduler) SetAIResultHandler(fn func(*scanv1.AIPentestResult)) { s.aiResultHandler = fn }

// projectIDOf 返回任务所属的 project_id（ObjectID），带内存缓存。
func (s *Scheduler) projectIDOf(taskID primitive.ObjectID) (primitive.ObjectID, bool) {
	if v, ok := s.projCache.Load(taskID); ok {
		return v.(primitive.ObjectID), true
	}
	var task struct {
		ProjectID primitive.ObjectID `bson:"project_id"`
	}
	if err := s.db.Collection("tasks").FindOne(context.Background(), bson.M{"_id": taskID}).Decode(&task); err != nil {
		return primitive.ObjectID{}, false
	}
	s.projCache.Store(taskID, task.ProjectID)
	return task.ProjectID, true
}

// taskLabelCache 缓存 taskID → "任务名(项目名)"
var taskLabelCache sync.Map

// shortID 返回不超过 8 字符的 taskID 前缀，空/短字符串安全。
func shortID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	if s == "" {
		return "<empty>"
	}
	return s
}

// TaskLabel 返回 "任务名(项目名)" 用于节点日志显示
func (s *Scheduler) TaskLabel(taskID string) string {
	if v, ok := taskLabelCache.Load(taskID); ok {
		return v.(string)
	}
	oid, err := primitive.ObjectIDFromHex(taskID)
	if err != nil {
		return shortID(taskID)
	}
	var task struct {
		Name      string             `bson:"name"`
		ProjectID primitive.ObjectID `bson:"project_id"`
	}
	if err := s.db.Collection("tasks").FindOne(context.Background(), bson.M{"_id": oid}).Decode(&task); err != nil {
		return shortID(taskID)
	}
	label := task.Name
	var proj struct {
		Name string `bson:"name"`
	}
	if err := s.db.Collection("projects").FindOne(context.Background(), bson.M{"_id": task.ProjectID}).Decode(&proj); err == nil && proj.Name != "" {
		label = fmt.Sprintf("%s(%s)", task.Name, proj.Name)
	}
	taskLabelCache.Store(taskID, label)
	return label
}

func New(db *mongo.Database, rdb *redis.Client, nm *grpcsvr.NodeManager, h *hub.Hub, tp *taskprogress.Store, notifier *notify.Notifier, log *zap.Logger, blRepo *repositories.BlacklistRepo, settings *repositories.SettingsRepo) *Scheduler {
	return &Scheduler{
		db:       db,
		rdb:      rdb,
		nm:       nm,
		hub:      h,
		taskProg: tp,
		notifier: notifier,
		log:      log,
		blRepo:   blRepo,
		settings: settings,
		q:        queue.New(rdb),
		dedup:    dedup.New(rdb),
		progressPersist: make(map[string]time.Time),
	}
}

// OnNodeOffline is called by the gRPC server when a node disconnects. It
// releases all leases held by that node, requeueing or dead-lettering each.
func (s *Scheduler) OnNodeOffline(ctx context.Context, nodeID string) {
	if !s.queueMode {
		return
	}
	released, err := s.q.ReleaseNodeLeases(ctx, nodeID)
	if err != nil {
		s.log.Error("release node leases failed", zap.String("node_id", nodeID), zap.Error(err))
		return
	}
	s.log.Info("released node leases on disconnect",
		zap.String("node_id", nodeID),
		zap.Int("released", released),
	)
}

// EnableQueueMode switches the scheduler from legacy PickNode+gRPC dispatch to
// the Phase-3 subtask queue model. Call this after Redis connectivity is
// confirmed and scanner nodes support BLPop-based subtask execution.
func (s *Scheduler) EnableQueueMode() {
	s.queueMode = true
	s.log.Info("scheduler: queue mode enabled (subtask distribution)")
}

// Run 启动调度循环，每 5 秒轮询一次待调度任务，支持 Redis Pub/Sub 即时触发
func (s *Scheduler) Run(ctx context.Context) {
	// Phase 3: start watchdog for orphaned subtask recovery
	if s.queueMode {
		wd := NewWatchdog(s.q, s.rdb, s.log, s.markTaskFailed)
		go wd.Run(ctx)
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// 启动 Redis 订阅监听以支持即时调度
	go func() {
		pubsub := s.rdb.Subscribe(ctx, "nscan:tasks:trigger")
		defer pubsub.Close()
		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-ch:
				if !ok {
					return
				}
				s.log.Debug("tasks trigger received from Redis, dispatching...")
				s.dispatchPending(ctx)
			}
		}
	}()

	for {
		select {
		case <-ticker.C:
			s.dispatchPending(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) markTaskFailed(ctx context.Context, taskID, reason string) {
	oid, err := primitive.ObjectIDFromHex(taskID)
	if err != nil {
		return
	}
	res, err := s.db.Collection("tasks").UpdateOne(ctx, bson.M{
		"_id": oid,
		"status": bson.M{"$in": []models.TaskStatus{
			models.TaskStatusDispatched, models.TaskStatusRunning,
		}},
	}, bson.M{"$set": bson.M{
		"status":     models.TaskStatusFailed,
		"error":      reason,
		"done_at":    time.Now(),
		"updated_at": time.Now(),
	}})
	if err == nil && res.ModifiedCount > 0 {
		s.notifyTaskEvent(oid, models.NotifyEventTaskFailed, reason)
	}
}

// ErrTaskNameDuplicate 表示待创建的任务与库里已有任务同名。
// API 层可用 errors.Is 判断并映射为 409 Conflict。
var ErrTaskNameDuplicate = errors.New("task name already exists")

// Submit 提交新任务（API 层调用）。
// 任务名全局唯一：同名冲突时返回 ErrTaskNameDuplicate，让调用方决定后续行为
// （API 层直接返回错误给前端；cron runner 会追加秒数重试一次）。
func (s *Scheduler) Submit(ctx context.Context, task *models.Task) error {
	name := strings.TrimSpace(task.Name)
	if name == "" {
		return fmt.Errorf("task name is required")
	}
	task.Name = name
	if task.RunID == "" {
		task.RunID = uuid.NewString()
	}
	// 同名检查（业务层校验，避免对历史数据加唯一索引导致重启失败）
	nameFilter := bson.M{"name": name}
	if !task.UserID.IsZero() {
		nameFilter["user_id"] = task.UserID
	}
	exists, err := s.db.Collection("tasks").CountDocuments(ctx, nameFilter, options.Count().SetLimit(1))
	if err != nil {
		return fmt.Errorf("check task name: %w", err)
	}
	if exists > 0 {
		return fmt.Errorf("%w: %q", ErrTaskNameDuplicate, name)
	}

	task.Status = models.TaskStatusQueued
	task.CreatedAt = time.Now()
	task.UpdatedAt = time.Now()
	res, err := s.db.Collection("tasks").InsertOne(ctx, task)
	if err != nil {
		return fmt.Errorf("insert task: %w", err)
	}
	task.ID = res.InsertedID.(primitive.ObjectID)
	// 立即尝试调度，通知 Redis 频道触发
	s.triggerDispatch(ctx)
	return nil
}

// Rescan 重置已完成/失败的任务，重新加入调度队列
func (s *Scheduler) Rescan(ctx context.Context, taskID primitive.ObjectID) error {
	runID := uuid.NewString()
	_, err := s.db.Collection("tasks").UpdateByID(ctx, taskID, bson.M{
		"$set": bson.M{
			"status":     models.TaskStatusQueued,
			"node_id":    "",
			"error":      "",
			"retries":    0,
			"progress":   nil,
			"started_at": nil,
			"done_at":    nil,
			"run_id":     runID,
			"updated_at": time.Now(),
		},
	})
	if err != nil {
		return err
	}
	s.projCache.Delete(taskID)
	idHex := taskID.Hex()
	// Clear all stale Redis state from the previous run: pending set, stage
	// aggregate output, and dedup seen-sets. Without this, the new run's
	// targets are fully deduplicated as "already seen" and the task ends
	// immediately, or old aggregate output poisons the next stage's results.
	_ = s.q.ClearTask(ctx, idHex)
	_ = s.q.SetTaskRunID(ctx, idHex, runID)
	_ = s.q.ClearCancellation(ctx, idHex)
	s.dedup.Clear(ctx, idHex)
	// Clear stale per-task Redis keys so the new run starts fresh.
	_ = s.rdb.Del(ctx, taskLogKey+idHex)
	s.taskProg.Delete(idHex)
	// 立即尝试调度，通知 Redis 频道触发
	s.triggerDispatch(ctx)
	return nil
}

// CancelCleanup clears Redis state after a task is cancelled, so a subsequent
// Rescan starts with a clean slate. Safe to call even if nothing remains.
func (s *Scheduler) CancelCleanup(ctx context.Context, taskIDHex string) {
	_ = s.q.CancelTask(ctx, taskIDHex)
	_ = s.q.ClearTask(ctx, taskIDHex)
	s.dedup.Clear(ctx, taskIDHex)
	s.taskProg.Delete(taskIDHex)
}

func (s *Scheduler) triggerDispatch(ctx context.Context) {
	if err := s.rdb.Publish(ctx, "nscan:tasks:trigger", "run").Err(); err != nil {
		s.log.Warn("publish tasks trigger failed, fallback to local dispatch", zap.Error(err))
		go s.dispatchPending(context.Background())
	}
}

func (s *Scheduler) dispatchPending(ctx context.Context) {
	coll := s.db.Collection("tasks")
	filter := bson.M{"status": models.TaskStatusQueued}
	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		return
	}
	defer cursor.Close(ctx)

	var tasks []models.Task
	_ = cursor.All(ctx, &tasks)

	for _, task := range tasks {
		if err := s.dispatch(ctx, &task); err != nil {
			s.log.Warn("dispatch failed",
				zap.String("task_id", task.ID.Hex()),
				zap.Error(err),
			)
		}
	}
}

func (s *Scheduler) dispatch(ctx context.Context, task *models.Task) error {
	if s.queueMode {
		return s.dispatchViaQueue(ctx, task)
	}
	return s.dispatchLegacy(ctx, task)
}

// dispatchViaQueue splits the task into subtasks and pushes them to Redis queues.
func (s *Scheduler) dispatchViaQueue(ctx context.Context, task *models.Task) error {
	if task.RunID == "" {
		task.RunID = uuid.NewString()
		if _, err := s.db.Collection("tasks").UpdateByID(ctx, task.ID, bson.M{"$set": bson.M{"run_id": task.RunID}}); err != nil {
			return err
		}
	}
	if err := s.q.SetTaskRunID(ctx, task.ID.Hex(), task.RunID); err != nil {
		return err
	}
	// Queue workers execute the same stages as legacy workers, so inject the
	// server-managed dictionaries, provider keys and proxy before splitting.
	params := make(map[string]string, len(task.Config.Params))
	for k, v := range task.Config.Params {
		params[k] = v
	}
	s.injectProviderKeys(ctx, task.Config.Stages, params)
	s.injectBbotKeys(ctx, task.Config.Stages, params)
	s.injectBruteDicts(ctx, task.Config.Stages, params)
	s.injectOnlineSearchKeys(ctx, task.Config.Stages, params)
	s.injectSensitiveRules(ctx, task.Config.Stages, params)
	s.injectDirWordlists(ctx, task.Config.Stages, params)
	s.injectSubdomainWordlists(ctx, task.Config.Stages, params)
	s.injectSystemProxy(ctx, params)
	task.Config.Params = params
	if s.blRepo != nil {
		rules, _ := s.blRepo.List(ctx)
		for _, r := range rules {
			task.Blacklist = append(task.Blacklist, &scanv1.BlacklistRule{Type: r.Type, Value: r.Value})
		}
	}
	subtasks, err := SplitFirstStage(task)
	if err != nil {
		return fmt.Errorf("split task %s: %w", task.ID.Hex(), err)
	}

	now := time.Now()
	_, err = s.db.Collection("tasks").UpdateByID(ctx, task.ID, bson.M{
		"$set": bson.M{
			"status":     models.TaskStatusDispatched,
			"updated_at": now,
			"started_at": now,
		},
	})
	if err != nil {
		return err
	}

	for _, st := range subtasks {
		// Store metadata for watchdog recovery before enqueue.
		if err := StoreSubtaskMeta(ctx, s.rdb, st); err != nil {
			s.log.Warn("store subtask meta failed", zap.String("subtask_id", st.ID), zap.Error(err))
		}
		if err := s.q.Enqueue(ctx, st); err != nil {
			s.log.Error("enqueue subtask failed",
				zap.String("task_id", task.ID.Hex()),
				zap.String("subtask_id", st.ID),
				zap.Error(err),
			)
		}
	}
	s.log.Info("task enqueued as subtasks",
		zap.String("task_id", task.ID.Hex()),
		zap.String("stage", task.Config.Stages[0]),
		zap.Int("subtasks", len(subtasks)),
	)
	return nil
}

// dispatchLegacy is the Phase 1/2 single-node dispatch path.
func (s *Scheduler) dispatchLegacy(ctx context.Context, task *models.Task) error {
	var node *models.Node
	if len(task.NodeIDs) > 0 {
		node = s.nm.PickNodeFrom(task.NodeIDs)
	} else {
		node = s.nm.PickNode(task.Config.Stages)
	}
	if node == nil {
		// 降级：不检查能力直接分配最低负载节点
		node = s.nm.PickNode(nil)
	}
	if node == nil {
		return fmt.Errorf("no available node for task %s", task.ID.Hex())
	}

	// 更新任务状态为 dispatched
	now := time.Now()
	_, err := s.db.Collection("tasks").UpdateByID(ctx, task.ID, bson.M{
		"$set": bson.M{
			"status":     models.TaskStatusDispatched,
			"node_id":    node.ID,
			"updated_at": now,
			"started_at": now,
		},
	})
	if err != nil {
		return err
	}

	// 注入 provider API keys 到 params（如 subfinder 的第三方数据源密钥）
	params := task.Config.Params
	if params == nil {
		params = map[string]string{}
	}
	s.injectProviderKeys(ctx, task.Config.Stages, params)
	s.injectBbotKeys(ctx, task.Config.Stages, params)
	s.injectBruteDicts(ctx, task.Config.Stages, params)
	s.injectOnlineSearchKeys(ctx, task.Config.Stages, params)
	s.injectSensitiveRules(ctx, task.Config.Stages, params)
	s.injectDirWordlists(ctx, task.Config.Stages, params)
	s.injectSubdomainWordlists(ctx, task.Config.Stages, params)
	s.injectSystemProxy(ctx, params)

	// 获取黑名单规则
	var blacklist []*scanv1.BlacklistRule
	if s.blRepo != nil {
		rules, _ := s.blRepo.List(ctx)
		for _, r := range rules {
			blacklist = append(blacklist, &scanv1.BlacklistRule{
				Type:  r.Type,
				Value: r.Value,
			})
		}
	}

	// 向节点发送任务
	grpcTask := &scanv1.ScanTask{
		TaskId:    task.ID.Hex(),
		ProjectId: task.ProjectID.Hex(),
		Targets:   task.Targets,
		Config: &scanv1.TaskConfig{
			Stages: task.Config.Stages,
			Params: params,
		},
		Blacklist: blacklist,
	}

	sent := s.nm.Send(node.ID, &scanv1.ServerMessage{
		Payload: &scanv1.ServerMessage_Task{Task: grpcTask},
	})
	if !sent {
		// 节点发送失败，回滚为 queued
		_, _ = s.db.Collection("tasks").UpdateByID(ctx, task.ID, bson.M{
			"$set": bson.M{"status": models.TaskStatusQueued, "node_id": ""},
		})
		return fmt.Errorf("send to node %s failed", node.ID)
	}

	s.log.Info("task dispatched",
		zap.String("task_id", task.ID.Hex()),
		zap.String("node_id", node.ID),
	)
	return nil
}

// OnResult 处理来自扫描节点的结果（实现 grpc.ResultHandler 接口）
// 按 project_id + 自然键 upsert，实现跨任务去重：
//
//	subdomain: project_id + domain
//	port:      project_id + ip + port + protocol
//	http:      project_id + url
//	vuln:      project_id + target + template_id
func (s *Scheduler) OnResult(r *scanv1.TaskResult) {
	// 严格校验 taskID：非法则拒绝写入，避免零 ObjectID 污染 task_id 字段。
	taskID, err := primitive.ObjectIDFromHex(r.TaskId)
	if err != nil {
		s.log.Warn("OnResult: 非法 task_id, 丢弃", zap.String("task_id", r.TaskId), zap.Error(err))
		return
	}

	collName := resultCollection(r.ResultType)
	doc := bson.M{}
	if err := json.Unmarshal(r.Data, &doc); err != nil {
		s.log.Warn("unmarshal result failed", zap.Error(err))
		return
	}
	doc["task_id"] = taskID
	pid, ok := s.projectIDOf(taskID)
	if !ok {
		// project_id 缺失时不能去重（否则不同项目相同 URL 会被合并到"project_id=nil"这一条）。
		// 直接丢弃即可 —— 一般来说 task 存在就能查到 project_id；查不到基本是任务已被删除。
		s.log.Warn("OnResult: 找不到任务的 project_id, 丢弃结果",
			zap.String("task_id", r.TaskId), zap.String("type", r.ResultType))
		return
	}
	doc["project_id"] = pid
	var owner struct { UserID primitive.ObjectID `bson:"user_id"` }
	if err := s.db.Collection("tasks").FindOne(context.Background(), bson.M{"_id": taskID}).Decode(&owner); err == nil && !owner.UserID.IsZero() {
		doc["user_id"] = owner.UserID
	}

	// Handle screenshot: extract base64 PNG, save to disk, replace with filename.
	if r.ResultType == "http" {
		if pngB64, ok := doc["screenshot_png"].(string); ok && pngB64 != "" {
			delete(doc, "screenshot_png")
			if pngData, err := base64.StdEncoding.DecodeString(pngB64); err == nil && len(pngData) > 0 {
				urlVal, _ := doc["url"].(string)
				hash := fmt.Sprintf("%x", md5.Sum([]byte(urlVal)))
				screenshotDir := "images/screenshots"
				_ = os.MkdirAll(screenshotDir, 0o755)
				fname := hash
				fpath := filepath.Join(screenshotDir, hash+".png")
				if err := os.WriteFile(fpath, pngData, 0o644); err == nil {
					doc["screenshot"] = fname
				}
			}
		} else {
			delete(doc, "screenshot_png")
		}
	}

	now := time.Now()

	filter := deduplicationFilter(r.ResultType, doc, pid)

	if filter != nil {
		setOnInsert := bson.M{"created_at": now}
		doc["updated_at"] = now
		delete(doc, "created_at")
		delete(doc, "id")
		// 收集所有来源到 sources 数组，兼容旧扫描节点的单值 source 字段
		var sourcesToAdd []interface{}
		seen := map[string]bool{}
		addSource := func(s string) {
			if s == "" || seen[s] {
				return
			}
			seen[s] = true
			sourcesToAdd = append(sourcesToAdd, s)
		}
		// HTTP 资产保留 source 单值语义（首次发现者胜出）
		if src, ok := doc["source"].(string); ok && src != "" {
			setOnInsert["source"] = src
			if r.ResultType != "http" {
				addSource(src)
			}
		}
		delete(doc, "source")
		// 新扫描节点直接写 sources 数组
		if raw, ok := doc["sources"]; ok {
			switch v := raw.(type) {
			case []interface{}:
				for _, s := range v {
					if str, ok := s.(string); ok {
						addSource(str)
					}
				}
			case []string:
				for _, s := range v {
					addSource(s)
				}
			}
		}
		delete(doc, "sources")
		update := bson.M{"$set": doc, "$setOnInsert": setOnInsert}
		if len(sourcesToAdd) > 0 {
			update["$addToSet"] = bson.M{"sources": bson.M{"$each": sourcesToAdd}}
		}

		tracked := r.ResultType == "subdomain" || r.ResultType == "port" || r.ResultType == "http"
		if tracked {
			fopts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.Before)
			var oldDoc bson.M
			err := s.db.Collection(collName).FindOneAndUpdate(context.Background(), filter, update, fopts).Decode(&oldDoc)
			if err != nil && err != mongo.ErrNoDocuments {
				s.log.Warn("upsert result failed", zap.String("type", r.ResultType), zap.Error(err))
				return
			}
			if oldDoc != nil {
				s.recordAssetChange(r.ResultType, oldDoc, doc, pid)
			}
		} else {
			opts := options.Update().SetUpsert(true)
			_, err := s.db.Collection(collName).UpdateOne(context.Background(), filter, update, opts)
			if err != nil {
				s.log.Warn("upsert result failed", zap.String("type", r.ResultType), zap.Error(err))
				return
			}
		}
	} else {
		doc["created_at"] = now
		_, err := s.db.Collection(collName).InsertOne(context.Background(), doc)
		if err != nil {
			s.log.Warn("insert result failed", zap.String("type", r.ResultType), zap.Error(err))
			return
		}
	}

	// 发现漏洞时推送通知
	if r.ResultType == "vuln" {
		name, _ := doc["name"].(string)
		severity, _ := doc["severity"].(string)
		target, _ := doc["target"].(string)
		if name == "" {
			name = "未命名漏洞"
		}
		title := fmt.Sprintf("[%s] 发现漏洞: %s", strings.ToUpper(severity), name)
		body := fmt.Sprintf("目标: %s\n严重程度: %s\n漏洞: %s", target, severity, name)
		s.notifier.Notify(models.NotifyEventVulnFound, title, body)
	}
}

// OnAIPentestResult is handled by the API handler, which owns the AI job state.
func (s *Scheduler) OnAIPentestResult(result *scanv1.AIPentestResult) {
	if s.aiResultHandler != nil {
		s.aiResultHandler(result)
	}
}

// deduplicationFilter 返回去重用的查询条件；无法构建时返回 nil。
func deduplicationFilter(resultType string, doc bson.M, pid interface{}) bson.M {
	switch resultType {
	case "subdomain":
		domain, _ := doc["domain"].(string)
		if domain == "" {
			return nil
		}
		return bson.M{"project_id": pid, "domain": domain}
	case "port":
		ip, _ := doc["ip"].(string)
		if ip == "" {
			return nil
		}
		port := doc["port"]
		protocol := doc["protocol"]
		if protocol == nil || protocol == "" {
			protocol = "tcp"
		}
		return bson.M{"project_id": pid, "ip": ip, "port": port, "protocol": protocol}
	case "http":
		url, _ := doc["url"].(string)
		if url == "" {
			return nil
		}
		return bson.M{"project_id": pid, "url": url}
	case "vuln":
		target, _ := doc["target"].(string)
		templateID, _ := doc["template_id"].(string)
		if target == "" || templateID == "" {
			return nil
		}
		return bson.M{"project_id": pid, "target": target, "template_id": templateID}
	case "dir":
		url, _ := doc["url"].(string)
		if url == "" {
			return nil
		}
		return bson.M{"project_id": pid, "url": url}
	case "sensitive":
		url, _ := doc["url"].(string)
		ruleID, _ := doc["rule_id"].(string)
		if url == "" || ruleID == "" {
			return nil
		}
		return bson.M{"project_id": pid, "url": url, "rule_id": ruleID}
	default:
		return nil
	}
}

// notifyTaskEvent 加载任务信息并推送任务完成/失败通知。
func (s *Scheduler) notifyTaskEvent(taskID primitive.ObjectID, event, errMsg string) {
	var task models.Task
	if err := s.db.Collection("tasks").FindOne(context.Background(), bson.M{"_id": taskID}).Decode(&task); err != nil {
		return
	}
	var title, body string
	if event == models.NotifyEventTaskDone {
		title = "任务完成: " + task.Name
		body = fmt.Sprintf("扫描任务「%s」已完成。\n阶段: %s\n目标数: %d",
			task.Name, strings.Join(task.Config.Stages, ", "), len(task.Targets))
	} else {
		title = "任务失败: " + task.Name
		body = fmt.Sprintf("扫描任务「%s」执行失败。\n错误: %s", task.Name, errMsg)
	}
	s.notifier.Notify(event, title, body)
}

// OnProgress 处理进度更新和日志推送
func (s *Scheduler) OnProgress(p *scanv1.TaskProgress) {
	// 收到任何进度/日志说明任务已在跑；补一次 dispatched→running
	// 覆盖 scanner 断线重连时错失的 running status
	s.ensureRunning(p.TaskId)

	if p.Log != "" {
		e := hub.Event{
			TaskID: p.TaskId,
			Kind:   "log",
			Stage:  p.Stage,
			Log:    p.Log,
			Level:  p.Level,
		}
		s.hub.Publish(p.TaskId, e)
		s.appendTaskLog(p.TaskId, e)
		return
	}
	e := hub.Event{
		TaskID:  p.TaskId,
		Kind:    "progress",
		Stage:   p.Stage,
		Percent: p.Percent,
		Message: p.Message,
	}
	s.taskProg.Update(p.TaskId, e)
	s.hub.Publish(p.TaskId, e)
	// Persist stage boundary events so getLogs replay can reconstruct stage states.
	if p.Percent == 0 || p.Percent == 100 {
		s.appendTaskLog(p.TaskId, e)
	}

	// 持久化进度到 task 文档，但限制写入频率，避免高并发扫描时每条
	// progress 都触发一次 MongoDB UpdateByID。
	persist := p.Percent == 0 || p.Percent >= 100
	s.progressPersistMu.Lock()
	last := s.progressPersist[p.TaskId]
	if time.Since(last) >= 2*time.Second {
		persist = true
	}
	if persist {
		s.progressPersist[p.TaskId] = time.Now()
	}
	s.progressPersistMu.Unlock()
	if persist {
		if taskID, err := primitive.ObjectIDFromHex(p.TaskId); err == nil {
			s.db.Collection("tasks").UpdateByID(context.Background(), taskID, bson.M{
				"$set": bson.M{
					"progress": models.StageProgress{
						Stage:   p.Stage,
						Percent: p.Percent,
						Message: p.Message,
					},
				},
			})
		}
	}
}

// OnStatusUpdate 处理任务状态变更
func (s *Scheduler) OnStatusUpdate(u *scanv1.TaskStatusUpdate) {
	taskID, err := primitive.ObjectIDFromHex(u.TaskId)
	if err != nil {
		s.log.Warn("OnStatusUpdate: 非法 task_id, 丢弃",
			zap.String("task_id", u.TaskId), zap.Error(err))
		return
	}
	status := models.TaskStatus(u.Status)

	// 如果任务已被用户手动取消，忽略节点发来的任何后续状态更新
	var current models.Task
	if ferr := s.db.Collection("tasks").FindOne(context.Background(), bson.M{"_id": taskID}).Decode(&current); ferr == nil {
		if current.Error == "user cancelled" {
			return
		}
	}

	update := bson.M{"status": status, "updated_at": time.Now()}
	if u.Error != "" {
		update["error"] = u.Error
	}
	if status == models.TaskStatusDone {
		now := time.Now()
		update["done_at"] = now
		update["progress"] = models.StageProgress{Stage: "done", Percent: 100, Message: "扫描完成"}
	} else if status == models.TaskStatusFailed {
		now := time.Now()
		update["done_at"] = now
	}

	if _, err := s.db.Collection("tasks").UpdateByID(
		context.Background(), taskID, bson.M{"$set": update},
	); err != nil {
		s.log.Warn("update task status failed", zap.Error(err))
		return
	}

	// 任务完成/失败时推送通知
	if status == models.TaskStatusDone {
		s.notifyTaskEvent(taskID, models.NotifyEventTaskDone, "")
		// 跨扫描离线资产 Diff
		if projID, ok := s.projectIDOf(taskID); ok {
			repo := repositories.NewAssetRepo(s.db)
			var taskDoc struct {
				Config models.TaskConfig `bson:"config"`
			}
			_ = s.db.Collection("tasks").FindOne(context.Background(), bson.M{"_id": taskID}).Decode(&taskDoc)
			_ = repo.DiffOfflineAssets(context.Background(), projID.Hex(), u.TaskId, taskDoc.Config.Stages)
		}
		s.maybeAnalyzeTask(taskID)
	} else if status == models.TaskStatusFailed {
		s.notifyTaskEvent(taskID, models.NotifyEventTaskFailed, u.Error)
		// 失败重试逻辑
		s.retryIfNeeded(taskID)
	}

	statusEvt := hub.Event{
		TaskID: u.TaskId,
		Kind:   "status",
		Status: u.Status,
	}
	s.taskProg.Update(u.TaskId, statusEvt)
	s.hub.Publish(u.TaskId, statusEvt)

	s.log.Info("task status updated",
		zap.String("task_id", u.TaskId),
		zap.String("status", u.Status),
	)
}

func (s *Scheduler) retryIfNeeded(taskID primitive.ObjectID) {
	ctx := context.Background()
	var task models.Task
	err := s.db.Collection("tasks").FindOne(ctx, bson.M{"_id": taskID}).Decode(&task)
	if err != nil || task.Retries >= 3 || task.Error == "user cancelled" {
		return
	}
	// 重试要把上一轮的时间戳/进度也清掉，否则前端"结束时间/进度条"还是失败那一刻的值。
	_, _ = s.db.Collection("tasks").UpdateByID(ctx, taskID, bson.M{
		"$set": bson.M{
			"status":     models.TaskStatusQueued,
			"node_id":    "",
			"error":      "",
			"progress":   nil,
			"started_at": nil,
			"done_at":    nil,
			"updated_at": time.Now(),
		},
		"$inc": bson.M{"retries": 1},
	})
	s.log.Info("task queued for retry", zap.String("task_id", taskID.Hex()), zap.Int("retries", task.Retries+1))
}

// injectProviderKeys 从 settings 集合读取 provider API keys 并注入到 task params 中
func (s *Scheduler) injectProviderKeys(ctx context.Context, stages []string, params map[string]string) {
	hasSubdomain := false
	for _, st := range stages {
		if st == "subdomain" {
			hasSubdomain = true
			break
		}
	}
	if !hasSubdomain {
		return
	}

	var cfg struct {
		Providers map[string][]string `bson:"providers"`
		Enabled   map[string]bool     `bson:"enabled"`
	}
	err := s.db.Collection("settings").FindOne(ctx, bson.M{"key": "subfinder"}).Decode(&cfg)
	if err != nil {
		s.log.Info("injectProviderKeys: settings 查询失败或无配置", zap.Error(err))
		return
	}
	if len(cfg.Providers) == 0 {
		s.log.Info("injectProviderKeys: providers 为空")
		return
	}

	s.log.Info("injectProviderKeys: 读取到配置",
		zap.Int("providers_count", len(cfg.Providers)),
		zap.Any("enabled", cfg.Enabled),
	)

	active := make(map[string][]string)
	for name, keys := range cfg.Providers {
		if len(keys) > 0 && cfg.Enabled[name] {
			active[name] = keys
			s.log.Info("injectProviderKeys: 启用", zap.String("provider", name), zap.Int("keys", len(keys)))
		} else {
			s.log.Info("injectProviderKeys: 跳过", zap.String("provider", name), zap.Bool("enabled", cfg.Enabled[name]), zap.Int("keys", len(keys)))
		}
	}
	if len(active) == 0 {
		s.log.Info("injectProviderKeys: 没有启用的 provider")
		return
	}

	data, err := json.Marshal(active)
	if err != nil {
		s.log.Warn("injectProviderKeys: marshal 失败", zap.Error(err))
		return
	}
	s.log.Info("injectProviderKeys: 注入成功", zap.String("provider_keys", string(data)))
	params["subdomain.provider_keys"] = string(data)
}

// injectBbotKeys 从 settings 集合读取 bbot API keys 并注入到 task params 中
func (s *Scheduler) injectBbotKeys(ctx context.Context, stages []string, params map[string]string) {
	hasSubdomain := false
	for _, st := range stages {
		if st == "subdomain" {
			hasSubdomain = true
			break
		}
	}
	if !hasSubdomain {
		return
	}

	var cfg struct {
		Providers map[string][]string `bson:"providers"`
		Enabled   map[string]bool     `bson:"enabled"`
	}
	err := s.db.Collection("settings").FindOne(ctx, bson.M{"key": "bbot"}).Decode(&cfg)
	if err != nil || len(cfg.Providers) == 0 {
		return
	}

	active := make(map[string]string)
	for name, keys := range cfg.Providers {
		if len(keys) > 0 && cfg.Enabled[name] {
			active[name] = keys[0]
		}
	}
	if len(active) == 0 {
		return
	}

	data, err := json.Marshal(active)
	if err != nil {
		return
	}
	params["bbot.provider_keys"] = string(data)
}

// ensureRunning 把 dispatched 状态的任务原子升级为 running（并发安全）
// 用 filter 保证只有真正 dispatched 的才会被改，避免覆盖 done/failed
func (s *Scheduler) ensureRunning(taskIDHex string) {
	taskID, err := primitive.ObjectIDFromHex(taskIDHex)
	if err != nil {
		return
	}
	now := time.Now()
	filter := bson.M{"_id": taskID, "status": models.TaskStatusDispatched}
	update := bson.M{"$set": bson.M{
		"status":     models.TaskStatusRunning,
		"started_at": now,
		"updated_at": now,
	}}
	res, err := s.db.Collection("tasks").UpdateOne(context.Background(), filter, update)
	if err != nil || res.ModifiedCount == 0 {
		return
	}
	statusEvt := hub.Event{TaskID: taskIDHex, Kind: "status", Status: string(models.TaskStatusRunning)}
	s.taskProg.Update(taskIDHex, statusEvt)
	s.hub.Publish(taskIDHex, statusEvt)
}

const taskLogKey = "nscan:task:logs:"
const taskLogMax = 2000

func (s *Scheduler) appendTaskLog(taskID string, e hub.Event) {
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	ctx := context.Background()
	key := taskLogKey + taskID
	pipe := s.rdb.Pipeline()
	pipe.RPush(ctx, key, string(data))
	pipe.LTrim(ctx, key, -taskLogMax, -1)
	pipe.Expire(ctx, key, 7*24*time.Hour)
	_, _ = pipe.Exec(ctx)
}

func (s *Scheduler) GetTaskLogs(taskID string) []hub.Event {
	ctx := context.Background()
	items, err := s.rdb.LRange(ctx, taskLogKey+taskID, 0, -1).Result()
	if err != nil {
		return nil
	}
	out := make([]hub.Event, 0, len(items))
	for _, raw := range items {
		var e hub.Event
		if json.Unmarshal([]byte(raw), &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

// injectSensitiveRules 把 sensitive_rules 集合里 active=true 的规则注入 sensitive stage params：
//
//	输出: sensitive.rules = JSON array [{id, name, pattern, severity}]
//
// scanner 拿到后编译正则即可，无需访问 DB。
func (s *Scheduler) injectSensitiveRules(ctx context.Context, stages []string, params map[string]string) {
	hasSensitive := false
	for _, st := range stages {
		if st == "sensitive" {
			hasSensitive = true
			break
		}
	}
	if !hasSensitive {
		return
	}
	cursor, err := s.db.Collection("sensitive_rules").Find(ctx, bson.M{"active": true},
		options.Find().SetProjection(bson.M{"_id": 1, "name": 1, "pattern": 1, "severity": 1}))
	if err != nil {
		s.log.Warn("injectSensitiveRules: 查询失败", zap.Error(err))
		return
	}
	defer cursor.Close(ctx)
	type wire struct {
		ID       string `bson:"_id" json:"id"`
		Name     string `bson:"name" json:"name"`
		Pattern  string `bson:"pattern" json:"pattern"`
		Severity string `bson:"severity" json:"severity"`
	}
	var docs []struct {
		ID       primitive.ObjectID `bson:"_id"`
		Name     string             `bson:"name"`
		Pattern  string             `bson:"pattern"`
		Severity string             `bson:"severity"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return
	}
	if len(docs) == 0 {
		s.log.Info("injectSensitiveRules: 没有启用的规则")
		return
	}
	rules := make([]wire, len(docs))
	for i, d := range docs {
		rules[i] = wire{ID: d.ID.Hex(), Name: d.Name, Pattern: d.Pattern, Severity: d.Severity}
	}
	data, err := json.Marshal(rules)
	if err != nil {
		return
	}
	params["sensitive.rules"] = string(data)
	s.log.Info("injectSensitiveRules: 注入完成", zap.Int("rules", len(rules)))
}

// injectOnlineSearchKeys 把 settings.online_search 中启用的 provider key 注入到 search stage params：
//
//	输入: 有 stage=search
//	输出: search.<provider>.key = "xxx"
//
// scanner 端 search stage 直接读取，无需访问 DB。
func (s *Scheduler) injectOnlineSearchKeys(ctx context.Context, stages []string, params map[string]string) {
	hasSearch := false
	for _, st := range stages {
		if st == "search" {
			hasSearch = true
			break
		}
	}
	if !hasSearch {
		return
	}
	var cfg struct {
		Providers map[string][]string `bson:"providers"`
		Enabled   map[string]bool     `bson:"enabled"`
	}
	err := s.db.Collection("settings").FindOne(ctx, bson.M{"key": "online_search"}).Decode(&cfg)
	if err != nil {
		s.log.Info("injectOnlineSearchKeys: 未配置 online_search", zap.Error(err))
		return
	}
	for name, keys := range cfg.Providers {
		if !cfg.Enabled[name] || len(keys) == 0 || strings.TrimSpace(keys[0]) == "" {
			continue
		}
		params["search."+name+".key"] = strings.TrimSpace(keys[0])
	}
	s.log.Info("injectOnlineSearchKeys: 注入完成")
}

// injectBruteDicts 为每个待扫描协议自动挑选「字典管理」里对应协议且已启用的字典，
// 合并去重后作为 credentials（user:pass 行）注入 params：
//
//	输入: brute.protocols = "ssh,mysql"
//	输出: brute.<proto>.credentials = "root:root\nadmin:admin\n..."
//
// scanner 无需访问 DB。
func (s *Scheduler) injectBruteDicts(ctx context.Context, stages []string, params map[string]string) {
	hasBrute := false
	for _, st := range stages {
		if st == "brute" {
			hasBrute = true
			break
		}
	}
	if !hasBrute {
		return
	}
	protoList := strings.Split(params["brute.protocols"], ",")
	seen := map[string]bool{}
	for _, proto := range protoList {
		proto = strings.TrimSpace(proto)
		if proto == "" || seen[proto] {
			continue
		}
		seen[proto] = true
		s.fillBruteCredentials(ctx, params, proto)
	}
}

func (s *Scheduler) fillBruteCredentials(ctx context.Context, params map[string]string, proto string) {
	cursor, err := s.db.Collection("dicts").Find(ctx, bson.M{
		"category": "password",
		"service":  proto,
		"active":   true,
	}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return
	}
	defer cursor.Close(ctx)
	var docs []struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return
	}

	var lines []string
	for _, d := range docs {
		l, err := s.loadDictLines(ctx, d.ID)
		if err != nil {
			s.log.Warn("brute dict load failed", zap.String("id", d.ID.Hex()), zap.Error(err))
			continue
		}
		lines = append(lines, l...)
	}
	lines = dedupPreserveOrder(lines)
	if len(lines) > 0 {
		params["brute."+proto+".credentials"] = strings.Join(lines, "\n")
	}
}

// injectDirWordlists 展开 dir 阶段要用的目录字典，注入到 dir.wordlist_lines：
//
//	dir.wordlist = "<oid>,<oid>"（用户勾选） → 展开对应字典
//	dir.wordlist = "" 且启用了 dir → 自动使用「字典管理」里 category=directory 且 active 的字典（包含内置 10k 词表）
//
// scanner 端直接消费 wordlist_lines，无需再访问 DB。
// 找不到任何可用字典时不注入，scanner 回落到内置 66 词硬编码 defaultWordlist 兜底。
func (s *Scheduler) injectDirWordlists(ctx context.Context, stages []string, params map[string]string) {
	hasDir := false
	for _, st := range stages {
		if st == "dir" {
			hasDir = true
			break
		}
	}
	if !hasDir {
		return
	}
	raw := strings.TrimSpace(params["dir.wordlist"])
	var ids []primitive.ObjectID
	if raw != "" {
		// 用户显式选择了字典
		for _, idStr := range strings.Split(raw, ",") {
			idStr = strings.TrimSpace(idStr)
			if idStr == "" {
				continue
			}
			oid, err := primitive.ObjectIDFromHex(idStr)
			if err != nil {
				s.log.Warn("injectDirWordlists: invalid dict id", zap.String("id", idStr))
				continue
			}
			ids = append(ids, oid)
		}
	} else {
		// 留空 → 自动挑选所有启用的目录字典（含 seed 出来的 10k 内置词表）
		cursor, err := s.db.Collection("dicts").Find(ctx, bson.M{
			"category": "directory",
			"active":   true,
		}, options.Find().SetProjection(bson.M{"_id": 1}))
		if err == nil {
			var docs []struct {
				ID primitive.ObjectID `bson:"_id"`
			}
			if err := cursor.All(ctx, &docs); err == nil {
				for _, d := range docs {
					ids = append(ids, d.ID)
				}
			}
			cursor.Close(ctx)
		}
	}
	if len(ids) == 0 {
		return
	}
	var lines []string
	for _, oid := range ids {
		l, err := s.loadDictLines(ctx, oid)
		if err != nil {
			s.log.Warn("injectDirWordlists: load failed", zap.String("id", oid.Hex()), zap.Error(err))
			continue
		}
		lines = append(lines, l...)
	}
	lines = dedupPreserveOrder(lines)
	if len(lines) > 0 {
		params["dir.wordlist_lines"] = strings.Join(lines, "\n")
		s.log.Info("injectDirWordlists: 注入完成", zap.Int("dicts", len(ids)), zap.Int("lines", len(lines)))
	}
}

func (s *Scheduler) injectSystemProxy(ctx context.Context, params map[string]string) {
	var cfg models.ProviderConfig
	err := s.db.Collection("settings").FindOne(ctx, bson.M{"key": "system"}).Decode(&cfg)
	if err != nil {
		if err != mongo.ErrNoDocuments {
			s.log.Warn("injectSystemProxy: fetch failed", zap.Error(err))
		}
		return
	}
	if proxyList, ok := cfg.Providers["proxy"]; ok && len(proxyList) > 0 {
		proxyURL := proxyList[0]
		if proxyURL != "" {
			params["global_proxy"] = proxyURL
			s.log.Info("injectSystemProxy: proxy injected", zap.String("proxy", proxyURL))
		}
	}
}

func (s *Scheduler) loadDictLines(ctx context.Context, id primitive.ObjectID) ([]string, error) {
	cursor, err := s.db.Collection("dict_lines").Find(ctx, bson.M{"dict_id": id}, options.Find().SetProjection(bson.M{"line": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var docs []struct {
		Line string `bson:"line"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.Line
	}
	return out, nil
}

// injectSubdomainWordlists 展开 subdomain 阶段要用的字典，注入到 ksubdomain.wordlist_lines
func (s *Scheduler) injectSubdomainWordlists(ctx context.Context, stages []string, params map[string]string) {
	hasSub := false
	for _, st := range stages {
		if st == "subdomain" {
			hasSub = true
			break
		}
	}
	if !hasSub {
		return
	}
	raw := strings.TrimSpace(params["subdomain.wordlist"])
	var ids []primitive.ObjectID
	if raw != "" {
		for _, idStr := range strings.Split(raw, ",") {
			idStr = strings.TrimSpace(idStr)
			if idStr == "" {
				continue
			}
			oid, err := primitive.ObjectIDFromHex(idStr)
			if err != nil {
				continue
			}
			ids = append(ids, oid)
		}
	} else {
		cursor, err := s.db.Collection("dicts").Find(ctx, bson.M{
			"category": "subdomain",
			"active":   true,
		}, options.Find().SetProjection(bson.M{"_id": 1}))
		if err == nil {
			var docs []struct {
				ID primitive.ObjectID `bson:"_id"`
			}
			if err := cursor.All(ctx, &docs); err == nil {
				for _, d := range docs {
					ids = append(ids, d.ID)
				}
			}
			cursor.Close(ctx)
		}
	}
	if len(ids) == 0 {
		return
	}
	var lines []string
	for _, oid := range ids {
		l, err := s.loadDictLines(ctx, oid)
		if err != nil {
			continue
		}
		lines = append(lines, l...)
	}
	lines = dedupPreserveOrder(lines)
	if len(lines) > 0 {
		params["subdomain.wordlist_lines"] = strings.Join(lines, "\n")
		s.log.Info("injectSubdomainWordlists: 注入完成", zap.Int("dicts", len(ids)), zap.Int("lines", len(lines)))
	}
}

func dedupPreserveOrder(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// recordAssetChange 对比旧文档和新 $set 内容，有实际变更时写入 asset_changes 并推送通知。
func (s *Scheduler) recordAssetChange(resultType string, oldDoc, newDoc bson.M, pid interface{}) {
	trackFields := map[string][]string{
		"subdomain": {"ips", "cname"},
		"port":      {"service", "banner", "products"},
		"http":      {"status_code", "title", "server", "tech"},
	}
	fields, ok := trackFields[resultType]
	if !ok {
		return
	}

	var changes []models.FieldChange
	for _, f := range fields {
		oldVal := fmt.Sprint(oldDoc[f])
		newVal := fmt.Sprint(newDoc[f])
		if oldVal == newVal || oldVal == "<nil>" {
			continue
		}
		changes = append(changes, models.FieldChange{Field: f, Old: oldVal, New: newVal})
	}
	if len(changes) == 0 {
		return
	}

	assetID, _ := oldDoc["_id"].(primitive.ObjectID)
	taskID := fmt.Sprint(newDoc["task_id"])
	changeLog := &models.AssetChangeLog{
		AssetID:   assetID,
		AssetType: resultType,
		ProjectID: fmt.Sprint(pid),
		TaskID:    taskID,
		Changes:   changes,
		CreatedAt: time.Now(),
	}
	_, _ = s.db.Collection("asset_changes").InsertOne(context.Background(), changeLog)

	var assetLabel string
	switch resultType {
	case "subdomain":
		assetLabel, _ = oldDoc["domain"].(string)
	case "port":
		assetLabel = fmt.Sprintf("%v:%v", oldDoc["ip"], oldDoc["port"])
	case "http":
		assetLabel, _ = oldDoc["url"].(string)
	}
	fieldSummary := make([]string, 0, len(changes))
	for _, c := range changes {
		fieldSummary = append(fieldSummary, fmt.Sprintf("%s: %s → %s", c.Field, c.Old, c.New))
	}
	title := fmt.Sprintf("资产变更: %s", assetLabel)
	body := fmt.Sprintf("类型: %s\n资产: %s\n变更:\n%s", resultType, assetLabel, strings.Join(fieldSummary, "\n"))
	s.notifier.Notify(models.NotifyEventAssetChanged, title, body)
}

func resultCollection(typ string) string {
	switch typ {
	case "subdomain":
		return "assets_subdomain"
	case "port":
		return "assets_port"
	case "http":
		return "assets_http"
	case "vuln":
		return "assets_vuln"
	case "dir":
		return "assets_dir"
	case "sensitive":
		return "assets_sensitive"
	case "crawler":
		return "assets_crawler"
	default:
		return "assets_other"
	}
}

// ListDeadLetterByTask returns dead-lettered subtasks for a task.
func (s *Scheduler) ListDeadLetterByTask(ctx context.Context, taskID string) ([]*models.Subtask, error) {
	return s.q.ListDeadLetterByTask(ctx, taskID)
}

// RetryDeadLetter resets and re-enqueues a dead-lettered subtask.
func (s *Scheduler) RetryDeadLetter(ctx context.Context, subtaskID string) error {
	return s.q.RetryDeadLetter(ctx, subtaskID)
}

// ListSubtasks returns all subtask metadata for a given task from Redis.
// This is used by the UI to show per-subtask progress in queue mode.
func (s *Scheduler) ListSubtasks(ctx context.Context, taskID string) ([]*models.Subtask, error) {
	ids, err := s.q.PendingMembers(ctx, taskID)
	if err != nil {
		return nil, err
	}
	var result []*models.Subtask
	for _, id := range ids {
		data, err := s.rdb.Get(ctx, "subtask:"+id+":meta").Bytes()
		if err != nil {
			continue
		}
		var st models.Subtask
		if err := json.Unmarshal(data, &st); err == nil {
			// annotate lease status
			owner, _ := s.q.LeaseOwner(ctx, id)
			if owner != "" {
				st.Status = models.SubtaskLeased
				st.LeasedBy = owner
			}
			result = append(result, &st)
		}
	}
	return result, nil
}
