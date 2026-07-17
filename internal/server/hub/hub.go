package hub

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Event 是推送给前端的实时消息
type Event struct {
	TaskID  string                 `json:"task_id"`
	Kind    string                 `json:"kind"`             // "progress" | "status" | "log" | "install_result"
	Stage   string                 `json:"stage,omitempty"`
	Percent int32                  `json:"percent,omitempty"`
	Message string                 `json:"message,omitempty"`
	Status  string                 `json:"status,omitempty"`
	Log     string                 `json:"log,omitempty"`
	Level   string                 `json:"level,omitempty"`  // "info" | "warn" | "error" | "debug"
	Data    map[string]interface{} `json:"data,omitempty"`
}

type taskGroup struct {
	subs   []chan Event
	cancel func()
}

// Hub 管理按 task_id 分组的分布式订阅者
type Hub struct {
	rdb    *redis.Client
	log    *zap.Logger
	mu     sync.RWMutex
	groups map[string]*taskGroup
}

func New(rdb *redis.Client, log *zap.Logger) *Hub {
	return &Hub{
		rdb:    rdb,
		log:    log,
		groups: make(map[string]*taskGroup),
	}
}

// Subscribe 返回一个事件 channel 和取消订阅的 cleanup 函数
func (h *Hub) Subscribe(taskID string) (<-chan Event, func()) {
	ch := make(chan Event, 64)
	h.mu.Lock()
	defer h.mu.Unlock()

	g, ok := h.groups[taskID]
	if !ok {
		ctx, cancel := context.WithCancel(context.Background())
		g = &taskGroup{
			subs:   []chan Event{ch},
			cancel: cancel,
		}
		h.groups[taskID] = g
		go h.listenRedis(ctx, taskID, g)
	} else {
		g.subs = append(g.subs, ch)
	}

	cleanup := func() {
		h.mu.Lock()
		defer h.mu.Unlock()

		g, ok := h.groups[taskID]
		if !ok {
			return
		}

		for i, s := range g.subs {
			if s == ch {
				g.subs = append(g.subs[:i], g.subs[i+1:]...)
				close(ch)
				break
			}
		}

		if len(g.subs) == 0 {
			g.cancel()
			delete(h.groups, taskID)
		}
	}

	return ch, cleanup
}

func (h *Hub) listenRedis(ctx context.Context, taskID string, g *taskGroup) {
	pubsub := h.rdb.Subscribe(ctx, "nscan:hub:chan:"+taskID)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var e Event
			if err := json.Unmarshal([]byte(msg.Payload), &e); err == nil {
				h.mu.RLock()
				if currentGroup, exists := h.groups[taskID]; exists && currentGroup == g {
					for _, localCh := range g.subs {
						select {
						case localCh <- e:
						default:
						}
					}
				}
				h.mu.RUnlock()
			}
		}
	}
}

// Publish 向 taskID 的所有订阅者广播事件（非阻塞，慢消费者丢弃）
func (h *Hub) Publish(taskID string, e Event) {
	data, err := json.Marshal(e)
	if err != nil {
		h.log.Warn("marshal event failed", zap.Error(err))
		return
	}

	ctx := context.Background()
	_ = h.rdb.Publish(ctx, "nscan:hub:chan:"+taskID, data).Err()
}
