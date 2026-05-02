// Package server wires HTTP routes for room-service. The /api prefix is
// stripped by api-gateway, so routes here are mounted at the bare path.
package server

import (
	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/room-service/internal/handler"
	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/obs"
)

type Deps struct {
	JWTSecret      string
	Room           *service.RoomService
	Social         *service.SocialService
	Live           *service.LiveService
	Permission     service.LivePermissionChecker
	CoverDir       string
	CoverPublicURL string
}

func NewRouter(d Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(obs.HTTPMiddleware("room-service"))
	obs.MountMetrics(r)

	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	roomH := handler.NewRoomHandler(d.Room)
	socialH := handler.NewSocialHandler(d.Social)
	liveH := handler.NewLiveHandler(d.Live, d.Permission)
	srsH := handler.NewSRSHandler(d.Live)
	coverH := handler.NewCoverUploadHandler(d.CoverDir, d.CoverPublicURL)

	auth := handler.AuthRequired(d.JWTSecret)
	optionalAuth := handler.OptionalAuth(d.JWTSecret)

	r.GET("/subscriptions", auth, socialH.ListSubscriptions)

	// Public room endpoints (no auth).
	rooms := r.Group("/rooms")
	{
		rooms.GET("", roomH.List)
		rooms.GET("/channels/:channel/history", optionalAuth, roomH.ChannelHistory)
		rooms.GET("/channels/:channel/analytics", auth, roomH.ChannelAnalytics)
		rooms.GET("/channels/:channel/history/:recordID/analytics", auth, roomH.LiveAnalysis)
		rooms.GET("/:id", optionalAuth, roomH.Get)
		rooms.GET("/:id/follow", optionalAuth, socialH.GetFollow)

		// Authenticated mutations / personalized state.
		authed := rooms.Group("", auth)
		authed.POST("/:id/follow", socialH.Follow)
		authed.DELETE("/:id/follow", socialH.Unfollow)

		authed.GET("/:id/like", socialH.GetLike)
		authed.POST("/:id/like", socialH.Like)
		authed.DELETE("/:id/like", socialH.Unlike)

		authed.POST("/:id/dislike", socialH.Dislike)
		authed.DELETE("/:id/dislike", socialH.Undislike)

		authed.POST("/live", liveH.GoLive)
		authed.PATCH("/live", liveH.UpdateLive)
		authed.DELETE("/live", liveH.StopLive)
		authed.POST("/live/cover", coverH.Upload)
	}

	uploadDir := d.CoverDir
	if uploadDir == "" {
		uploadDir = "./uploads/covers"
	}
	r.Static("/uploads/covers", uploadDir)

	// SRS callbacks — server-to-server, no JWT.
	srs := r.Group("/srs")
	{
		srs.POST("/on_publish", srsH.OnPublish)
		srs.POST("/on_unpublish", srsH.OnUnpublish)
	}

	return r
}
