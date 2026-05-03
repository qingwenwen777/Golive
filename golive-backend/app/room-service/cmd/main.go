package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis/v9"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/config"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/app/room-service/internal/server"
	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/logger"
	"github.com/qingwenwen777/golive/pkg/obs"
)

func main() {
	cfgPath := flag.String("config", "", "path to config.yaml")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		panic(err)
	}
	logger.Init(cfg.Service.Name, cfg.Service.LogLevel)
	defer logger.Sync()
	log := logger.L()

	shutdownTracer := obs.InitTracing(cfg.Service.Name)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracer(ctx)
	}()

	db, err := openMySQL(cfg.MySQL)
	if err != nil {
		log.Fatal("open mysql", zap.Error(err))
	}
	rdb, err := openRedis(cfg.Redis)
	if err != nil {
		log.Fatal("open redis", zap.Error(err))
	}

	roomRepo := repo.NewRoomRepo(db)
	if err := roomRepo.AutoMigrate(); err != nil {
		log.Fatal("automigrate", zap.Error(err))
	}
	appointmentRepo := repo.NewAppointmentRepo(db)
	if err := appointmentRepo.AutoMigrate(); err != nil {
		log.Fatal("appointment automigrate", zap.Error(err))
	}
	moderationRepo := repo.NewModerationRepo(db, rdb)
	if err := moderationRepo.AutoMigrate(); err != nil {
		log.Fatal("moderation automigrate", zap.Error(err))
	}
	postRepo := repo.NewPostRepo(db)
	if err := postRepo.AutoMigrate(); err != nil {
		log.Fatal("post automigrate", zap.Error(err))
	}
	if n, err := roomRepo.FixUUIDChannels(context.Background()); err != nil {
		log.Warn("fix uuid channels", zap.Error(err))
	} else if n > 0 {
		log.Info("fixed legacy uuid channels", zap.Int64("rows", n))
	}
	socialRepo := repo.NewSocialRepo(rdb)
	liveRepo := repo.NewLiveRepo(rdb)

	roomSvc := service.NewRoomService(roomRepo, cfg.Live.FlvBase, socialRepo)
	socialSvc := service.NewSocialService(socialRepo, roomRepo)
	liveSvc := service.NewLiveService(roomRepo, liveRepo, cfg.Live.StreamKeySecret, cfg.Live.StreamKeyTTL, cfg.Live.FlvBase)
	liveSvc.SetAppointmentRepo(appointmentRepo)
	liveSvc.SetModerationRepo(moderationRepo)
	appointmentSvc := service.NewAppointmentService(appointmentRepo, roomRepo, socialRepo, liveSvc)
	moderationSvc := service.NewModerationService(moderationRepo, roomRepo, socialRepo)
	permission, err := service.NewUserPermissionClient(cfg.Users.GRPCAddr, cfg.Users.ServiceURL)
	if err != nil {
		log.Fatal("new user permission client", zap.Error(err))
	}
	defer permission.Close()
	postSvc := service.NewPostService(postRepo, roomRepo, socialRepo, permission)
	if activeRooms, err := roomRepo.ActiveRooms(context.Background()); err != nil {
		log.Warn("load active rooms for moderation cache", zap.Error(err))
	} else if err := moderationRepo.SyncActiveRooms(context.Background(), activeRooms); err != nil {
		log.Warn("sync moderation cache", zap.Error(err))
	}
	if err := moderationRepo.SyncActiveMutes(context.Background(), time.Now()); err != nil {
		log.Warn("sync active mutes", zap.Error(err))
	}

	r := server.NewRouter(server.Deps{
		JWTSecret:      cfg.JWT.Secret,
		Room:           roomSvc,
		Social:         socialSvc,
		Posts:          postSvc,
		Live:           liveSvc,
		Appointments:   appointmentSvc,
		Moderation:     moderationSvc,
		Permission:     permission,
		CoverDir:       cfg.Upload.CoverDir,
		CoverPublicURL: cfg.Upload.CoverPublicURL,
		PostImageDir:   cfg.Upload.PostImageDir,
		PostPublicURL:  cfg.Upload.PostPublicURL,
	})
	httpSrv := &http.Server{Addr: cfg.Service.HTTPAddr, Handler: r}
	schedulerCtx, stopScheduler := context.WithCancel(context.Background())
	defer stopScheduler()

	go func() {
		if err := http.ListenAndServe(cfg.Service.PprofAddr, nil); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn("pprof exit", zap.Error(err))
		}
	}()

	go func() {
		log.Info("room-service listening", zap.String("addr", cfg.Service.HTTPAddr))
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("http exit", zap.Error(err))
		}
	}()
	go appointmentSvc.RunScheduler(schedulerCtx)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Info("shutting down")
	stopScheduler()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}

func openMySQL(c config.MySQLCfg) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(c.DSN), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(c.MaxOpen)
	sqlDB.SetMaxIdleConns(c.MaxIdle)
	return db, nil
}

func openRedis(c config.RedisCfg) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{Addr: c.Addr, DB: c.DB, Password: c.Password})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return rdb, nil
}
