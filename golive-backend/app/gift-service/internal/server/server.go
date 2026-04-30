package server

import (
	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/gift-service/internal/handler"
	"github.com/qingwenwen777/golive/pkg/obs"
)

type Deps struct {
	JWTSecret string
	Gift      *handler.GiftHandler
	SuperChat *handler.SuperChatHandler
}

// NewRouter mounts gift-service routes. /api prefix stripped by api-gateway.
//
//	GET  /gifts          public catalog
//	POST /gifts/send     auth required
//	POST /super-chats    auth required
func NewRouter(d Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(obs.HTTPMiddleware("gift-service"))
	obs.MountMetrics(r)
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	r.GET("/gifts", d.Gift.List)

	auth := handler.AuthRequired(d.JWTSecret)
	r.POST("/gifts/send", auth, d.Gift.Send)
	r.POST("/super-chats", auth, d.SuperChat.Send)

	return r
}
