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

	"github.com/qingwenwen777/golive/app/chat-service/internal/config"
	"github.com/qingwenwen777/golive/app/chat-service/internal/consumer"
	"github.com/qingwenwen777/golive/app/chat-service/internal/filter"
	"github.com/qingwenwen777/golive/app/chat-service/internal/handler"
	"github.com/qingwenwen777/golive/app/chat-service/internal/ratelimit"
	"github.com/qingwenwen777/golive/app/chat-service/internal/redissub"
	"github.com/qingwenwen777/golive/app/chat-service/internal/repo"
	"github.com/qingwenwen777/golive/app/chat-service/internal/server"
	"github.com/qingwenwen777/golive/app/chat-service/internal/service"
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
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.Redis.Addr, DB: cfg.Redis.DB, Password: cfg.Redis.Password,
	})
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		cancelPing()
		log.Fatal("redis ping", zap.Error(err))
	}
	cancelPing()

	danmuRepo := repo.NewDanmuRepo(db, cfg.MySQL.Shards)
	if err := danmuRepo.AutoMigrate(); err != nil {
		log.Fatal("migrate shards", zap.Error(err))
	}

	words, err := filter.LoadWords(cfg.Filter.SensitivePath)
	if err != nil {
		log.Warn("load sensitive words", zap.Error(err))
	}
	f := filter.New(words, filter.WithMask(cfg.Filter.Mask), filter.WithSkipChars(" .*-_"))

	limiter := ratelimit.New(rdb, cfg.RateLimit.PerUserPerSec, cfg.RateLimit.Window())
	pub := repo.NewPublisher(rdb)
	svc := service.New(f, limiter, danmuRepo, pub)

	cons, err := consumer.New(consumer.Config{
		Brokers: cfg.Kafka.Brokers,
		Topic:   cfg.Kafka.Topic,
		Group:   cfg.Kafka.Group,
		Workers: cfg.Kafka.Workers,
	}, svc)
	if err != nil {
		log.Fatal("kafka client", zap.Error(err))
	}

	histH := handler.NewHistoryHandler(svc, cfg.Room.HistoryDefaultLimit, cfg.Room.HistoryMaxLimit)
	r := server.NewRouter(histH)
	httpSrv := &http.Server{Addr: cfg.Service.HTTPAddr, Handler: r}

	go func() {
		if err := http.ListenAndServe(cfg.Service.PprofAddr, nil); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn("pprof exit", zap.Error(err))
		}
	}()

	consumerCtx, cancelConsumer := context.WithCancel(context.Background())
	defer cancelConsumer()
	go func() {
		log.Info("kafka consumer started",
			zap.Strings("brokers", cfg.Kafka.Brokers),
			zap.String("topic", cfg.Kafka.Topic),
			zap.Int("workers", cfg.Kafka.Workers))
		if err := cons.Run(consumerCtx); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal("consumer exit", zap.Error(err))
		}
	}()

	// Redis pub/sub subscriber for live-chat persistence. This is what
	// actually writes danmus to MySQL in the current deployment, because
	// im-gateway runs with kafka.enabled=false and broadcasts directly to
	// "room:<id>" Redis channels. Unlike a Fatal on the kafka consumer,
	// failures here are logged and retried — the live broadcast path keeps
	// working even if persistence is briefly unavailable.
	subscriber := redissub.New(rdb, danmuRepo)
	go func() {
		for {
			err := subscriber.Run(consumerCtx)
			if err == nil || errors.Is(err, context.Canceled) {
				return
			}
			log.Warn("redis chat subscriber exited; retrying", zap.Error(err))
			select {
			case <-consumerCtx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}()

	go func() {
		log.Info("chat-service listening", zap.String("addr", cfg.Service.HTTPAddr))
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("http exit", zap.Error(err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Info("shutting down")
	cancelConsumer()
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
