package server

import (
	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/gift-service/internal/handler"
	"github.com/qingwenwen777/golive/pkg/jwtauth"
	"github.com/qingwenwen777/golive/pkg/obs"
)

type Deps struct {
	JWTSecret string
	JWTKeys   *jwtauth.KeySet
	Gift      *handler.GiftHandler
	SuperChat *handler.SuperChatHandler
	Bet       *handler.BetHandler
	LuckyBag  *handler.LuckyBagHandler
	Admin     *handler.AdminHandler
}

// NewRouter mounts gift-service routes. /api prefix stripped by api-gateway.
//
//	GET  /gifts          public catalog
//	POST /gifts/send     auth required
//	POST /gifts/fan-clubs/join auth required
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

	var auth gin.HandlerFunc
	var optionalAuth gin.HandlerFunc
	if d.JWTKeys != nil {
		auth = handler.AuthRequiredWithKeySet(d.JWTKeys)
		optionalAuth = handler.OptionalAuthWithKeySet(d.JWTKeys)
	} else {
		auth = handler.AuthRequired(d.JWTSecret)
		keys, err := jwtauth.NewKeySet(d.JWTSecret, "", nil)
		if err != nil {
			panic("gift-service jwt key set: " + err.Error())
		}
		optionalAuth = handler.OptionalAuthWithKeySet(keys)
	}

	r.GET("/gifts", d.Gift.List)
	r.GET("/gifts/fan-clubs/:creatorID/members", d.Gift.FanClubMembers)
	r.GET("/bets/latest", optionalAuth, d.Bet.Latest)
	r.GET("/gifts/fan-badges/me", auth, d.Gift.FanBadges)
	r.POST("/gifts/send", auth, d.Gift.Send)
	r.POST("/gifts/fan-clubs/join", auth, d.Gift.JoinFanClub)
	r.POST("/super-chats", auth, d.SuperChat.Send)
	r.POST("/bets", auth, d.Bet.Open)
	r.POST("/bets/:id/wagers", auth, d.Bet.Wager)
	r.POST("/bets/:id/settle", auth, d.Bet.Settle)
	r.POST("/bets/:id/cancel", auth, d.Bet.Cancel)
	r.GET("/lucky-bags/latest", optionalAuth, d.LuckyBag.Latest)
	r.POST("/lucky-bags", auth, d.LuckyBag.Open)
	r.POST("/lucky-bags/:id/join", auth, d.LuckyBag.Join)
	r.POST("/lucky-bags/:id/cancel", auth, d.LuckyBag.Cancel)
	admin := r.Group("/admin/economy", auth, handler.AdminRequired(d.Admin))
	admin.GET("/summary", d.Admin.Summary)
	admin.GET("/gifts", d.Admin.Gifts)
	admin.PATCH("/gifts/:id", d.Admin.UpdateGift)
	admin.GET("/orders", d.Admin.Orders)
	admin.GET("/coins", d.Admin.Coins)
	admin.GET("/bets", d.Admin.Bets)
	admin.POST("/bets/:id/settle", d.Admin.SettleBet)
	admin.POST("/bets/:id/cancel", d.Admin.CancelBet)
	admin.GET("/reports", d.Admin.Reports)

	return r
}
