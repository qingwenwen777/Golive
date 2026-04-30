package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/user-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type registerReq struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	DisplayName string `json:"displayName"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		// Same shape as MSW: 401 + Invalid username or password.
		// We could 400 on missing fields, but the frontend treats anything
		// non-2xx as auth failure here.
		errcode.Respond(c, service.ErrInvalidCredentials)
		return
	}
	resp, err := h.svc.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, service.ErrInvalidRegister)
		return
	}
	resp, err := h.svc.Register(c.Request.Context(), req.Username, req.Password, req.DisplayName)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, resp)
}

type refreshReq struct {
	RefreshToken string `json:"refreshToken"`
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshReq
	if err := c.ShouldBindJSON(&req); err != nil || req.RefreshToken == "" {
		errcode.Respond(c, service.ErrInvalidRefresh)
		return
	}
	resp, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// Logout best-effort revokes the refreshToken if the body provides one.
// Per contract it always returns { ok: true }.
func (h *AuthHandler) Logout(c *gin.Context) {
	var req refreshReq
	_ = c.ShouldBindJSON(&req) // body is optional
	_ = h.svc.Logout(c.Request.Context(), req.RefreshToken)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
