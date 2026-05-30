// Package router wires the gateway: middleware order + upstream routing.
//
// Middleware order matters:
//
//  1. Recovery       — never let a panic kill the process
//  2. CORS           — preflight must short-circuit before auth runs
//  3. RequestID      — generate before anything that might log or forward
//  4. RateLimit      — cheap, drop floods early
//  5. JWT            — last gate before forwarding; injects X-User-Id
//  6. Proxy handler  — picked per route by FullPath()
package router

import (
	"bytes"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/api-gateway/internal/config"
	"github.com/qingwenwen777/golive/app/api-gateway/internal/middleware"
	"github.com/qingwenwen777/golive/app/api-gateway/internal/proxy"
	"github.com/qingwenwen777/golive/pkg/obs"
)

// New builds the gateway's *gin.Engine.
func New(cfg *config.Config) (*gin.Engine, error) {
	userURL, err := url.Parse(cfg.Upstreams.UserService)
	if err != nil {
		return nil, err
	}
	roomURL, err := url.Parse(cfg.Upstreams.RoomService)
	if err != nil {
		return nil, err
	}
	giftURL, err := url.Parse(cfg.Upstreams.GiftService)
	if err != nil {
		return nil, err
	}
	chatURL, err := url.Parse(cfg.Upstreams.ChatService)
	if err != nil {
		return nil, err
	}
	popts := proxy.Options{
		Timeout:             cfg.Proxy.Timeout,
		MaxIdleConns:        cfg.Proxy.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.Proxy.MaxIdleConnsPerHost,
	}
	userProxy := proxy.New(userURL, popts)
	roomProxy := proxy.New(roomURL, popts)
	giftProxy := proxy.New(giftURL, popts)
	chatProxy := proxy.New(chatURL, popts)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(obs.HTTPMiddleware("api-gateway"))
	r.Use(middleware.SecurityHeaders())
	obs.MountMetrics(r)
	r.Use(middleware.CORS(cfg.CORS.AllowedOrigins, cfg.CORS.MaxAge))
	csrf, err := middleware.NewCSRFProtector(cfg.CSRF.Secret, cfg.CSRF.TokenTTL, cfg.CORS.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	r.Use(csrf.Guard())
	r.Use(middleware.RequestID())
	if cfg.RateLimit.Enabled {
		r.Use(middleware.RateLimit(cfg.RateLimit.RatePerSec, cfg.RateLimit.Burst))
		r.Use(middleware.AuthRateLimit(cfg.RateLimit.AuthRatePerSec, cfg.RateLimit.AuthBurst))
	}
	jwtKeys, err := cfg.JWT.KeySet()
	if err != nil {
		return nil, err
	}

	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	api := r.Group("/api", middleware.JWTWithKeySet(jwtKeys, publicRoutes()))
	{
		// user-service
		api.GET("/csrf-token", csrf.Token)
		api.Any("/auth/*action", gin.WrapH(userProxy))
		api.Any("/users/*action", gin.WrapH(userProxy))
		api.Any("/creator/*action", gin.WrapH(userProxy))
		api.Any("/admin/*action", adminProxy(userProxy, giftProxy))

		// room-service
		api.Any("/rooms", gin.WrapH(roomProxy))
		api.Any("/rooms/*action", gin.WrapH(roomProxy))
		api.Any("/subscriptions", gin.WrapH(roomProxy))
		api.Any("/subscriptions/*action", gin.WrapH(roomProxy))
		api.Any("/appointments/*action", gin.WrapH(roomProxy))
		api.Any("/notifications", gin.WrapH(roomProxy))
		api.Any("/notifications/*action", gin.WrapH(roomProxy))
		api.Any("/messages", gin.WrapH(roomProxy))
		api.Any("/messages/*action", gin.WrapH(roomProxy))
		api.Any("/uploads/*action", uploadProxy(userProxy, roomProxy))

		// gift-service
		api.Any("/gifts", gin.WrapH(giftProxy))
		api.Any("/gifts/*action", gin.WrapH(giftProxy))
		api.Any("/super-chats", gin.WrapH(giftProxy))
		api.Any("/bets", gin.WrapH(giftProxy))
		api.Any("/bets/*action", gin.WrapH(giftProxy))
		api.Any("/lucky-bags", gin.WrapH(giftProxy))
		api.Any("/lucky-bags/*action", gin.WrapH(giftProxy))

		// chat-service
		api.Any("/chat/*action", gin.WrapH(chatProxy))
	}

	return r, nil
}

// publicRoutes returns method+FullPath pairs that bypass JWT. The path must
// match Gin's FullPath() format (with the parameter name preserved).
func publicRoutes() []middleware.PublicRoute {
	return []middleware.PublicRoute{
		{Method: http.MethodGet, Path: "/api/csrf-token"},
		{Method: http.MethodGet, Path: "/api/auth/*action"},
		{Method: http.MethodPost, Path: "/api/auth/*action"},
		// Public user profiles power creator/channel pages.
		{Method: http.MethodGet, Path: "/api/users/*action"},
		// /api/rooms list + detail are also public for guest browsing.
		{Method: http.MethodGet, Path: "/api/rooms"},
		{Method: http.MethodGet, Path: "/api/rooms/*action"},
		{Method: http.MethodGet, Path: "/api/rooms/channels/:channel/posts"},
		{Method: http.MethodGet, Path: "/api/rooms/posts/:postID/comments"},
		{Method: http.MethodGet, Path: "/api/appointments/*action"},
		// Gifts catalog and fan-club public previews are public.
		{Method: http.MethodGet, Path: "/api/gifts"},
		{Method: http.MethodGet, Path: "/api/gifts/*action"},
		{Method: http.MethodGet, Path: "/api/bets/*action"},
		{Method: http.MethodGet, Path: "/api/lucky-bags/*action"},
		// Chat history is public for viewers entering a live room.
		{Method: http.MethodGet, Path: "/api/chat/*action"},
		// Uploaded live covers and avatars are public assets.
		{Method: http.MethodGet, Path: "/api/uploads/*action"},
		{Method: http.MethodHead, Path: "/api/uploads/*action"},
	}
}

func adminProxy(userProxy, giftProxy http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Param("action"), "/economy") {
			giftProxy.ServeHTTP(c.Writer, c.Request)
			return
		}
		userProxy.ServeHTTP(c.Writer, c.Request)
	}
}

