package server

import (
	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/chat-service/internal/handler"
	"github.com/qingwenwen777/golive/pkg/obs"
)

func NewRouter(h *handler.HistoryHandler) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(obs.HTTPMiddleware("chat-service"))
	obs.MountMetrics(r)

	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	rooms := r.Group("/rooms")
	rooms.GET("/:id/danmus", h.Get)

	return r
}
