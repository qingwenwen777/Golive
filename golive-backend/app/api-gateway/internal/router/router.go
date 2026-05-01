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
	obs.MountMetrics(r)
	r.Use(middleware.CORS(cfg.CORS.AllowedOrigins, cfg.CORS.MaxAge))
	r.Use(middleware.RequestID())
	if cfg.RateLimit.Enabled {
		r.Use(middleware.RateLimit(cfg.RateLimit.RatePerSec, cfg.RateLimit.Burst))
	}

	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	api := r.Group("/api", middleware.JWT(cfg.JWT.Secret, publicRoutes()))
	{
		// user-service
		api.Any("/auth/*action", gin.WrapH(userProxy))
		api.Any("/users/*action", gin.WrapH(userProxy))
		api.Any("/creator/*action", gin.WrapH(userProxy))
		api.Any("/admin/*action", gin.WrapH(userProxy))

		// room-service
		api.Any("/rooms", gin.WrapH(roomProxy))
		api.Any("/rooms/*action", gin.WrapH(roomProxy))
		api.Any("/subscriptions", gin.WrapH(roomProxy))
		api.Any("/srs/*action", gin.WrapH(roomProxy))
		api.Any("/uploads/*action", uploadProxy(userProxy, roomProxy))

		// gift-service
		api.Any("/gifts", gin.WrapH(giftProxy))
		api.Any("/gifts/*action", gin.WrapH(giftProxy))
		api.Any("/super-chats", gin.WrapH(giftProxy))
		api.Any("/bets", gin.WrapH(giftProxy))
		api.Any("/bets/*action", gin.WrapH(giftProxy))

		// chat-service
		api.Any("/chat/*action", gin.WrapH(chatProxy))
	}

	return r, nil
}

// publicRoutes returns method+FullPath pairs that bypass JWT. The path must
// match Gin's FullPath() format (with the parameter name preserved).
func publicRoutes() []middleware.PublicRoute {
	return []middleware.PublicRoute{
		{Method: http.MethodPost, Path: "/api/auth/*action"},
		// Public user profiles power creator/channel pages.
		{Method: http.MethodGet, Path: "/api/users/*action"},
		// /api/rooms list + detail are also public for guest browsing.
		{Method: http.MethodGet, Path: "/api/rooms"},
		{Method: http.MethodGet, Path: "/api/rooms/*action"},
		// Gifts catalog is public.
		{Method: http.MethodGet, Path: "/api/gifts"},
		{Method: http.MethodGet, Path: "/api/bets/*action"},
		// Chat history is public for viewers entering a live room.
		{Method: http.MethodGet, Path: "/api/chat/*action"},
		// SRS callbacks come server-to-server.
		{Method: http.MethodPost, Path: "/api/srs/*action"},
		// Uploaded live covers and avatars are public assets.
		{Method: http.MethodGet, Path: "/api/uploads/*action"},
		{Method: http.MethodHead, Path: "/api/uploads/*action"},
	}
}

func uploadProxy(userProxy, roomProxy http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Param("action"), "/avatars/") {
			userProxy.ServeHTTP(c.Writer, c.Request)
			return
		}
		roomProxy.ServeHTTP(c.Writer, c.Request)
	}
}
