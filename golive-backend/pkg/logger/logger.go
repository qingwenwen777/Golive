// Package logger wraps zap and provides context-aware loggers.
//
// Usage:
//
//	logger.Init("api-gateway", "info")
//	logger.L().Info("started", zap.Int("port", 8080))
//	logger.FromCtx(ctx).Warn("slow query")
package logger

import (
	"context"
	"os"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ctxKey struct{}

var (
	global atomic.Pointer[zap.Logger]
	once   sync.Once

	fallback = sync.OnceValue(func() *zap.Logger {
		lg, _ := zap.NewDevelopment()
		return lg
	})
)

// Init initializes the global logger. Safe to call once per process.
func Init(service, level string) {
	once.Do(func() {
		lvl := zapcore.InfoLevel
		_ = lvl.UnmarshalText([]byte(level))

		cfg := zap.NewProductionEncoderConfig()
		cfg.TimeKey = "ts"
		cfg.EncodeTime = zapcore.ISO8601TimeEncoder

		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(cfg),
			zapcore.AddSync(os.Stdout),
			lvl,
		)
		global.Store(zap.New(core, zap.AddCaller()).With(zap.String("svc", service)))
	})
}

// L returns the global logger. If Init was never called, a no-op dev logger is used.
func L() *zap.Logger {
	if lg := global.Load(); lg != nil {
		return lg
	}
	return fallback()
}

// WithCtx returns a new context carrying the given logger.
func WithCtx(ctx context.Context, lg *zap.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, lg)
}

// FromCtx retrieves a logger from ctx, falling back to the global one.
func FromCtx(ctx context.Context) *zap.Logger {
	if lg, ok := ctx.Value(ctxKey{}).(*zap.Logger); ok && lg != nil {
		return lg
	}
	return L()
}

// Sync flushes any buffered log entries. Call before process exit.
func Sync() {
	if lg := global.Load(); lg != nil {
		_ = lg.Sync()
	}
}
