package agent

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yourname/nscan/internal/scanner/config"
	"github.com/yourname/nscan/internal/scanner/engine"
	"github.com/yourname/nscan/pkg/proto/scanv1"
	"github.com/yourname/nscan/pkg/tooldef"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

const (
	initialBackoff    = 1 * time.Second
	maxBackoff        = 30 * time.Second
	heartbeatInterval = 15 * time.Second
)

// shortID 返回 taskID 的前 8 字符前缀，空/短字符串安全。
func shortID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	if s == "" {
		return "<empty>"
	}
	return s
}

// Agent 负责连接服务端、接收任务、上报结果，断线自动重连
type Agent struct {
	cfg    *config.ScannerConfig
	engine *engine.PipelineEngine
	log    *zap.Logger
	nodeID string // 服务端分配，重连后可能变化
	kicked bool   // 被服务端主动踢出（删除节点），不再重连

	// installedTools 由多个 goroutine（runInstallTool/runUninstallTool）修改，
	// 也被 sendInstallResult / register 读取；用 mu 保护。
	mu             sync.Mutex
	installedTools []string

	// sendMu 串行化所有 stream.Send 调用。gRPC 明确规定 ClientStream 的
	// SendMsg 不能被多个 goroutine 并发调用；heartbeat/task/install 三条路径
	// 都会 Send，如果不加锁会破坏流。
	sendMu sync.Mutex
	aiMu   sync.Mutex
	aiJobs map[string]context.CancelFunc
}

func New(cfg *config.ScannerConfig, eng *engine.PipelineEngine, log *zap.Logger, installedTools []string) *Agent {
	return &Agent{cfg: cfg, engine: eng, log: log, installedTools: installedTools, aiJobs: make(map[string]context.CancelFunc)}
}

// sendMsg 是所有 stream.Send 的唯一入口，保证串行。
func (a *Agent) sendMsg(stream scanv1.ScanService_ConnectClient, msg *scanv1.ScannerMessage) error {
	a.sendMu.Lock()
	defer a.sendMu.Unlock()
	return stream.Send(msg)
}

// getInstalledTools 返回已安装工具的拷贝，避免调用方持有内部 slice。
func (a *Agent) getInstalledTools() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, len(a.installedTools))
	copy(out, a.installedTools)
	return out
}

func (a *Agent) setInstalledTools(tools []string) {
	a.mu.Lock()
	a.installedTools = tools
	a.mu.Unlock()
}

// Run 启动 Agent，断线自动重连，只有以下情况才退出：
//   - 收到 SIGINT/SIGTERM（ctx 被取消）
//   - 被服务端 Kick（管理员删除节点）
func (a *Agent) Run(ctx context.Context) {
	attempt := 0
	for {
		if ctx.Err() != nil {
			a.log.Info("context cancelled, agent exiting")
			return
		}
		if a.kicked {
			a.log.Warn("kicked by server, agent exiting")
			return
		}

		backoff := backoffDuration(attempt)
		if attempt > 0 {
			a.log.Info("reconnecting to server",
				zap.Int("attempt", attempt),
				zap.Duration("backoff", backoff),
				zap.String("server", a.cfg.ServerAddr),
			)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
		}

		connected, err := a.connect(ctx)
		if a.kicked {
			return
		}
		if connected {
			attempt = 0
		}
		attempt++

		if ctx.Err() != nil {
			return
		}
		if err != nil {
			a.log.Warn("connection lost, will reconnect", zap.Error(err))
		}
	}
}

