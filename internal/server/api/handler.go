package api

import (
	"context"
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	grpcsvr "github.com/yourname/nscan/internal/server/grpc"
	"github.com/yourname/nscan/internal/server/hub"
	"github.com/yourname/nscan/internal/server/nodelog"
	"github.com/yourname/nscan/internal/server/notify"
	"github.com/yourname/nscan/internal/server/repositories"
	"github.com/yourname/nscan/internal/server/scheduler"
	"github.com/yourname/nscan/internal/server/taskprogress"
	"github.com/yourname/nscan/internal/server/tokenstore"
	"go.uber.org/zap"
)

type Handler struct {
	projects     *repositories.ProjectRepo
	tasks        *repositories.TaskRepo
	assets       *repositories.AssetRepo
	scanTpl      *repositories.ScanTemplateRepo
	plugins      *repositories.PluginRepo
	scheduled    *repositories.ScheduledRepo
	notify       *repositories.NotifyRepo
	settings     *repositories.SettingsRepo
	blacklist    *repositories.BlacklistRepo
	poc          *repositories.PocRepo
	fingerprint  *repositories.FingerprintRepo
	dict         *repositories.DictRepo
	sensitive    *repositories.SensitiveRuleRepo
	users        *repositories.UserRepo
	notifier     *notify.Notifier
	sched        *scheduler.Scheduler
	nm           *grpcsvr.NodeManager
	hub          *hub.Hub
	nodeLog      *nodelog.Store
	taskProg     *taskprogress.Store
	log          *zap.Logger
	tokenStore   *tokenstore.Store
	grpcAddr     string
	jwtSecret    string
	scannerImage string
	aiJobs       map[string]context.CancelFunc
	aiJobsMu     sync.Mutex
}

func NewHandler(
	projects *repositories.ProjectRepo,
	tasks *repositories.TaskRepo,
	assets *repositories.AssetRepo,
	scanTpl *repositories.ScanTemplateRepo,
	plugins *repositories.PluginRepo,
	scheduled *repositories.ScheduledRepo,
	notifyRepo *repositories.NotifyRepo,
	settingsRepo *repositories.SettingsRepo,
	blacklistRepo *repositories.BlacklistRepo,
	pocRepo *repositories.PocRepo,
	fingerprintRepo *repositories.FingerprintRepo,
	dictRepo *repositories.DictRepo,
	sensitiveRepo *repositories.SensitiveRuleRepo,
	userRepo *repositories.UserRepo,
	notifier *notify.Notifier,
	sched *scheduler.Scheduler,
	nm *grpcsvr.NodeManager,
	h *hub.Hub,
	nodeLog *nodelog.Store,
	tp *taskprogress.Store,
	log *zap.Logger,
	tokenStore *tokenstore.Store,
	grpcAddr string,
	jwtSecret string,
	scannerImage string,
) *Handler {
	return &Handler{
		projects:     projects,
		tasks:        tasks,
		assets:       assets,
		scanTpl:      scanTpl,
		plugins:      plugins,
		scheduled:    scheduled,
		notify:       notifyRepo,
		settings:     settingsRepo,
		blacklist:    blacklistRepo,
		poc:          pocRepo,
		fingerprint:  fingerprintRepo,
		dict:         dictRepo,
		sensitive:    sensitiveRepo,
		users:        userRepo,
		notifier:     notifier,
		sched:        sched,
		nm:           nm,
		hub:          h,
		nodeLog:      nodeLog,
		taskProg:     tp,
		log:          log,
		tokenStore:   tokenStore,
		grpcAddr:     grpcAddr,
		jwtSecret:    jwtSecret,
		scannerImage: scannerImage,
		aiJobs:       make(map[string]context.CancelFunc),
	}
}

func errResp(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"error": msg})
}

// batchDeleteLimit 是批量删除接口允许的最大 ID 数量。
// 超过时直接拒绝，避免前端误传大列表导致长事务/WS 广播风暴。
const batchDeleteLimit = 500

// checkBatchLimit 若 ids 超过上限，写入 400 并返回 true 表示已处理，调用方应立即 return。
func checkBatchLimit(c *gin.Context, ids []string) bool {
	if len(ids) > batchDeleteLimit {
		errResp(c, http.StatusBadRequest, "ids too many, max 500 per batch")
		return true
	}
	return false
}

func paginate(c *gin.Context) (limit, skip int64) {
	limit = 20
	skip = 0
	if v, err := strconv.ParseInt(c.Query("limit"), 10, 64); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	if v, err := strconv.ParseInt(c.Query("skip"), 10, 64); err == nil && v >= 0 {
		skip = v
	}
	return
}

func listResp(c *gin.Context, data any, total int64) {
	c.JSON(http.StatusOK, gin.H{"data": data, "total": total})
}
