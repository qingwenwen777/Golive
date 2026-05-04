// Package server wires HTTP routes for room-service. The /api prefix is
// stripped by api-gateway, so routes here are mounted at the bare path.
package server

import (
	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/room-service/internal/handler"
	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/jwtauth"
	"github.com/qingwenwen777/golive/pkg/obs"
)

type Deps struct {
	JWTSecret      string
	JWTKeys        *jwtauth.KeySet
	Room           *service.RoomService
	Social         *service.SocialService
	Posts          *service.PostService
	Live           *service.LiveService
	Replay         *service.ReplayService
	Search         *service.SearchService
	Appointments   *service.AppointmentService
	Moderation     *service.ModerationService
	Permission     service.LivePermissionChecker
	CoverDir       string
	CoverPublicURL string
	PostImageDir   string
	PostPublicURL  string
}

func NewRouter(d Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(obs.HTTPMiddleware("room-service"))
	obs.MountMetrics(r)

	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	roomH := handler.NewRoomHandler(d.Room)
	socialH := handler.NewSocialHandler(d.Social)
	postH := handler.NewPostHandler(d.Posts, d.Permission, d.PostImageDir, d.PostPublicURL)
	liveH := handler.NewLiveHandler(d.Live, d.Permission)
	replayH := handler.NewReplayHandler(d.Replay)
	searchH := handler.NewSearchHandler(d.Search)
	appointmentH := handler.NewAppointmentHandler(d.Appointments)
	moderationH := handler.NewModerationHandler(d.Moderation)
	srsH := handler.NewSRSHandler(d.Live)
	coverH := handler.NewCoverUploadHandler(d.CoverDir, d.CoverPublicURL)

	auth := handler.AuthRequired(d.JWTSecret)
	optionalAuth := handler.OptionalAuth(d.JWTSecret)
	if d.JWTKeys != nil {
		auth = handler.AuthRequiredWithKeySet(d.JWTKeys)
		optionalAuth = handler.OptionalAuthWithKeySet(d.JWTKeys)
	}

	r.GET("/subscriptions", auth, socialH.ListSubscriptions)
	r.GET("/subscriptions/appointments", auth, appointmentH.ListSubscriptionAppointments)
	r.GET("/subscriptions/posts", auth, postH.ListSubscriptionLatest)
	r.GET("/appointments/upcoming", optionalAuth, appointmentH.ListUpcoming)
	r.GET("/appointments/my", auth, appointmentH.ListReserved)
	r.GET("/notifications", auth, appointmentH.Notifications)
	r.PATCH("/notifications/read-all", auth, appointmentH.MarkAllNotificationsRead)
	r.PATCH("/notifications/:id/read", auth, appointmentH.MarkNotificationRead)

	// Public room endpoints (no auth).
	rooms := r.Group("/rooms")
	{
		rooms.GET("", roomH.List)
		rooms.GET("/recommended", optionalAuth, roomH.Recommended)
		if d.Search != nil {
			rooms.GET("/search", optionalAuth, searchH.Search)
			rooms.GET("/search/suggestions", optionalAuth, searchH.Suggest)
		}
		rooms.GET("/appointments", auth, appointmentH.ListOwner)
		rooms.POST("/appointments", auth, appointmentH.Create)
		rooms.PATCH("/appointments/:id", auth, appointmentH.Update)
		rooms.DELETE("/appointments/:id/record", auth, appointmentH.DeleteRecord)
		rooms.DELETE("/appointments/:id", auth, appointmentH.Cancel)
		rooms.POST("/appointments/:id/start", auth, appointmentH.Start)
		rooms.POST("/appointments/:id/reservations", auth, appointmentH.Reserve)
		rooms.DELETE("/appointments/:id/reservations", auth, appointmentH.Unreserve)
		rooms.GET("/moderation/followers", auth, moderationH.ListFollowers)
		rooms.GET("/moderation/moderators", auth, moderationH.ListModerators)
		rooms.POST("/moderation/moderators/:userID", auth, moderationH.AddModerator)
		rooms.DELETE("/moderation/moderators/:userID", auth, moderationH.RemoveModerator)
		rooms.GET("/moderation/logs", auth, moderationH.Logs)
		rooms.GET("/channels/:channel/history", optionalAuth, roomH.ChannelHistory)
		rooms.GET("/channels/:channel/appointments", optionalAuth, appointmentH.ListChannel)
		rooms.GET("/channels/:channel/posts", optionalAuth, postH.ListChannel)
		rooms.GET("/channels/:channel/analytics", auth, roomH.ChannelAnalytics)
		rooms.GET("/channels/:channel/history/:recordID/analytics", auth, roomH.LiveAnalysis)
		rooms.GET("/posts/mine", auth, postH.ListMine)
		rooms.GET("/replays/hot", optionalAuth, roomH.HotReplays)
		rooms.GET("/replays/mine", auth, replayH.ListMine)
		rooms.POST("/posts", auth, postH.Create)
		rooms.POST("/posts/images", auth, postH.UploadImage)
		rooms.PATCH("/posts/:postID", auth, postH.UpdateVisibility)
		rooms.DELETE("/posts/:postID", auth, postH.Delete)
		rooms.GET("/posts/:postID/comments", optionalAuth, postH.ListComments)
		rooms.POST("/posts/:postID/comments", auth, postH.CreateComment)
		rooms.DELETE("/posts/:postID/comments/:commentID", auth, postH.DeleteComment)
		rooms.POST("/posts/:postID/like", auth, postH.Like)
		rooms.DELETE("/posts/:postID/like", auth, postH.Unlike)
		rooms.POST("/posts/:postID/comments/:commentID/like", auth, postH.LikeComment)
		rooms.DELETE("/posts/:postID/comments/:commentID/like", auth, postH.UnlikeComment)
		rooms.GET("/recommended-creators", optionalAuth, socialH.RecommendedCreators)
		rooms.GET("/:id/moderation/state", optionalAuth, moderationH.RoomState)
		rooms.GET("/:id/moderation/mutes/:userID", auth, moderationH.MuteState)
		rooms.POST("/:id/moderation/mutes", auth, moderationH.Mute)
		rooms.DELETE("/:id/moderation/mutes/:userID", auth, moderationH.Unmute)
		rooms.GET("/:id", optionalAuth, roomH.Get)
		rooms.GET("/:id/follow", optionalAuth, socialH.GetFollow)

		// Authenticated mutations / personalized state.
		authed := rooms.Group("", auth)
		authed.POST("/:id/follow", socialH.Follow)
		authed.DELETE("/:id/follow", socialH.Unfollow)

		authed.GET("/:id/like", socialH.GetLike)
		authed.POST("/:id/watch", roomH.RecordWatch)
		authed.POST("/:id/like", socialH.Like)
		authed.DELETE("/:id/like", socialH.Unlike)

		authed.POST("/:id/dislike", socialH.Dislike)
		authed.DELETE("/:id/dislike", socialH.Undislike)

		authed.POST("/live", liveH.GoLive)
		authed.PATCH("/live", liveH.UpdateLive)
		authed.DELETE("/live", liveH.StopLive)
		authed.PATCH("/live/replay", replayH.UpdateActiveSettings)
		authed.POST("/live/cover", coverH.Upload)
		authed.PATCH("/replays/:id", replayH.Update)
		authed.DELETE("/replays/:id", replayH.Delete)
	}

	uploadDir := d.CoverDir
	if uploadDir == "" {
		uploadDir = "./uploads/covers"
	}
	r.Static("/uploads/covers", uploadDir)
	postUploadDir := d.PostImageDir
	if postUploadDir == "" {
		postUploadDir = "./uploads/posts"
	}
	r.Static("/uploads/posts", postUploadDir)

	// SRS callbacks — server-to-server, no JWT.
	srs := r.Group("/srs")
	{
		srs.POST("/on_publish", srsH.OnPublish)
		srs.POST("/on_unpublish", srsH.OnUnpublish)
	}

	return r
}
