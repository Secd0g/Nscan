package grpc

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/yourname/nscan/internal/server/hub"
	"github.com/yourname/nscan/internal/server/nodelog"
	"github.com/yourname/nscan/pkg/models"
	"github.com/yourname/nscan/pkg/proto/scanv1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const (
	sendBufSize      = 64               // 每个节点发送缓冲大小
	heartbeatTimeout = 60 * time.Second // 超过此时间未收到心跳则判断节点离线
)

// shortID 返回 taskID 的前 8 字符（不足则原样返回），避免直接切片导致越界 panic。
func shortID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	if s == "" {
		return "<empty>"
	}
	return s
}

// ResultHandler 扫描结果处理回调，由上层（Scheduler/Service）注入
type ResultHandler interface {
	OnResult(result *scanv1.TaskResult)
	OnProgress(progress *scanv1.TaskProgress)
	OnStatusUpdate(update *scanv1.TaskStatusUpdate)
	TaskLabel(taskID string) string // 返回 "任务名(项目名)" 用于日志显示
	// Phase 3: subtask queue callbacks
	OnSubtaskComplete(ctx context.Context, msg *scanv1.SubtaskComplete)
	OnAIPentestResult(result *scanv1.AIPentestResult)
}

// NodeOfflineHook is called when a node disconnects (heartbeat timeout or stream end).
// nodeID is the disconnected node's identifier.
type NodeOfflineHook func(ctx context.Context, nodeID string)

// Server gRPC 服务，实现 ScanServiceServer 接口
type Server struct {
	scanv1.UnimplementedScanServiceServer

	tokenProvider   interface{ Get() string }
	nm              *NodeManager
	handler         ResultHandler
	nodeLog         *nodelog.Store
	hub             *hub.Hub
	log             *zap.Logger
	srv             *grpc.Server
	nodeOfflineHook NodeOfflineHook
}

// SetNodeOfflineHook registers a callback invoked whenever a node goes offline.
func (s *Server) SetNodeOfflineHook(hook NodeOfflineHook) {
	s.nodeOfflineHook = hook
}

func NewServer(tokenProvider interface{ Get() string }, nm *NodeManager, handler ResultHandler, nodeLog *nodelog.Store, h *hub.Hub, log *zap.Logger) *Server {
	return &Server{
		tokenProvider: tokenProvider,
		nm:            nm,
		handler:       handler,
		nodeLog:       nodeLog,
		hub:           h,
		log:           log,
	}
}

func (s *Server) ListenAndServe(addr string, creds credentials.TransportCredentials) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("grpc listen %s: %w", addr, err)
	}

	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute,
			Time:              30 * time.Second,
			Timeout:           10 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             15 * time.Second,
			PermitWithoutStream: true,
		}),
	}
	if creds != nil {
		opts = append(opts, grpc.Creds(creds))
	}

	s.srv = grpc.NewServer(opts...)
	scanv1.RegisterScanServiceServer(s.srv, s)

	s.log.Info("gRPC server listening", zap.String("addr", addr))
	return s.srv.Serve(lis)
}

func (s *Server) Stop() {
	if s.srv != nil {
		s.srv.GracefulStop()
	}
}

