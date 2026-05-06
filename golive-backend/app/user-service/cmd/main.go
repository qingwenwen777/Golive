package main

import (
	"context"
	"errors"
	"flag"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-redis/redis/v9"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	userv1 "github.com/qingwenwen777/golive/api/gen/go/user/v1"
	"github.com/qingwenwen777/golive/app/user-service/internal/config"
	"github.com/qingwenwen777/golive/app/user-service/internal/grpcserver"
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

	userRepo := repo.NewUserRepo(db).WithRedis(rdb)
	if err := userRepo.AutoMigrate(); err != nil {
		log.Fatal("auto migrate", zap.Error(err))
	}
	if err := userRepo.BackfillMissingEmails(context.Background()); err != nil {
		log.Warn("backfill user emails", zap.Error(err))
	}
	if err := userRepo.ReconcilePlatformVerification(context.Background()); err != nil {
		log.Warn("reconcile platform verification", zap.Error(err))
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
	jwtKeys, err := cfg.JWT.KeySet()
	if err != nil {
		log.Fatal("jwt config", zap.Error(err))
	}

	tokenRepo := repo.NewTokenRepo(rdb)
	captcha := service.NewCaptchaService(rdb, 5*time.Minute)
	var emailMailer service.EmailCodeMailer
	if cfg.Email.Enabled {
		emailMailer, err = service.NewSMTPMailer(service.SMTPMailerConfig{
			Host:        cfg.Email.SMTPHost,
			Port:        cfg.Email.SMTPPort,
			Username:    cfg.Email.SMTPUsername,
			Password:    cfg.Email.SMTPPassword,
			SenderEmail: cfg.Email.SenderEmail,
			SenderName:  cfg.Email.SenderName,
		})
		if err != nil {
			log.Fatal("email config", zap.Error(err))
		}
	}
	emailCodes := service.NewEmailCodeService(rdb, emailMailer, cfg.Email.CodeTTL, cfg.Email.ResendInterval)
	auth := service.NewAuthService(userRepo, tokenRepo, service.Options{
		JWTSecret:      cfg.JWT.Secret,
		JWTKeys:        jwtKeys,
		AccessTTL:      cfg.JWT.AccessTTL,
		RefreshTTL:     cfg.JWT.RefreshTTL,
		GoogleClientID: cfg.Google.ClientID,
	})
	stripeSvc := service.NewStripeService(service.StripeOptions{
		PublishableKey:       cfg.Stripe.PublishableKey,
		SecretKey:            cfg.Stripe.SecretKey,
		Currency:             cfg.Stripe.Currency,
		CoinsPerCurrencyUnit: cfg.Stripe.CoinsPerCurrencyUnit,
		ConnectCountry:       cfg.Stripe.ConnectCountry,
	})

	r := server.NewRouter(server.Deps{
		Auth:            auth,
		Captcha:         captcha,
		EmailCodes:      emailCodes,
		Users:           userRepo,
		Stripe:          stripeSvc,
		AvatarDir:       cfg.Upload.AvatarDir,
		AvatarPublicURL: cfg.Upload.AvatarPublicURL,
		CoverDir:        cfg.Upload.CoverDir,
		CoverPublicURL:  cfg.Upload.CoverPublicURL,
	})
	httpSrv := &http.Server{Addr: cfg.Service.HTTPAddr, Handler: r}
	var grpcSrv *grpc.Server

	// pprof on a side port.
	go func() {
		if err := http.ListenAndServe(cfg.Service.PprofAddr, nil); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn("pprof exit", zap.Error(err))
		}
	}()
	if cfg.Service.GRPCAddr != "" {
		lis, err := net.Listen("tcp", cfg.Service.GRPCAddr)
		if err != nil {
			log.Fatal("listen grpc", zap.String("addr", cfg.Service.GRPCAddr), zap.Error(err))
		}
		grpcSrv = grpc.NewServer()
		userv1.RegisterUserServiceServer(grpcSrv, grpcserver.NewUserServer(userRepo))
		go func() {
			log.Info("user-service grpc listening", zap.String("addr", cfg.Service.GRPCAddr))
			if err := grpcSrv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				log.Fatal("grpc exit", zap.Error(err))
			}
		}()
	}

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
	if grpcSrv != nil {
		done := make(chan struct{})
		go func() {
			grpcSrv.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
			grpcSrv.Stop()
		}
	}
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
		Email:                strings.ToLower(c.Username) + "@gmail.com",
		DisplayName:          c.Username,
		PasswordHash:         hash,
		Avatar:               "https://api.dicebear.com/7.x/avataaars/svg?seed=demo",
		CoinBalance:          c.CoinBalance,
		Verified:             false,
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
		Email:                strings.ToLower(c.Username) + "@gmail.com",
		DisplayName:          displayName,
		PasswordHash:         hash,
		Avatar:               "https://api.dicebear.com/7.x/avataaars/svg?seed=admin",
		CoinBalance:          100000,
		Verified:             false,
		Role:                 model.RoleAdmin,
		LivePermissionStatus: model.LivePermissionApproved,
	})
}