// connect 建立单次连接，返回时表示连接断开。connected 表示是否曾成功注册。
func (a *Agent) connect(ctx context.Context) (connected bool, err error) {
	var creds credentials.TransportCredentials
	if a.cfg.TLS.Enabled {
		if a.cfg.TLS.CAFile != "" {
			c, err := credentials.NewClientTLSFromFile(a.cfg.TLS.CAFile, "")
			if err != nil {
				return false, fmt.Errorf("load ca file: %w", err)
			}
			creds = c
		} else {
			creds = credentials.NewTLS(&tls.Config{
				InsecureSkipVerify: a.cfg.TLS.InsecureSkipVerify,
			})
		}
	} else {
		creds = insecure.NewCredentials()
	}

	conn, err := grpc.NewClient(
		a.cfg.ServerAddr,
		grpc.WithTransportCredentials(creds),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                20 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return false, fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	client := scanv1.NewScanServiceClient(conn)
	stream, err := client.Connect(ctx)
	if err != nil {
		return false, fmt.Errorf("open stream: %w", err)
	}

	// ── Step 1: 发送注册消息 ──────────────────────────────────────────────────
	if err := a.sendMsg(stream, &scanv1.ScannerMessage{
		Payload: &scanv1.ScannerMessage_Register{
			Register: &scanv1.RegisterRequest{
				Name:           a.cfg.Name,
				Token:          a.cfg.Token,
				Version:        "0.1.0",
				Capabilities:   a.cfg.Capabilities,
				MaxTasks:       a.cfg.MaxTasks,
				InstalledTools: a.getInstalledTools(),
			},
		},
	}); err != nil {
		return false, fmt.Errorf("send register: %w", err)
	}

	// ── Step 2: 等待注册确认 ──────────────────────────────────────────────────
	firstMsg, err := stream.Recv()
	if err != nil {
		return false, fmt.Errorf("recv ack: %w", err)
	}
	ack, ok := firstMsg.Payload.(*scanv1.ServerMessage_Ack)
	if !ok || !ack.Ack.Accepted {
		return false, fmt.Errorf("registration rejected: %s", ack.Ack.GetMessage())
	}
	a.nodeID = ack.Ack.NodeId
	a.log.Info("registered with server",
		zap.String("node_id", a.nodeID),
		zap.String("server", a.cfg.ServerAddr),
	)

	// ── Step 3: 启动心跳 goroutine ────────────────────────────────────────────
	connCtx, connCancel := context.WithCancel(ctx)
	defer connCancel()

	go a.sendHeartbeats(connCtx, stream)
	stopSubtaskWorkers := StartSubtaskWorkers(
		connCtx,
		a.cfg.Queue.RedisAddr,
		a.cfg.Queue.RedisPass,
		a.engine,
		a.nodeID,
		a.cfg.Capabilities,
		a.cfg.Queue.NumWorkers,
		a.sendMsgFunc(stream),
		a.log,
	)
	defer stopSubtaskWorkers()

	// ── Step 4: 接收服务端消息主循环 ──────────────────────────────────────────
	for {
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return true, nil
			}
			return true, err
		}
		a.handleServerMessage(connCtx, connCancel, stream, msg)
	}
}

// sendMsgFunc adapts the Agent's serialized stream sender for queue workers.
func (a *Agent) sendMsgFunc(stream scanv1.ScanService_ConnectClient) func(*scanv1.ScannerMessage) error {
	return func(msg *scanv1.ScannerMessage) error {
		return a.sendMsg(stream, msg)
	}
}

