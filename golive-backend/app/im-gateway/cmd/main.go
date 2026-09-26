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

	"github.com/qingwenwen777/golive/app/im-gateway/internal/auth"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/config"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/hub"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/moderation"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/producer"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/profile"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/pubsub"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/rooms"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/server"
	"github.com/qingwenwen777/golive/pkg/chatfilter"
	"github.com/qingwenwen777/golive/pkg/chatlimit"
	"github.com/qingwenwen777/golive/pkg/logger"
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

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		DB:       cfg.Redis.DB,
		Password: cfg.Redis.Password,
	})
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		cancelPing()
		log.Fatal("redis ping", zap.Error(err))
	}
	cancelPing()

	hubCtx, cancelHub := context.WithCancel(context.Background())
	defer cancelHub()

	broker := pubsub.NewRedis(rdb)
	defer broker.Close()
	h := hub.New(hubCtx, broker, cfg.Room.ViewerPushInterval)
	jwtKeys, err := cfg.JWT.KeySet()
	if err != nil {
		log.Fatal("jwt config", zap.Error(err))
	}
	verifier := auth.NewHMACVerifierWithKeySet(jwtKeys)

	var p producer.Producer
	if cfg.Kafka.Enabled {
		p, err = producer.NewFranz(cfg.Kafka.Brokers, cfg.Kafka.ChatTopic)
		if err != nil {
			log.Fatal("kafka producer", zap.Error(err))
		}
	} else {
		p = producer.NewNoop()
	}
	defer p.Close()

	trustedProxies, err := cfg.WS.TrustedProxyNets()
	if err != nil {
		log.Fatal("ws config", zap.Error(err))
	}
	wsCfg := server.WSConfig{
		ReadLimitBytes:   cfg.WS.ReadLimitBytes,
		ReadIdleTimeout:  cfg.WS.ReadIdleTimeout,
		WriteDeadline:    cfg.WS.WriteDeadline,
		SendBuffer:       cfg.WS.SendBuffer,
		PongWait:         cfg.WS.PongWait,
		MaxMessageRate:   cfg.WS.MaxMessageRate,
		AllowedOrigins:   cfg.WS.AllowedOrigins,
		MaxConnsPerUser:  cfg.WS.MaxConnsPerUser,
		MaxConnsPerIP:    cfg.WS.MaxConnsPerIP,
		TrustedProxies:   trustedProxies,
		RequireKnownRoom: cfg.Room.RequireKnown,
	}
	words, err := chatfilter.LoadWords(cfg.Filter.SensitivePath)
	if err != nil {
		// Keep chat available, but make the gap loud: without the list only
		// the admin blocked-word set applies.
		log.Error("load sensitive words; chat masking disabled", zap.String("path", cfg.Filter.SensitivePath), zap.Error(err))
	}
	deps := server.Deps{
		Hub:         h,
		Producer:    p,
		Moderation:  moderation.NewRedisChecker(rdb),
		Filter:      chatfilter.New(words, chatfilter.WithMask(cfg.Filter.Mask), chatfilter.WithSkipChars(chatfilter.DefaultSkipChars)),
		ChatLimiter: chatlimit.New(rdb, "rl:imgw:chat:", cfg.ChatRateLimit.PerUserPerSec, cfg.ChatRateLimit.Window()),
		Rooms:       rooms.NewRedisDirectory(rdb, rooms.Config{RoomServiceURL: cfg.Room.ServiceURL}),
		Profiles: profile.NewHTTPResolver(profile.Config{
			UserServiceURL: cfg.Profile.UserServiceURL,
			ChatServiceURL: cfg.Profile.ChatServiceURL,
			TTL:            cfg.Profile.TTL,
		}),
	}
	wsH := server.NewWSHandler(deps, verifier, wsCfg, cfg.Room.WelcomeText)
	mux := server.NewMux(wsH, h)

	httpSrv := &http.Server{Addr: cfg.Service.HTTPAddr, Handler: mux}

	go func() {
		if err := http.ListenAndServe(cfg.Service.PprofAddr, nil); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn("pprof exit", zap.Error(err))
		}
	}()

	go func() {
		log.Info("im-gateway listening", zap.String("addr", cfg.Service.HTTPAddr))
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("http exit", zap.Error(err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}
