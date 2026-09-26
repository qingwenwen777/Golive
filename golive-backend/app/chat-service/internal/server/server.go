package server

import (
	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/chat-service/internal/handler"
	"github.com/qingwenwen777/golive/pkg/obs"
)

func NewRouter(h *handler.HistoryHandler, badges *handler.FanBadgeHandler) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(obs.HTTPMiddleware("chat-service"))
	obs.MountMetrics(r)

	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	rooms := r.Group("/rooms")
	rooms.GET("/:id/danmus", h.Get)

	chat := r.Group("/chat")
	chat.GET("/rooms/:id/danmus", h.Get)

	// Service-to-service only: api-gateway proxies /api/chat/* here, never
	// /internal/*, and chat-service publishes no port.
	internal := r.Group("/internal")
	internal.GET("/rooms/:id/fan-badges/:userId", badges.Get)

	return r
}
