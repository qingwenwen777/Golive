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

	"github.com/qingwenwen777/golive/app/gift-service/internal/config"
	"github.com/qingwenwen777/golive/app/gift-service/internal/handler"
	"github.com/qingwenwen777/golive/app/gift-service/internal/producer"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
	"github.com/qingwenwen777/golive/app/gift-service/internal/seed"
	"github.com/qingwenwen777/golive/app/gift-service/internal/server"
	"github.com/qingwenwen777/golive/app/gift-service/internal/service"
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
	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, DB: cfg.Redis.DB, Password: cfg.Redis.Password})
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		cancelPing()
		log.Fatal("redis ping", zap.Error(err))
	}
	cancelPing()

	giftRepo := repo.NewGiftRepo(db)
	if err := giftRepo.AutoMigrate(); err != nil {
		log.Fatal("migrate gifts", zap.Error(err))
	}
	orderRepo := repo.NewOrderRepo(db)
	if err := orderRepo.AutoMigrate(); err != nil {
		log.Fatal("migrate orders", zap.Error(err))
	}
	outboxRepo := repo.NewOutboxRepo(db)

	if err := seed.SeedGifts(context.Background(), giftRepo); err != nil {
		log.Warn("seed gifts", zap.Error(err))
	}

	idem := service.NewIdemCache(rdb, cfg.Idempotency.TTL)
	giftSvc := service.NewGiftService(giftRepo, orderRepo)
	scSvc := service.NewSuperChatService(orderRepo, rdb)
	betSvc := service.NewBetService(orderRepo)
	adminRepo := repo.NewAdminRepo(db)
	adminSvc := service.NewAdminService(adminRepo, orderRepo)

	var prod producer.Producer
	if cfg.Kafka.Enabled {
		prod, err = producer.NewFranz(cfg.Kafka.Brokers, cfg.Kafka.GiftTopic)
		if err != nil {
			log.Fatal("kafka producer", zap.Error(err))
		}
	} else {
		prod = producer.NewRedisFanout(rdb)
	}
	defer prod.Close()

	outboxSvc := service.NewOutboxService(outboxRepo, prod, service.OutboxConfig{
		PollInterval: cfg.Outbox.PollInterval,
		BatchSize:    cfg.Outbox.BatchSize,
		MaxRetries:   cfg.Outbox.MaxRetries,
		BaseBackoff:  cfg.Outbox.BaseBackoff,
	})

	giftH := handler.NewGiftHandler(giftSvc, idem)
	scH := handler.NewSuperChatHandler(scSvc, idem)
	betH := handler.NewBetHandler(betSvc)
	adminH := handler.NewAdminHandler(adminSvc)
	jwtKeys, err := cfg.JWT.KeySet()
	if err != nil {
		log.Fatal("jwt config", zap.Error(err))
	}

	r := server.NewRouter(server.Deps{
		JWTSecret: cfg.JWT.Secret,
		JWTKeys:   jwtKeys,
		Gift:      giftH,
		SuperChat: scH,
		Bet:       betH,
		Admin:     adminH,
	})
	httpSrv := &http.Server{Addr: cfg.Service.HTTPAddr, Handler: r}

	outboxCtx, cancelOutbox := context.WithCancel(context.Background())
	defer cancelOutbox()
	go outboxSvc.Run(outboxCtx)

	go func() {
		if err := http.ListenAndServe(cfg.Service.PprofAddr, nil); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn("pprof exit", zap.Error(err))
		}
	}()

	go func() {
		log.Info("gift-service listening", zap.String("addr", cfg.Service.HTTPAddr))
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("http exit", zap.Error(err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Info("shutting down")
	cancelOutbox()
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