func uploadProxy(userProxy, roomProxy http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		action := c.Param("action")
		if strings.HasPrefix(action, "/avatars/") {
			userProxy.ServeHTTP(c.Writer, c.Request)
			return
		}
		if strings.HasPrefix(action, "/covers/") {
			serveCoverUpload(c, roomProxy, userProxy)
			return
		}
		roomProxy.ServeHTTP(c.Writer, c.Request)
	}
}

func serveCoverUpload(c *gin.Context, roomProxy, userProxy http.Handler) {
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		userProxy.ServeHTTP(c.Writer, c.Request)
		return
	}

	rec := newCaptureWriter()
	roomProxy.ServeHTTP(rec, c.Request)
	if rec.status != http.StatusNotFound {
		rec.replay(c.Writer, c.Request.Method == http.MethodHead)
		return
	}
	userProxy.ServeHTTP(c.Writer, c.Request)
}

type captureWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newCaptureWriter() *captureWriter {
	return &captureWriter{header: make(http.Header)}
}

func (w *captureWriter) Header() http.Header {
	return w.header
}

func (w *captureWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *captureWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(p)
}

func (w *captureWriter) replay(dst http.ResponseWriter, head bool) {
	for key, values := range w.header {
		for _, value := range values {
			dst.Header().Add(key, value)
		}
	}
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	dst.WriteHeader(status)
	if !head {
		_, _ = dst.Write(w.body.Bytes())
	}
}