// Connect 是唯一的 RPC，处理节点双向流
func (s *Server) Connect(stream scanv1.ScanService_ConnectServer) error {
	// ── Step 1: 等待节点发送 RegisterRequest ──────────────────────────────────
	firstMsg, err := stream.Recv()
	if err != nil {
		return err
	}
	reg, ok := firstMsg.Payload.(*scanv1.ScannerMessage_Register)
	if !ok {
		return status.Error(codes.InvalidArgument, "first message must be RegisterRequest")
	}

	// ── Step 2: 验证 token ────────────────────────────────────────────────────
	if reg.Register.Token != s.tokenProvider.Get() {
		_ = stream.Send(&scanv1.ServerMessage{
			Payload: &scanv1.ServerMessage_Ack{
				Ack: &scanv1.RegisterAck{Accepted: false, Message: "invalid token"},
			},
		})
		return status.Error(codes.Unauthenticated, "invalid token")
	}

	// ── Step 3: 创建节点记录 ──────────────────────────────────────────────────
	nodeID := generateNodeID(reg.Register.Name)
	peerAddr := ""
	if p, ok := peer.FromContext(stream.Context()); ok {
		if host, _, err := net.SplitHostPort(p.Addr.String()); err == nil {
			peerAddr = host
		} else {
			peerAddr = p.Addr.String()
		}
		// IPv6 loopback → IPv4
		if peerAddr == "::1" {
			peerAddr = "127.0.0.1"
		}
	}
	node := &models.Node{
		ID:             nodeID,
		Name:           reg.Register.Name,
		Addr:           peerAddr,
		Status:         models.NodeStatusOnline,
		Capabilities:   reg.Register.Capabilities,
		InstalledTools: reg.Register.InstalledTools,
		MaxTasks:       reg.Register.MaxTasks,
		Version:        reg.Register.Version,
		RegisteredAt:   time.Now(),
		LastSeenAt:     time.Now(),
	}

	ctx, cancel := context.WithCancel(stream.Context())
	sendCh := make(chan *scanv1.ServerMessage, sendBufSize)

	conn := &nodeConn{node: node, send: sendCh, cancel: cancel}
	s.nm.Register(conn)
	defer func() {
		offMsg := fmt.Sprintf("节点离线: %s", reg.Register.Name)
		s.nodeLog.Append(nodeID, offMsg, "warn")
		s.hub.Publish("node:"+nodeID, hub.Event{Kind: "log", Log: offMsg, Level: "warn"})
		s.nm.Unregister(nodeID)
		if s.nodeOfflineHook != nil {
			go s.nodeOfflineHook(context.Background(), nodeID)
		}
	}()
	defer cancel()

	regMsg := fmt.Sprintf("节点上线: %s (%s), 能力: %v, 工具: %v, 版本: %s", reg.Register.Name, peerAddr, reg.Register.Capabilities, reg.Register.InstalledTools, reg.Register.Version)
	s.nodeLog.Append(nodeID, regMsg, "info")
	s.hub.Publish("node:"+nodeID, hub.Event{Kind: "log", Log: regMsg, Level: "info"})

	// ── Step 4: 回复注册确认 ──────────────────────────────────────────────────
	if err := stream.Send(&scanv1.ServerMessage{
		Payload: &scanv1.ServerMessage_Ack{
			Ack: &scanv1.RegisterAck{NodeId: nodeID, Accepted: true, Message: "welcome"},
		},
	}); err != nil {
		return err
	}

	// ── Step 5: 启动发送 goroutine ────────────────────────────────────────────
	sendErr := make(chan error, 1)
	go func() {
		for {
			select {
			case msg, ok := <-sendCh:
				if !ok {
					sendErr <- nil
					return
				}
				if err := stream.Send(msg); err != nil {
					sendErr <- err
					return
				}
			case <-ctx.Done():
				sendErr <- nil
				return
			}
		}
	}()

	// ── Step 6: 接收节点消息主循环 ────────────────────────────────────────────
	heartbeatTimer := time.NewTimer(heartbeatTimeout)
	defer heartbeatTimer.Stop()

	recvErr := make(chan error, 1)
	msgCh := make(chan *scanv1.ScannerMessage, 32)

	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				recvErr <- err
				return
			}
			// 主循环退出后 msgCh 无人读，这里不能无条件阻塞发送，否则 goroutine 泄漏。
			select {
			case msgCh <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case msg := <-msgCh:
			heartbeatTimer.Reset(heartbeatTimeout)
			s.handleScannerMessage(stream.Context(), nodeID, msg)

		case <-heartbeatTimer.C:
			s.log.Warn("node heartbeat timeout, closing connection",
				zap.String("node_id", nodeID))
			return status.Error(codes.DeadlineExceeded, "heartbeat timeout")

		case err := <-recvErr:
			if err == io.EOF {
				s.log.Info("node stream EOF", zap.String("node_id", nodeID))
				return nil
			}
			s.log.Warn("node recv error", zap.String("node_id", nodeID), zap.Error(err))
			return err

		case err := <-sendErr:
			s.log.Warn("node send error", zap.String("node_id", nodeID), zap.Error(err))
			return err

		case <-ctx.Done():
			s.log.Info("node ctx done", zap.String("node_id", nodeID))
			return nil
		}
	}
}

