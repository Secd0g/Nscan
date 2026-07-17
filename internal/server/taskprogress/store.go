package taskprogress

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yourname/nscan/internal/server/hub"
)

// Store 保存每个任务各阶段的最新进度，供 WS 连接时回放
type Store struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Store {
	return &Store{rdb: rdb}
}

func taskKey(taskID string) string {
	return "nscan:task:progress:" + taskID
}

// Update 更新指定任务的某个阶段进度
func (s *Store) Update(taskID string, e hub.Event) {
	key := e.Stage
	if e.Kind == "status" {
		key = "__status__"
	}

	data, err := json.Marshal(e)
	if err != nil {
		return
	}

	ctx := context.Background()
	rKey := taskKey(taskID)

	// 使用 Redis Hash 存储
	_ = s.rdb.HSet(ctx, rKey, key, data).Err()
	// 设置 7 天有效期防止内存泄露
	_ = s.rdb.Expire(ctx, rKey, 7*24*time.Hour).Err()
}

// Get 返回某任务所有阶段的最新进度快照
func (s *Store) Get(taskID string) []hub.Event {
	ctx := context.Background()
	rKey := taskKey(taskID)

	res, err := s.rdb.HGetAll(ctx, rKey).Result()
	if err != nil || len(res) == 0 {
		return nil
	}

	out := make([]hub.Event, 0, len(res))
	for _, val := range res {
		var e hub.Event
		if err := json.Unmarshal([]byte(val), &e); err == nil {
			out = append(out, e)
		}
	}
	return out
}

// Delete 任务完成/删除后清理
func (s *Store) Delete(taskID string) {
	ctx := context.Background()
	rKey := taskKey(taskID)
	_ = s.rdb.Del(ctx, rKey).Err()
}
