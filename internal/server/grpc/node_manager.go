package grpc

import (
	"sync"
	"time"

	"github.com/yourname/nscan/pkg/models"
	"github.com/yourname/nscan/pkg/proto/scanv1"
	"go.uber.org/zap"
)

// nodeConn 表示一个已连接的扫描节点
type nodeConn struct {
	node   *models.Node
	send   chan *scanv1.ServerMessage // 向节点发送消息的通道
	cancel func()                     // 关闭该节点连接
}

// NodeManager 管理所有在线扫描节点
type NodeManager struct {
	mu    sync.RWMutex
	nodes map[string]*nodeConn // nodeID → nodeConn
	log   *zap.Logger
}

func NewNodeManager(log *zap.Logger) *NodeManager {
	return &NodeManager{
		nodes: make(map[string]*nodeConn),
		log:   log,
	}
}

func (m *NodeManager) Register(conn *nodeConn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// 同名节点重连时替换旧连接
	for id, oldConn := range m.nodes {
		if oldConn.node.Name == conn.node.Name {
			oldConn.cancel()
			delete(m.nodes, id)
			m.log.Info("replaced old node connection", zap.String("old_id", id), zap.String("new_id", conn.node.ID))
		}
	}
	m.nodes[conn.node.ID] = conn
	m.log.Info("node registered", zap.String("node_id", conn.node.ID), zap.String("name", conn.node.Name))
}

func (m *NodeManager) Unregister(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if conn, ok := m.nodes[nodeID]; ok {
		conn.node.Status = models.NodeStatusOffline
		delete(m.nodes, nodeID)
		// 先发 kick 消息让节点侧知道是被主动删除，不应重连
		select {
		case conn.send <- &scanv1.ServerMessage{
			Payload: &scanv1.ServerMessage_Kick{
				Kick: &scanv1.KickNode{Reason: "deleted"},
			},
		}:
		default:
		}
		conn.cancel()
		m.log.Info("node kicked and offline", zap.String("node_id", nodeID))
	}
}

func (m *NodeManager) UpdateHeartbeat(nodeID string, hb *scanv1.Heartbeat) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if conn, ok := m.nodes[nodeID]; ok {
		conn.node.CPUPercent = hb.CpuPercent
		conn.node.MemPercent = hb.MemPercent
		conn.node.ActiveTasks = hb.ActiveTasks
		conn.node.LastSeenAt = time.Now()
	}
}

// snapshotNode 返回节点信息的拷贝，避免调用方（scheduler / API JSON 序列化）
// 与 UpdateHeartbeat / UpdateInstalledTools 之间发生 data race。
func snapshotNode(n *models.Node) *models.Node {
	cp := *n
	if n.Capabilities != nil {
		cp.Capabilities = append([]string(nil), n.Capabilities...)
	}
	if n.InstalledTools != nil {
		cp.InstalledTools = append([]string(nil), n.InstalledTools...)
	}
	return &cp
}

// PickNode 按最小负载选择一个可用节点，能力匹配 required 中的所有项
// 返回节点信息快照（caller 用 node.ID 再调 Send）
func (m *NodeManager) PickNode(required []string) *models.Node {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var best *nodeConn
	for _, conn := range m.nodes {
		if conn.node.Status != models.NodeStatusOnline {
			continue
		}
		if conn.node.ActiveTasks >= conn.node.MaxTasks {
			continue
		}
		if !hasAll(conn.node.Capabilities, required) {
			continue
		}
		if best == nil || conn.node.ActiveTasks < best.node.ActiveTasks {
			best = conn
		}
	}
	if best == nil {
		return nil
	}
	return snapshotNode(best.node)
}

// PickNodeFrom 从指定节点列表中选最低负载的在线节点
func (m *NodeManager) PickNodeFrom(nodeIDs []string) *models.Node {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var best *nodeConn
	for _, id := range nodeIDs {
		conn, ok := m.nodes[id]
		if !ok || conn.node.Status != models.NodeStatusOnline {
			continue
		}
		if conn.node.ActiveTasks >= conn.node.MaxTasks {
			continue
		}
		if best == nil || conn.node.ActiveTasks < best.node.ActiveTasks {
			best = conn
		}
	}
	if best == nil {
		return nil
	}
	return snapshotNode(best.node)
}