func (s *Server) handleScannerMessage(ctx context.Context, nodeID string, msg *scanv1.ScannerMessage) {
	switch p := msg.Payload.(type) {
	case *scanv1.ScannerMessage_Heartbeat:
		s.nm.UpdateHeartbeat(nodeID, p.Heartbeat)

	case *scanv1.ScannerMessage_Result:
		label := shortID(p.Result.TaskId)
		if s.handler != nil {
			label = s.handler.TaskLabel(p.Result.TaskId)
			s.handler.OnResult(p.Result)
		}
		dataSnippet := string(p.Result.Data)
		if len(dataSnippet) > 200 {
			dataSnippet = dataSnippet[:200] + "..."
		}
		resultMsg := fmt.Sprintf("[%s] 收到 %s 结果: %s", label, p.Result.ResultType, dataSnippet)
		s.nodeLog.Append(nodeID, resultMsg, "debug")
		s.hub.Publish("node:"+nodeID, hub.Event{Kind: "log", Log: resultMsg, Level: "debug"})

	case *scanv1.ScannerMessage_Progress:
		if p.Progress.Log != "" {
			var logMsg string
			if p.Progress.TaskId == "" {
				logMsg = p.Progress.Log
			} else {
				label := shortID(p.Progress.TaskId)
				if s.handler != nil {
					label = s.handler.TaskLabel(p.Progress.TaskId)
				}
				logMsg = fmt.Sprintf("[%s] %s", label, p.Progress.Log)
			}
			s.nodeLog.Append(nodeID, logMsg, p.Progress.Level)
			s.hub.Publish("node:"+nodeID, hub.Event{
				Kind:  "log",
				Log:   logMsg,
				Level: p.Progress.Level,
			})
		}
		if p.Progress.TaskId != "" && s.handler != nil {
			s.handler.OnProgress(p.Progress)
		}

	case *scanv1.ScannerMessage_Status:
		label := shortID(p.Status.TaskId)
		if s.handler != nil {
			label = s.handler.TaskLabel(p.Status.TaskId)
		}
		statusMsg := fmt.Sprintf("[%s] 状态变更: %s", label, p.Status.Status)
		if p.Status.Error != "" {
			statusMsg += " (" + p.Status.Error + ")"
		}
		s.nodeLog.Append(nodeID, statusMsg, "info")
		s.hub.Publish("node:"+nodeID, hub.Event{Kind: "log", Log: statusMsg, Level: "info"})
		if s.handler != nil {
			s.handler.OnStatusUpdate(p.Status)
		}

	case *scanv1.ScannerMessage_InstallResult:
		ir := p.InstallResult
		status := "成功"
		level := "info"
		if !ir.Success {
			status = "失败: " + ir.ErrorMsg
			level = "error"
		}
		installMsg := fmt.Sprintf("[安装工具] %s 安装%s", ir.ToolName, status)
		s.nodeLog.Append(nodeID, installMsg, level)
		s.hub.Publish("node:"+nodeID, hub.Event{Kind: "log", Log: installMsg, Level: level})
		// update node's installed tools
		s.nm.UpdateInstalledTools(nodeID, ir.InstalledTools)
		// broadcast install result to frontend
		s.hub.Publish("node:"+nodeID, hub.Event{
			Kind: "install_result",
			Data: map[string]interface{}{
				"tool_name":       ir.ToolName,
				"success":         ir.Success,
				"error_msg":       ir.ErrorMsg,
				"installed_tools": ir.InstalledTools,
			},
		})

	case *scanv1.ScannerMessage_SubtaskProgress:
		sp := p.SubtaskProgress
		if sp.Log != "" {
			label := shortID(sp.TaskId)
			if s.handler != nil && sp.TaskId != "" {
				label = s.handler.TaskLabel(sp.TaskId)
			}
			logMsg := fmt.Sprintf("[%s/%s] %s", label, sp.Stage, sp.Log)
			s.nodeLog.Append(nodeID, logMsg, sp.Level)
			s.hub.Publish("node:"+nodeID, hub.Event{Kind: "log", Log: logMsg, Level: sp.Level})
		}
		// Queue-mode stages report SubtaskProgress instead of TaskProgress.
		// Forward it through the task handler too, otherwise the event is only
		// visible in node logs and never reaches task details or log replay.
		if s.handler != nil {
			s.handler.OnProgress(&scanv1.TaskProgress{
				TaskId:  sp.TaskId,
				Stage:   sp.Stage,
				Percent: sp.Percent,
				Message: sp.Message,
				Log:     sp.Log,
				Level:   sp.Level,
			})
		}

	case *scanv1.ScannerMessage_SubtaskComplete:
		sc := p.SubtaskComplete
		sc.NodeId = nodeID
		logMsg := fmt.Sprintf("[subtask:%s/%s] complete success=%v", shortID(sc.SubtaskId), sc.Stage, sc.Success)
		s.nodeLog.Append(nodeID, logMsg, "info")
		if s.handler != nil {
			go s.handler.OnSubtaskComplete(ctx, sc)
		}
	case *scanv1.ScannerMessage_AiPentestResult:
		result := p.AiPentestResult
		result.NodeId = nodeID
		if result.Log != "" {
			s.nodeLog.Append(nodeID, "[AI渗透/"+shortID(result.TaskId)+"] "+result.Log, "info")
		}
		if s.handler != nil {
			s.handler.OnAIPentestResult(result)
		}
	}
}

func generateNodeID(name string) string {
	return fmt.Sprintf("%s-%d", name, time.Now().UnixMilli())
}

// 从 gRPC metadata 中取 token（备用方案，当前用 RegisterRequest 里的 token）
func tokenFromMD(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}
