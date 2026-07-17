package nodelog

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

const maxPerNode = 2000

// Entry 一条节点日志
type Entry struct {
	Time   time.Time              `json:"time"`
	NodeID string                 `json:"node_id"`
	Level  string                 `json:"level,omitempty"`
	Log    string                 `json:"log,omitempty"`
	Kind   string                 `json:"kind,omitempty"`
	Data   map[string]interface{} `json:"data,omitempty"`
}

// Store 每个节点维护一个有界日志缓冲，保存在 Redis 中
type Store struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Store {
	return &Store{rdb: rdb}
}

func nodeLogKey(nodeID string) string {
	return "nscan:node:logs:" + nodeID
}

func (s *Store) Append(nodeID, log, level string) {
	if log == "" {
		return
	}
	e := Entry{Time: time.Now(), NodeID: nodeID, Level: level, Log: log}
	data, err := json.Marshal(e)
	if err != nil {
		return
	}

	ctx := context.Background()
	key := nodeLogKey(nodeID)

	// 使用 Redis List 存储，RPush 追加到尾部
	_ = s.rdb.RPush(ctx, key, data).Err()
	// LTrim 裁剪列表，仅保留最后 maxPerNode 个元素
	_ = s.rdb.LTrim(ctx, key, -maxPerNode, -1).Err()
	// 设置过期时间，以防废弃节点占用空间
	_ = s.rdb.Expire(ctx, key, 7*24*time.Hour).Err()
}

// Get 返回最近 limit 条日志（limit<=0 返回全部）
func (s *Store) Get(nodeID string, limit int) []Entry {
	ctx := context.Background()
	key := nodeLogKey(nodeID)

	var start int64
	if limit > 0 {
		start = -int64(limit)
	} else {
		start = 0
	}

	res, err := s.rdb.LRange(ctx, key, start, -1).Result()
	if err != nil || len(res) == 0 {
		return nil
	}

	out := make([]Entry, len(res))
	for i, val := range res {
		var e Entry
		if err := json.Unmarshal([]byte(val), &e); err == nil {
			out[i] = e
		}
	}
	return out
}

// Clear 清除节点日志（节点被删除时调用）
func (s *Store) Clear(nodeID string) {
	ctx := context.Background()
	key := nodeLogKey(nodeID)
	_ = s.rdb.Del(ctx, key).Err()
}
