// Package server wires HTTP routes for user-service. Routes do NOT carry the
// /api prefix — that is added by api-gateway via reverse proxy.
package server

import (
	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/user-service/internal/handler"
	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"github.com/qingwenwen777/golive/app/user-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/obs"
)

type Deps struct {
	Auth            *service.AuthService
	Users           *repo.UserRepo
	AvatarDir       string
	AvatarPublicURL string
	CoverDir        string
	CoverPublicURL  string
}

// NewRouter builds a Gin engine. Logger/Recovery is wired by the caller's
// preference; we keep this function pure for ease of testing.
func NewRouter(d Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(obs.HTTPMiddleware("user-service"))
	obs.MountMetrics(r)

	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	authH := handler.NewAuthHandler(d.Auth)
	userH := handler.NewUserHandler(d.Users)
	creatorH := handler.NewCreatorHandler(d.Users)
	adminH := handler.NewAdminHandler(d.Users)
	internalH := handler.NewInternalHandler(d.Users)
	avatarH := handler.NewAvatarUploadHandler(d.Users, d.AvatarDir, d.AvatarPublicURL)
	coverH := handler.NewCoverUploadHandler(d.Users, d.CoverDir, d.CoverPublicURL)

	auth := r.Group("/auth")
	{
		auth.POST("/login", authH.Login)
		auth.POST("/register", authH.Register)
		auth.POST("/refresh", authH.Refresh)
		auth.POST("/logout", authH.Logout)
		auth.GET("/me", handler.AuthRequired(d.Auth), userH.Me)
	}

	users := r.Group("/users")
	{
		users.GET("/profile/:id", userH.PublicProfile)
		users.GET("/me", handler.AuthRequired(d.Auth), userH.Me)
		users.GET("/me/coins/transactions", handler.AuthRequired(d.Auth), userH.CoinTransactions)
		users.POST("/me/coins/topup", handler.AuthRequired(d.Auth), userH.TopupCoins)
		users.POST("/me/coins/daily-tasks/:taskID/claim", handler.AuthRequired(d.Auth), userH.ClaimDailyCoinTask)
		users.POST("/me/avatar", handler.AuthRequired(d.Auth), avatarH.Upload)
		users.POST("/me/cover", handler.AuthRequired(d.Auth), coverH.Upload)
	}

	creator := r.Group("/creator", handler.AuthRequired(d.Auth))
	{
		creator.POST("/applications", creatorH.SubmitApplication)
	}

	admin := r.Group("/admin", handler.AuthRequired(d.Auth), handler.AdminRequired(d.Users))
	{
		admin.GET("/creator-applications", adminH.ListCreatorApplications)
		admin.POST("/creator-applications/:id/approve", adminH.ApproveCreatorApplication)
		admin.POST("/creator-applications/:id/reject", adminH.RejectCreatorApplication)
		admin.POST("/admins", adminH.CreateAdmin)
	}

	internal := r.Group("/internal")
	{
		internal.GET("/users/:id/permission", internalH.UserPermission)
	}

	avatarDir := d.AvatarDir
	if avatarDir == "" {
		avatarDir = "./uploads/avatars"
	}
	r.Static("/uploads/avatars", avatarDir)
	coverDir := d.CoverDir
	if coverDir == "" {
		coverDir = "./uploads/covers"
	}
	r.Static("/uploads/covers", coverDir)

	return r
}