// InstallTool 向指定节点发送工具安装指令
func (m *NodeManager) InstallTool(nodeID, toolName, installCmd string, reinstall bool) bool {
	return m.Send(nodeID, &scanv1.ServerMessage{
		Payload: &scanv1.ServerMessage_InstallTool{
			InstallTool: &scanv1.InstallTool{
				ToolName:   toolName,
				InstallCmd: installCmd,
				Reinstall:  reinstall,
			},
		},
	})
}

// UninstallTool 向节点发送工具卸载指令（删除二进制）
func (m *NodeManager) UninstallTool(nodeID, toolName string) bool {
	return m.Send(nodeID, &scanv1.ServerMessage{
		Payload: &scanv1.ServerMessage_InstallTool{
			InstallTool: &scanv1.InstallTool{
				ToolName:  toolName,
				Uninstall: true,
			},
		},
	})
}

// RunAIPentest sends a user-authorized Claude Code job to a node.
func (m *NodeManager) RunAIPentest(nodeID string, taskID, prompt string, targets []string, timeoutSeconds int32, apiKey string) bool {
	return m.Send(nodeID, &scanv1.ServerMessage{Payload: &scanv1.ServerMessage_AiPentest{
		AiPentest: &scanv1.AIPentestTask{TaskId: taskID, Prompt: prompt, Targets: targets, TimeoutSeconds: timeoutSeconds, ApiKey: apiKey},
	}})
}

func (m *NodeManager) CancelAIPentest(nodeID, taskID string) bool {
	return m.Send(nodeID, &scanv1.ServerMessage{Payload: &scanv1.ServerMessage_Cancel{Cancel: &scanv1.CancelTask{TaskId: "ai:" + taskID, Reason: "user stopped AI pentest"}}})
}

// UpdateInstalledTools 更新节点已安装工具列表
func (m *NodeManager) UpdateInstalledTools(nodeID string, tools []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if conn, ok := m.nodes[nodeID]; ok {
		conn.node.InstalledTools = tools
	}
}

// CancelTask 向指定节点发送取消任务指令
func (m *NodeManager) CancelTask(nodeID, taskID string) bool {
	return m.Send(nodeID, &scanv1.ServerMessage{
		Payload: &scanv1.ServerMessage_Cancel{
			Cancel: &scanv1.CancelTask{TaskId: taskID, Reason: "deleted"},
		},
	})
}

// SendCancelTask 向所有在线节点广播取消任务指令
func (m *NodeManager) SendCancelTask(taskID string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, conn := range m.nodes {
		if conn.node.Status != models.NodeStatusOnline {
			continue
		}
		select {
		case conn.send <- &scanv1.ServerMessage{
			Payload: &scanv1.ServerMessage_Cancel{
				Cancel: &scanv1.CancelTask{TaskId: taskID, Reason: "user cancelled"},
			},
		}:
		default:
			m.log.Warn("node send buffer full, dropping cancel task message", zap.String("node_id", conn.node.ID))
		}
	}
	return nil
}

// Send 向指定节点发送消息，非阻塞
func (m *NodeManager) Send(nodeID string, msg *scanv1.ServerMessage) bool {
	m.mu.RLock()
	conn, ok := m.nodes[nodeID]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	select {
	case conn.send <- msg:
		return true
	default:
		m.log.Warn("node send buffer full, dropping message", zap.String("node_id", nodeID))
		return false
	}
}

// Restart 向节点发送重启指令
func (m *NodeManager) Restart(nodeID string) bool {
	return m.Send(nodeID, &scanv1.ServerMessage{
		Payload: &scanv1.ServerMessage_Restart{
			Restart: &scanv1.RestartNode{},
		},
	})
}

// ListNodes 返回所有节点的快照（用于 API），返回拷贝避免 JSON 序列化时与心跳写竞争。
func (m *NodeManager) ListNodes() []*models.Node {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*models.Node, 0, len(m.nodes))
	for _, c := range m.nodes {
		list = append(list, snapshotNode(c.node))
	}
	return list
}

func hasAll(caps, required []string) bool {
	set := make(map[string]struct{}, len(caps))
	for _, c := range caps {
		set[c] = struct{}{}
	}
	for _, r := range required {
		if _, ok := set[r]; !ok {
			return false
		}
	}
	return true
}
