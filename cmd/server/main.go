package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	nsserver "github.com/yourname/nscan/internal/server"
	nsapi "github.com/yourname/nscan/internal/server/api"
	svcfg "github.com/yourname/nscan/internal/server/config"
	"github.com/yourname/nscan/internal/server/cronjob"
	grpcsvr "github.com/yourname/nscan/internal/server/grpc"
	"github.com/yourname/nscan/internal/server/hub"
	_ "github.com/yourname/nscan/internal/server/metrics" // register prometheus metrics
	"github.com/yourname/nscan/internal/server/nodelog"
	"github.com/yourname/nscan/internal/server/notify"
	"github.com/yourname/nscan/internal/server/repositories"
	"github.com/yourname/nscan/internal/server/scheduler"
	"github.com/yourname/nscan/internal/server/taskprogress"
	"github.com/yourname/nscan/internal/server/tokenstore"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
	"google.golang.org/grpc/credentials"
)

func main() {
	cfgPath := flag.String("config", "configs/server.yaml", "config file path")
	flag.Parse()

	cfg, err := svcfg.Load(*cfgPath)
	if err != nil {
		panic("load config: " + err.Error())
	}

	log := buildLogger(cfg.Log.Level, cfg.Log.Format)
	defer log.Sync()

	// ── MongoDB ───────────────────────────────────────────────────────────────
	mongoClient, err := mongo.Connect(
		context.Background(),
		options.Client().ApplyURI(cfg.MongoDB.URI),
	)
	if err != nil {
		log.Fatal("mongodb connect", zap.Error(err))
	}
	defer mongoClient.Disconnect(context.Background())
	db := mongoClient.Database(cfg.MongoDB.Database)

	// ── Redis ──────────────────────────────────────────────────────────────────
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatal("redis connect", zap.Error(err))
	}
	defer rdb.Close()

	// ── 组件组装 ──────────────────────────────────────────────────────────────
	h := hub.New(rdb, log)
	nm := grpcsvr.NewNodeManager(log)
	nodeLog := nodelog.New(rdb)
	tp := taskprogress.New(rdb)

	repositories.EnsureUserIndexes(context.Background(), db)

	projectRepo := repositories.NewProjectRepo(db)
	taskRepo := repositories.NewTaskRepo(db)
	assetRepo := repositories.NewAssetRepo(db)
	scanTplRepo := repositories.NewScanTemplateRepo(db)
	pluginRepo := repositories.NewPluginRepo(db)
	scheduledRepo := repositories.NewScheduledRepo(db)
	notifyRepo := repositories.NewNotifyRepo(db)
	settingsRepo := repositories.NewSettingsRepo(db)
	blacklistRepo := repositories.NewBlacklistRepo(db)
	pocRepo := repositories.NewPocRepo(db)
	fingerprintRepo := repositories.NewFingerprintRepo(db)
	dictRepo := repositories.NewDictRepo(db)
	sensitiveRepo := repositories.NewSensitiveRuleRepo(db)
	userRepo := repositories.NewUserRepo(db)
	notifier := notify.New(notifyRepo, log)

	// 注册内置插件 & 种子数据
	nsserver.SeedBuiltinPlugins(context.Background(), pluginRepo, log)
	nsserver.SeedFingerprints(context.Background(), fingerprintRepo, log)
	nsserver.SeedDicts(context.Background(), dictRepo, log)
	nsserver.SeedSensitiveRules(context.Background(), sensitiveRepo, log)
	nsserver.SeedAdminUser(context.Background(), userRepo, cfg.Server.AdminUser, cfg.Server.AdminPass, log)

	ts := tokenstore.New(settingsRepo, cfg.Server.AuthToken)
	ts.Init(context.Background())

	sched := scheduler.New(db, rdb, nm, h, tp, notifier, log, blacklistRepo, settingsRepo)
	if cfg.Queue.Mode == "redis" {
		sched.EnableQueueMode()
	}
	grpcServer := grpcsvr.NewServer(ts, nm, sched, nodeLog, h, log)
	grpcServer.SetNodeOfflineHook(sched.OnNodeOffline)
	cronRunner := cronjob.New(scheduledRepo, sched, log)
	scannerImage := cfg.Server.ScannerImage
	if scannerImage == "" {
		scannerImage = "nscan-scanner:latest"
	}
	apiHandler := nsapi.NewHandler(projectRepo, taskRepo, assetRepo, scanTplRepo, pluginRepo, scheduledRepo, notifyRepo, settingsRepo, blacklistRepo, pocRepo, fingerprintRepo, dictRepo, sensitiveRepo, userRepo, notifier, sched, nm, h, nodeLog, tp, log, ts, cfg.Server.GRPCAddr, cfg.Server.JWTSecret, scannerImage)
	sched.SetAIResultHandler(apiHandler.OnAIPentestResult)

	// ── 启动 gRPC ─────────────────────────────────────────────────────────────
	var grpcCreds credentials.TransportCredentials
	if cfg.Server.TLS.Enabled {
		creds, err := credentials.NewServerTLSFromFile(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
		if err != nil {
			log.Fatal("load tls cert", zap.Error(err))
		}
		grpcCreds = creds
	}

	go func() {
		if err := grpcServer.ListenAndServe(cfg.Server.GRPCAddr, grpcCreds); err != nil {
			log.Fatal("grpc server error", zap.Error(err))
		}
	}()

	// ── 启动调度器 ────────────────────────────────────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go sched.Run(ctx)
	go cronRunner.Run(ctx)

	// ── HTTP API ──────────────────────────────────────────────────────────────
	r := gin.New()
	r.Use(gin.Recovery())
	apiHandler.Register(r)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	httpSrv := &http.Server{
		Addr:    cfg.Server.HTTPAddr,
		Handler: r,
	}
	go func() {
		log.Info("HTTP server listening", zap.String("addr", cfg.Server.HTTPAddr))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("http server error", zap.Error(err))
		}
	}()

	log.Info("nscan server started",
		zap.String("http", cfg.Server.HTTPAddr),
		zap.String("grpc", cfg.Server.GRPCAddr),
	)

	<-ctx.Done()
	log.Info("shutting down...")

	grpcServer.Stop()
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutCtx)

	log.Info("server stopped")
}

func buildLogger(level, format string) *zap.Logger {
	var cfg zap.Config
	if format == "console" {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}
	_ = cfg.Level.UnmarshalText([]byte(level))
	logger, _ := cfg.Build()
	return logger
}
