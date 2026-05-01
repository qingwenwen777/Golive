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
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/user-service/internal/config"
	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"github.com/qingwenwen777/golive/app/user-service/internal/server"
	"github.com/qingwenwen777/golive/app/user-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/logger"
	"github.com/qingwenwen777/golive/pkg/obs"
)

func main() {
	cfgPath := flag.String("config", "", "path to config.yaml (default: ./configs/config.yaml)")
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

	userRepo := repo.NewUserRepo(db)
	if err := userRepo.AutoMigrate(); err != nil {
		log.Fatal("auto migrate", zap.Error(err))
	}
	if cfg.Bootstrap.DemoUser.Enabled {
		if err := seedDemoUser(context.Background(), userRepo, cfg.Bootstrap.DemoUser); err != nil {
			log.Warn("seed demo user", zap.Error(err))
		}
	}
	if cfg.Bootstrap.Admin.Enabled {
		if err := seedAdminUser(context.Background(), userRepo, cfg.Bootstrap.Admin); err != nil {
			log.Warn("seed admin user", zap.Error(err))
		}
	}

	tokenRepo := repo.NewTokenRepo(rdb)
	auth := service.NewAuthService(userRepo, tokenRepo, service.Options{
		JWTSecret:  cfg.JWT.Secret,
		AccessTTL:  cfg.JWT.AccessTTL,
		RefreshTTL: cfg.JWT.RefreshTTL,
	})

	r := server.NewRouter(server.Deps{
		Auth:            auth,
		Users:           userRepo,
		AvatarDir:       cfg.Upload.AvatarDir,
		AvatarPublicURL: cfg.Upload.AvatarPublicURL,
	})
	httpSrv := &http.Server{Addr: cfg.Service.HTTPAddr, Handler: r}

	// pprof on a side port.
	go func() {
		if err := http.ListenAndServe(cfg.Service.PprofAddr, nil); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn("pprof exit", zap.Error(err))
		}
	}()

	go func() {
		log.Info("user-service listening", zap.String("addr", cfg.Service.HTTPAddr))
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
	rdb := redis.NewClient(&redis.Options{
		Addr:     c.Addr,
		DB:       c.DB,
		Password: c.Password,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return rdb, nil
}

// seedDemoUser inserts the demo account if it doesn't exist. Frontend E2E
// expects username=demo / password=demo to log in successfully.
func seedDemoUser(ctx context.Context, ur *repo.UserRepo, c config.DemoUserCfg) error {
	if _, err := ur.FindByUsername(ctx, c.Username); err == nil {
		return nil // already exists
	} else if !errors.Is(err, repo.ErrUserNotFound) {
		return err
	}
	hash, err := service.HashPassword(c.Password)
	if err != nil {
		return err
	}
	return ur.Create(ctx, &model.User{
		ID:                   uuid.NewString(),
		Username:             c.Username,
		DisplayName:          c.Username,
		PasswordHash:         hash,
		Avatar:               "https://api.dicebear.com/7.x/avataaars/svg?seed=demo",
		CoinBalance:          c.CoinBalance,
		Verified:             true,
		Role:                 model.RoleUser,
		LivePermissionStatus: model.LivePermissionNone,
	})
}

func seedAdminUser(ctx context.Context, ur *repo.UserRepo, c config.AdminCfg) error {
	if c.Username == "" || c.Password == "" {
		return nil
	}
	if _, err := ur.FindByUsername(ctx, c.Username); err == nil {
		return ur.EnsureAdmin(ctx, c.Username)
	} else if !errors.Is(err, repo.ErrUserNotFound) {
		return err
	}
	displayName := c.DisplayName
	if displayName == "" {
		displayName = c.Username
	}
	hash, err := service.HashPassword(c.Password)
	if err != nil {
		return err
	}
	return ur.Create(ctx, &model.User{
		ID:                   uuid.NewString(),
		Username:             c.Username,
		DisplayName:          displayName,
		PasswordHash:         hash,
		Avatar:               "https://api.dicebear.com/7.x/avataaars/svg?seed=admin",
		CoinBalance:          100000,
		Verified:             true,
		Role:                 model.RoleAdmin,
		LivePermissionStatus: model.LivePermissionApproved,
	})
}
