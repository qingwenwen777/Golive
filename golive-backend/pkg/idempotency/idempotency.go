// Package idempotency provides an HTTP middleware that deduplicates write
// requests by X-Request-Id (fallback body.requestId) using Redis as backing
// store. On replay the middleware re-serves the cached response and sets
// the header `Idempotent-Replayed: true`.
//
// TTL defaults to 10 minutes to match the frontend contract.
package idempotency

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v9"

	"github.com/qingwenwen777/golive/pkg/errcode"
)

const (
	HeaderRequestID = "X-Request-Id"
	HeaderReplayed  = "Idempotent-Replayed"
	DefaultTTL      = 10 * time.Minute
)

// Store is the minimal contract we need from the cache backend.
type Store interface {
	Get(ctx context.Context, key string) (string, bool, error)
	SetNX(ctx context.Context, key, val string, ttl time.Duration) (bool, error)
	Set(ctx context.Context, key, val string, ttl time.Duration) error
}

// RedisStore implements Store with go-redis.
type RedisStore struct {
	Client *redis.Client
}

func (r *RedisStore) Get(ctx context.Context, key string) (string, bool, error) {
	v, err := r.Client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (r *RedisStore) SetNX(ctx context.Context, key, val string, ttl time.Duration) (bool, error) {
	return r.Client.SetNX(ctx, key, val, ttl).Result()
}

func (r *RedisStore) Set(ctx context.Context, key, val string, ttl time.Duration) error {
	return r.Client.Set(ctx, key, val, ttl).Err()
}

// cachedResp is what we persist in Redis for replay.
type cachedResp struct {
	Status int               `json:"s"`
	Header map[string]string `json:"h,omitempty"`
	Body   string            `json:"b"`
}

// Config tunes the middleware.
type Config struct {
	Store     Store
	TTL       time.Duration
	KeyPrefix string // default "idem:"
	// SubjectFn derives an actor id (e.g. userId) so different users cannot
	// collide on the same requestId. Return "" for anonymous.
	SubjectFn func(c *gin.Context) string
}

// Middleware returns a gin.HandlerFunc. Mount it on endpoints that require
// idempotency (e.g. POST /api/gifts/send, POST /api/super-chats).
func Middleware(cfg Config) gin.HandlerFunc {
	if cfg.TTL == 0 {
		cfg.TTL = DefaultTTL
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "idem:"
	}

	return func(c *gin.Context) {
		reqID := extractRequestID(c)
		if reqID == "" {
			errcode.Respond(c, errcode.ErrMissingRequestID)
			return
		}

		subject := ""
		if cfg.SubjectFn != nil {
			subject = cfg.SubjectFn(c)
		}
		key := cfg.KeyPrefix + subject + ":" + reqID

		// 1) Replay hit?
		if raw, ok, err := cfg.Store.Get(c.Request.Context(), key); err == nil && ok {
			var cr cachedResp
			if json.Unmarshal([]byte(raw), &cr) == nil {
				for k, v := range cr.Header {
					c.Writer.Header().Set(k, v)
				}
				c.Writer.Header().Set(HeaderReplayed, "true")
				c.Data(cr.Status, "application/json; charset=utf-8", []byte(cr.Body))
				c.Abort()
				return
			}
		}

		// 2) First flight. Use a placeholder SetNX to prevent concurrent dup.
		ok, err := cfg.Store.SetNX(c.Request.Context(), key, "__pending__", cfg.TTL)
		if err == nil && !ok {
			// Another request is in-flight with the same id. Tell the client
			// to retry; we do NOT block because holding the connection open
			// could exceed the frontend timeout.
			errcode.Respond(c, errcode.New(http.StatusConflict, "duplicate requestId in-flight"))
			return
		}

		// 3) Capture the downstream response body so we can cache it.
		rec := &bodyRecorder{ResponseWriter: c.Writer, buf: &bytes.Buffer{}}
		c.Writer = rec

		c.Next()

		// 4) Persist the final response.
		payload, _ := json.Marshal(cachedResp{
			Status: rec.status(),
			Body:   rec.buf.String(),
		})
		_ = cfg.Store.Set(c.Request.Context(), key, string(payload), cfg.TTL)
	}
}

// extractRequestID pulls from the header first, then peeks body.requestId.
// We restore the body so downstream handlers can re-read it.
func extractRequestID(c *gin.Context) string {
	if v := c.GetHeader(HeaderRequestID); v != "" {
		return v
	}
	if c.Request.Body == nil {
		return ""
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return ""
	}
	_ = c.Request.Body.Close()
	c.Request.Body = io.NopCloser(bytes.NewBuffer(raw))

	var probe struct {
		RequestID string `json:"requestId"`
	}
	if json.Unmarshal(raw, &probe) == nil {
		return probe.RequestID
	}
	return ""
}

// bodyRecorder wraps gin.ResponseWriter to tee the body into a buffer.
type bodyRecorder struct {
	gin.ResponseWriter
	buf *bytes.Buffer
}

func (r *bodyRecorder) Write(b []byte) (int, error) {
	r.buf.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r *bodyRecorder) WriteString(s string) (int, error) {
	r.buf.WriteString(s)
	return r.ResponseWriter.WriteString(s)
}

func (r *bodyRecorder) status() int {
	s := r.ResponseWriter.Status()
	if s == 0 {
		return http.StatusOK
	}
	return s
}