func (a *Agent) sendHeartbeats(ctx context.Context, stream scanv1.ScanService_ConnectClient) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			stats := a.engine.Stats()
			err := a.sendMsg(stream, &scanv1.ScannerMessage{
				Payload: &scanv1.ScannerMessage_Heartbeat{
					Heartbeat: &scanv1.Heartbeat{
						NodeId:      a.nodeID,
						ActiveTasks: stats.ActiveTasks,
						CpuPercent:  stats.CPUPercent,
						MemPercent:  stats.MemPercent,
					},
				},
			})
			if err != nil {
				a.log.Warn("heartbeat send failed", zap.Error(err))
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (a *Agent) handleServerMessage(ctx context.Context, connCancel context.CancelFunc, stream scanv1.ScanService_ConnectClient, msg *scanv1.ServerMessage) {
	switch p := msg.Payload.(type) {
	case *scanv1.ServerMessage_Task:
		a.log.Info("received task", zap.String("task_id", p.Task.TaskId))
		go a.runTask(ctx, stream, p.Task)

	case *scanv1.ServerMessage_Cancel:
		a.log.Info("cancel task", zap.String("task_id", p.Cancel.TaskId))
		a.engine.Cancel(p.Cancel.TaskId)
		// Mirror cancellation into the server's node/task log stream. The
		// scanner's local logger is not visible in node logs, which previously
		// made a deleted task look as if its last subtask was still running.
		if err := a.sendMsg(stream, &scanv1.ScannerMessage{Payload: &scanv1.ScannerMessage_Progress{Progress: &scanv1.TaskProgress{
			TaskId: p.Cancel.TaskId,
			Stage:  "_pipeline",
			Log:    fmt.Sprintf("[task] cancellation requested: %s", p.Cancel.Reason),
			Level:  "warn",
		}}}); err != nil {
			a.log.Warn("cancel log send failed", zap.Error(err))
		}
		if strings.HasPrefix(p.Cancel.TaskId, "ai:") {
			a.cancelAIPentest(strings.TrimPrefix(p.Cancel.TaskId, "ai:"))
		}

	case *scanv1.ServerMessage_Kick:
		a.log.Warn("kicked by server (node deleted), will exit", zap.String("reason", p.Kick.Reason))
		a.kicked = true
		connCancel()

	case *scanv1.ServerMessage_Restart:
		a.log.Info("restart requested by server, reconnecting")
		connCancel()

	case *scanv1.ServerMessage_InstallTool:
		a.log.Info("install tool requested", zap.String("tool", p.InstallTool.ToolName), zap.String("cmd", p.InstallTool.InstallCmd), zap.Bool("reinstall", p.InstallTool.Reinstall))
		go a.runInstallTool(ctx, stream, p.InstallTool)
	case *scanv1.ServerMessage_AiPentest:
		go a.runAIPentest(ctx, stream, p.AiPentest)
	}
}

func (a *Agent) sendLog(stream scanv1.ScanService_ConnectClient, taskID, level, msg string) {
	_ = a.sendMsg(stream, &scanv1.ScannerMessage{
		Payload: &scanv1.ScannerMessage_Progress{
			Progress: &scanv1.TaskProgress{
				TaskId: taskID,
				Stage:  "_pipeline",
				Log:    msg,
				Level:  level,
			},
		},
	})
}

// runAIPentest executes Claude Code only for a server-authorized task. The
// server supplies the target scope in the prompt; the node never accepts a
// raw command from the browser.
func (a *Agent) runAIPentest(parent context.Context, stream scanv1.ScanService_ConnectClient, task *scanv1.AIPentestTask) {
	if _, err := exec.LookPath("claude"); err != nil {
		a.sendAIPentestResult(stream, task.TaskId, "failed", "", "未找到 claude，请先在节点工具管理中安装 Claude Code")
		return
	}
	timeout := 30 * time.Minute
	if task.TimeoutSeconds > 0 && task.TimeoutSeconds <= 3600 {
		timeout = time.Duration(task.TimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	a.aiMu.Lock()
	a.aiJobs[task.TaskId] = cancel
	a.aiMu.Unlock()
	defer func() {
		cancel()
		a.aiMu.Lock()
		delete(a.aiJobs, task.TaskId)
		a.aiMu.Unlock()
	}()
	a.sendAIPentestResult(stream, task.TaskId, "running", "Claude Code 已启动，目标范围："+strings.Join(task.Targets, ", "), "")

	var output bytes.Buffer
	cmd := exec.CommandContext(ctx, "claude", "--print", "--dangerously-skip-permissions", task.Prompt)
	if task.ApiKey != "" {
		cmd.Env = append(os.Environ(), "ANTHROPIC_API_KEY="+task.ApiKey)
	}
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	text := output.String()
	if len(text) > 2*1024*1024 {
		text = text[len(text)-2*1024*1024:]
	}
	if ctx.Err() != nil {
		a.sendAIPentestResult(stream, task.TaskId, "cancelled", text, "AI 渗透任务已停止")
		return
	}
	if err != nil {
		a.sendAIPentestResult(stream, task.TaskId, "failed", text, "Claude Code 执行失败: "+err.Error())
		return
	}
	a.sendAIPentestResult(stream, task.TaskId, "done", text, "")
}

func (a *Agent) cancelAIPentest(taskID string) {
	a.aiMu.Lock()
	cancel := a.aiJobs[taskID]
	a.aiMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *Agent) sendAIPentestResult(stream scanv1.ScanService_ConnectClient, taskID, status, output, errMsg string) {
	_ = a.sendMsg(stream, &scanv1.ScannerMessage{Payload: &scanv1.ScannerMessage_AiPentestResult{AiPentestResult: &scanv1.AIPentestResult{TaskId: taskID, NodeId: a.nodeID, Status: status, Output: output, Error: errMsg}}})
}

func (a *Agent) runTask(ctx context.Context, stream scanv1.ScanService_ConnectClient, task *scanv1.ScanTask) {
	a.sendLog(stream, task.TaskId, "info", fmt.Sprintf("收到任务 %s, 目标数: %d, 阶段: %v", shortID(task.TaskId), len(task.Targets), task.Config.GetStages()))
	a.sendStatus(stream, task.TaskId, "running", "")

	resultCh := make(chan *engine.ScanResult, 128)
	progressCh := make(chan *engine.Progress, 32)

	go func() {
		if err := a.engine.Run(ctx, task, resultCh, progressCh); err != nil {
			a.sendLog(stream, task.TaskId, "error", fmt.Sprintf("任务 %s 执行失败: %v", shortID(task.TaskId), err))
			a.sendStatus(stream, task.TaskId, "failed", err.Error())
			return
		}
		a.sendLog(stream, task.TaskId, "info", fmt.Sprintf("任务 %s 执行完成", shortID(task.TaskId)))
		a.sendStatus(stream, task.TaskId, "done", "")
	}()

	// 转发结果和进度到服务端，两个 channel 都关闭才退出
	resultDone, progressDone := false, false
	for !resultDone || !progressDone {
		select {
		case r, ok := <-resultCh:
			if !ok {
				resultDone = true
				resultCh = nil
				continue
			}
			_ = a.sendMsg(stream, &scanv1.ScannerMessage{
				Payload: &scanv1.ScannerMessage_Result{
					Result: &scanv1.TaskResult{
						TaskId:     task.TaskId,
						NodeId:     a.nodeID,
						ResultType: r.Type,
						Data:       r.Data,
					},
				},
			})
		case p, ok := <-progressCh:
			if !ok {
				progressDone = true
				progressCh = nil
				continue
			}
			_ = a.sendMsg(stream, &scanv1.ScannerMessage{
				Payload: &scanv1.ScannerMessage_Progress{
					Progress: &scanv1.TaskProgress{
						TaskId:  task.TaskId,
						Stage:   p.Stage,
						Percent: p.Percent,
						Message: p.Message,
						Log:     p.Log,
						Level:   p.Level,
					},
				},
			})
		case <-ctx.Done():
			return
		}
	}
}

func (a *Agent) sendStatus(stream scanv1.ScanService_ConnectClient, taskID, status, errMsg string) {
	_ = a.sendMsg(stream, &scanv1.ScannerMessage{
		Payload: &scanv1.ScannerMessage_Status{
			Status: &scanv1.TaskStatusUpdate{
				TaskId: taskID,
				Status: status,
				Error:  errMsg,
			},
		},
	})
}

func (a *Agent) runInstallTool(ctx context.Context, stream scanv1.ScanService_ConnectClient, req *scanv1.InstallTool) {
	toolName := req.ToolName
	installCmd := req.InstallCmd

	if req.Uninstall {
		a.runUninstallTool(ctx, stream, toolName)
		return
	}

	if req.Reinstall {
		a.sendNodeLog(stream, "info", fmt.Sprintf("[安装工具] 重新安装 %s，先卸载旧版本…", toolName))
		if oldPath, err := exec.LookPath(toolName); err == nil {
			a.sendNodeLog(stream, "info", fmt.Sprintf("[安装工具/%s] 删除旧版本: %s", toolName, oldPath))
			if err := exec.CommandContext(ctx, "rm", "-f", oldPath).Run(); err != nil {
				a.sendNodeLog(stream, "warn", fmt.Sprintf("[安装工具/%s] 删除旧版本失败: %v，继续安装…", toolName, err))
			} else {
				a.sendNodeLog(stream, "info", fmt.Sprintf("[安装工具/%s] 旧版本已删除", toolName))
			}
		} else {
			a.sendNodeLog(stream, "info", fmt.Sprintf("[安装工具/%s] 未找到已安装的版本，直接安装", toolName))
		}
	}

	a.sendNodeLog(stream, "info", fmt.Sprintf("[安装工具] 开始安装 %s: %s", toolName, installCmd))

	cmd := exec.CommandContext(ctx, "sh", "-c", installCmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		a.sendInstallResult(stream, toolName, false, fmt.Sprintf("创建 stdout 管道失败: %v", err))
		return
	}
	// go install 等工具的输出走 stderr，必须单独捕获后合并读取。
	stderr, err := cmd.StderrPipe()
	if err != nil {
		a.sendInstallResult(stream, toolName, false, fmt.Sprintf("创建 stderr 管道失败: %v", err))
		return
	}

	if err := cmd.Start(); err != nil {
		a.sendInstallResult(stream, toolName, false, fmt.Sprintf("启动安装命令失败: %v", err))
		return
	}

	var wg sync.WaitGroup
	scanPipe := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for sc.Scan() {
			a.sendNodeLog(stream, "info", fmt.Sprintf("[安装工具/%s] %s", toolName, sc.Text()))
		}
	}
	wg.Add(2)
	go scanPipe(stdout)
	go scanPipe(stderr)
	wg.Wait()

	cmdErr := cmd.Wait()

	tools := a.redetectTools()
	a.setInstalledTools(tools)

	toolFound := false
	for _, t := range tools {
		if t == toolName {
			toolFound = true
			break
		}
	}

	if !toolFound {
		errMsg := fmt.Sprintf("安装 %s 失败", toolName)
		if cmdErr != nil {
			errMsg = fmt.Sprintf("安装 %s 失败: %v", toolName, cmdErr)
		}
		a.sendNodeLog(stream, "error", "[安装工具] "+errMsg)
		a.sendInstallResult(stream, toolName, false, errMsg)
		return
	}

	a.sendNodeLog(stream, "info", fmt.Sprintf("[安装工具] %s 安装完成, 当前已安装工具: %v", toolName, tools))
	a.sendInstallResult(stream, toolName, true, "")
}

func (a *Agent) runUninstallTool(ctx context.Context, stream scanv1.ScanService_ConnectClient, toolName string) {
	a.sendNodeLog(stream, "info", fmt.Sprintf("[安装工具] 卸载 %s…", toolName))
	path, err := exec.LookPath(toolName)
	if err != nil {
		a.sendNodeLog(stream, "warn", fmt.Sprintf("[安装工具/%s] 未找到已安装的版本，视为已卸载", toolName))
		a.setInstalledTools(a.redetectTools())
		a.sendInstallResult(stream, toolName, true, "")
		return
	}
	a.sendNodeLog(stream, "info", fmt.Sprintf("[安装工具/%s] 删除二进制: %s", toolName, path))
	if err := exec.CommandContext(ctx, "rm", "-f", path).Run(); err != nil {
		errMsg := fmt.Sprintf("删除 %s 失败: %v", toolName, err)
		a.sendNodeLog(stream, "error", "[安装工具] "+errMsg)
		a.sendInstallResult(stream, toolName, false, errMsg)
		return
	}
	tools := a.redetectTools()
	a.setInstalledTools(tools)
	a.sendNodeLog(stream, "info", fmt.Sprintf("[安装工具] %s 已卸载, 当前已安装工具: %v", toolName, tools))
	a.sendInstallResult(stream, toolName, true, "")
}

func (a *Agent) sendNodeLog(stream scanv1.ScanService_ConnectClient, level, msg string) {
	_ = a.sendMsg(stream, &scanv1.ScannerMessage{
		Payload: &scanv1.ScannerMessage_Progress{
			Progress: &scanv1.TaskProgress{
				NodeId: a.nodeID,
				Log:    msg,
				Level:  level,
			},
		},
	})
}

func (a *Agent) sendInstallResult(stream scanv1.ScanService_ConnectClient, toolName string, success bool, errMsg string) {
	_ = a.sendMsg(stream, &scanv1.ScannerMessage{
		Payload: &scanv1.ScannerMessage_InstallResult{
			InstallResult: &scanv1.InstallResult{
				ToolName:       toolName,
				Success:        success,
				ErrorMsg:       errMsg,
				InstalledTools: a.getInstalledTools(),
			},
		},
	})
}

var extraToolPaths = []string{
	"/root/.local/bin",
	"/usr/local/bin",
	"/home/ubuntu/.local/bin",
}

func (a *Agent) redetectTools() []string {
	var installed []string
	for _, name := range tooldef.Names() {
		if found := a.detectTool(name); found {
			installed = append(installed, name)
		}
	}
	a.log.Info("re-detected installed tools", zap.Strings("tools", installed))
	return installed
}

func (a *Agent) detectTool(name string) bool {
	if _, err := exec.LookPath(name); err == nil {
		return true
	}
	for _, dir := range extraToolPaths {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	// 检查 pipx venv（软链接丢失时 PATH 找不到，但 venv 里实际存在）
	pipxVenv := filepath.Join(os.Getenv("HOME"), ".local", "pipx", "venvs", name, "bin", name)
	if _, err := os.Stat(pipxVenv); err == nil {
		return true
	}
	return false
}

// backoffDuration 指数退避：1s 2s 4s 8s … 最大 30s
func backoffDuration(attempt int) time.Duration {
	if attempt == 0 {
		return 0
	}
	d := float64(initialBackoff) * math.Pow(2, float64(attempt-1))
	if d > float64(maxBackoff) {
		d = float64(maxBackoff)
	}
	return time.Duration(d)
}
