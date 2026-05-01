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
	Bet       *handler.BetHandler
}

// NewRouter mounts gift-service routes. /api prefix stripped by api-gateway.
//
//	GET  /gifts          public catalog
//	POST /gifts/send     auth required
//	GET  /gifts/fan-badges/me auth required
//	POST /super-chats    auth required
//	GET  /bets/latest    public current/latest room bet
//	POST /bets           auth required, host only
func NewRouter(d Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(obs.HTTPMiddleware("gift-service"))
	obs.MountMetrics(r)
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	r.GET("/gifts", d.Gift.List)
	r.GET("/bets/latest", d.Bet.Latest)

	auth := handler.AuthRequired(d.JWTSecret)
	r.GET("/gifts/fan-badges/me", auth, d.Gift.FanBadges)
	r.POST("/gifts/send", auth, d.Gift.Send)
	r.POST("/super-chats", auth, d.SuperChat.Send)
	r.POST("/bets", auth, d.Bet.Open)
	r.POST("/bets/:id/wagers", auth, d.Bet.Wager)
	r.POST("/bets/:id/settle", auth, d.Bet.Settle)
	r.POST("/bets/:id/cancel", auth, d.Bet.Cancel)

	return r
}
