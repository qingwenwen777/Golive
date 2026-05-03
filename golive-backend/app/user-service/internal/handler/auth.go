package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/user-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type AuthHandler struct {
	svc     *service.AuthService
	captcha *service.CaptchaService
}

func NewAuthHandler(svc *service.AuthService, captcha *service.CaptchaService) *AuthHandler {
	return &AuthHandler{svc: svc, captcha: captcha}
}

type loginReq struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	CaptchaID   string `json:"captchaId"`
	CaptchaCode string `json:"captchaCode"`
}

type registerReq struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email" binding:"required"`
	InviteCode  string `json:"inviteCode" binding:"required"`
	CaptchaID   string `json:"captchaId"`
	CaptchaCode string `json:"captchaCode"`
}

type resetPasswordReq struct {
	Email       string `json:"email" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required"`
}

func (h *AuthHandler) Captcha(c *gin.Context) {
	if h.captcha == nil {
		errcode.Respond(c, service.ErrInvalidCaptcha)
		return
	}
	resp, err := h.captcha.Generate(c.Request.Context())
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
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
	if !h.verifyCaptcha(c, req.CaptchaID, req.CaptchaCode) {
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
	if !h.verifyCaptcha(c, req.CaptchaID, req.CaptchaCode) {
		return
	}
	resp, err := h.svc.RegisterWithInvite(c.Request.Context(), req.Username, req.Password, req.DisplayName, req.Email, req.InviteCode)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, resp)
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req resetPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, service.ErrInvalidRegister)
		return
	}
	if err := h.svc.ResetPasswordByEmail(c.Request.Context(), req.Email, req.NewPassword); err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
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

func (h *AuthHandler) verifyCaptcha(c *gin.Context, id, code string) bool {
	if h.captcha == nil {
		return true
	}
	if err := h.captcha.Verify(c.Request.Context(), id, code); err != nil {
		errcode.Respond(c, err)
		return false
	}
	return true
}
