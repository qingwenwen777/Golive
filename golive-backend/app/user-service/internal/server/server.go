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
	avatarH := handler.NewAvatarUploadHandler(d.Users, d.AvatarDir, d.AvatarPublicURL)

	auth := r.Group("/auth")
	{
		auth.POST("/login", authH.Login)
		auth.POST("/register", authH.Register)
		auth.POST("/refresh", authH.Refresh)
		auth.POST("/logout", authH.Logout)
	}

	users := r.Group("/users")
	{
		users.GET("/profile/:id", userH.PublicProfile)
		users.GET("/me", handler.AuthRequired(d.Auth), userH.Me)
		users.POST("/me/coins/topup", handler.AuthRequired(d.Auth), userH.TopupCoins)
		users.POST("/me/avatar", handler.AuthRequired(d.Auth), avatarH.Upload)
	}

	avatarDir := d.AvatarDir
	if avatarDir == "" {
		avatarDir = "./uploads/avatars"
	}
	r.Static("/uploads/avatars", avatarDir)

	return r
}
