package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/yourname/nscan/internal/server/nodelog"
)

var upgrader = websocket.Upgrader{
	HandshakeTimeout: 5 * time.Second,
	ReadBufferSize:   1024,
	WriteBufferSize:  4096,
	CheckOrigin:      func(_ *http.Request) bool { return true },
}

// NodeLogs 通过 WebSocket 实时推送节点日志
func (h *Handler) NodeLogs(c *gin.Context) {
	nodeID := c.Param("id")

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// 先 Subscribe，再取历史。反过来的话，两步之间到达的事件既不在历史里也不在
	// 订阅通道里，会被静默丢弃。
	events, unsub := h.hub.Subscribe("node:" + nodeID)
	defer unsub()

	// 推送历史日志
	history := h.nodeLog.Get(nodeID, 200)
	for _, e := range history {
		if err := conn.WriteJSON(e); err != nil {
			return
		}
	}

	closeCh := make(chan struct{})
	go func() {
		defer close(closeCh)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case e, ok := <-events:
			if !ok {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			entry := nodelog.Entry{Time: time.Now(), NodeID: nodeID, Level: e.Level, Log: e.Log, Kind: e.Kind, Data: e.Data}
			if err := conn.WriteJSON(entry); err != nil {
				return
			}
		case <-closeCh:
			return
		}
	}
}

// TaskProgress 通过 WebSocket 推送任务实时进度
func (h *Handler) TaskProgress(c *gin.Context) {
	taskID := c.Param("id")

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// 先订阅，再发送快照，避免丢失订阅期间到达的事件
	events, unsub := h.hub.Subscribe(taskID)
	defer unsub()

	// 发送当前进度快照（重放已完成的阶段状态）
	for _, e := range h.taskProg.Get(taskID) {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteJSON(e); err != nil {
			return
		}
	}

	// 读取客户端关闭帧（忽略内容，只用于检测断开）
	closeCh := make(chan struct{})
	go func() {
		defer close(closeCh)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	conn.SetWriteDeadline(time.Now().Add(60 * time.Second))
	for {
		select {
		case e, ok := <-events:
			if !ok {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteJSON(e); err != nil {
				return
			}
		case <-closeCh:
			return
		}
	}
}
