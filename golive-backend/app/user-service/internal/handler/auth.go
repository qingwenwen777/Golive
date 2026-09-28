package handler

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/user-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

const refreshCookieName = "golive_refresh"

type AuthHandler struct {
	svc        *service.AuthService
	captcha    *service.CaptchaService
	emailCodes *service.EmailCodeService
}

func NewAuthHandler(svc *service.AuthService, captcha *service.CaptchaService, emailCodes *service.EmailCodeService) *AuthHandler {
	return &AuthHandler{svc: svc, captcha: captcha, emailCodes: emailCodes}
}

// loginReq carries the image captcha or, for people who cannot read it, a
// login code sent to the account's verified email (Email and EmailCode).
type loginReq struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	CaptchaID   string `json:"captchaId"`
	CaptchaCode string `json:"captchaCode"`
	Email       string `json:"email"`
	EmailCode   string `json:"emailCode"`
}

type registerReq struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email" binding:"required"`
	EmailCode   string `json:"emailCode" binding:"required"`
	InviteCode  string `json:"inviteCode" binding:"required"`
	CaptchaID   string `json:"captchaId"`
	CaptchaCode string `json:"captchaCode"`
}

type resetPasswordReq struct {
	Username    string `json:"username" binding:"required"`
	Email       string `json:"email" binding:"required"`
	EmailCode   string `json:"emailCode" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required"`
}

type sendEmailCodeReq struct {
	Purpose  string `json:"purpose" binding:"required"`
	Username string `json:"username"`
	Email    string `json:"email" binding:"required"`
}

type googleLoginReq struct {
	Credential string `json:"credential" binding:"required"`
}

type googleRegisterReq struct {
	Credential  string `json:"credential" binding:"required"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	InviteCode  string `json:"inviteCode" binding:"required"`
}

type googleLinkReq struct {
	Credential string `json:"credential" binding:"required"`
	Password   string `json:"password" binding:"required"`
}

type googleUnlinkReq struct {
	Password string `json:"password" binding:"required"`
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

func (h *AuthHandler) SendEmailCode(c *gin.Context) {
	var req sendEmailCodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, service.ErrInvalidRegister)
		return
	}
	if h.emailCodes == nil {
		errcode.Respond(c, service.ErrEmailNotConfigured)
		return
	}
	switch req.Purpose {
	case service.EmailPurposeRegister:
	case service.EmailPurposePasswordReset, service.EmailPurposeLogin:
		if err := h.svc.EnsureUsernameEmailMatch(c.Request.Context(), req.Username, req.Email); err != nil {
			errcode.Respond(c, err)
			return
		}
	default:
		errcode.Respond(c, service.ErrInvalidRegister.WithReason("invalid_email_code_purpose"))
		return
	}
	resp, err := h.emailCodes.Send(c.Request.Context(), req.Purpose, req.Email)
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
	if strings.TrimSpace(req.EmailCode) != "" {
		if !h.verifyLoginEmailCode(c, req.Username, req.Email, req.EmailCode) {
			return
		}
	} else if !h.verifyCaptcha(c, req.CaptchaID, req.CaptchaCode) {
		return
	}
	resp, err := h.svc.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	h.setRefreshCookie(c, resp.RefreshToken)
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
	if h.emailCodes == nil {
		errcode.Respond(c, service.ErrEmailNotConfigured)
		return
	}
	if err := h.emailCodes.Verify(c.Request.Context(), service.EmailPurposeRegister, req.Email, req.EmailCode); err != nil {
		errcode.Respond(c, err)
		return
	}
	resp, err := h.svc.RegisterWithInvite(c.Request.Context(), req.Username, req.Password, req.DisplayName, req.Email, req.InviteCode)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	h.setRefreshCookie(c, resp.RefreshToken)
	c.JSON(http.StatusCreated, resp)
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req resetPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, service.ErrInvalidRegister)
		return
	}
	if h.emailCodes == nil {
		errcode.Respond(c, service.ErrEmailNotConfigured)
		return
	}
	if err := h.svc.EnsureUsernameEmailMatch(c.Request.Context(), req.Username, req.Email); err != nil {
		errcode.Respond(c, err)
		return
	}
	if err := h.emailCodes.Verify(c.Request.Context(), service.EmailPurposePasswordReset, req.Email, req.EmailCode); err != nil {
		errcode.Respond(c, err)
		return
	}
	if err := h.svc.ResetPasswordByUsernameEmail(c.Request.Context(), req.Username, req.Email, req.NewPassword); err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AuthHandler) GoogleLogin(c *gin.Context) {
	var req googleLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, service.ErrInvalidGoogleCredential)
		return
	}
	resp, err := h.svc.GoogleLogin(c.Request.Context(), req.Credential)
	if err != nil {
		if respondGoogleEmailTaken(c, err) {
			return
		}
		errcode.Respond(c, err)
		return
	}
	h.setRefreshCookie(c, resp.RefreshToken)
	c.JSON(http.StatusOK, resp)
}

func (h *AuthHandler) GoogleRegister(c *gin.Context) {
	var req googleRegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, service.ErrInvalidRegister)
		return
	}
	resp, err := h.svc.GoogleRegisterWithInvite(c.Request.Context(), req.Credential, req.Username, req.DisplayName, req.InviteCode)
	if err != nil {
		if respondGoogleEmailTaken(c, err) {
			return
		}
		errcode.Respond(c, err)
		return
	}
	h.setRefreshCookie(c, resp.RefreshToken)
	c.JSON(http.StatusCreated, resp)
}

func (h *AuthHandler) GoogleLinkExisting(c *gin.Context) {
	var req googleLinkReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, service.ErrInvalidRegister)
		return
	}
	resp, err := h.svc.GoogleLinkByPassword(c.Request.Context(), req.Credential, req.Password)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	h.setRefreshCookie(c, resp.RefreshToken)
	c.JSON(http.StatusOK, resp)
}

func respondGoogleEmailTaken(c *gin.Context, err error) bool {
	var emailTaken *service.GoogleEmailTakenError
	if !errors.As(err, &emailTaken) {
		return false
	}
	c.AbortWithStatusJSON(http.StatusConflict, gin.H{
		"message": "This Google email is already used by another account",
		"reason":  "google_email_exists",
	})
	return true
}

func (h *AuthHandler) GoogleBind(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	var req googleLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, service.ErrInvalidGoogleCredential)
		return
	}
	user, err := h.svc.GoogleLinkCurrentUser(c.Request.Context(), uid, req.Credential)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

func (h *AuthHandler) GoogleUnbind(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	var req googleUnlinkReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, service.ErrInvalidRegister)
		return
	}
	user, err := h.svc.GoogleUnlinkCurrentUser(c.Request.Context(), uid, req.Password)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

type refreshReq struct {
	RefreshToken string `json:"refreshToken"`
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshReq
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		errcode.Respond(c, service.ErrInvalidRefresh)
		return
	}
	if req.RefreshToken == "" {
		if cookie, err := c.Cookie(refreshCookieName); err == nil {
			req.RefreshToken = cookie
		}
	}
	if req.RefreshToken == "" {
		errcode.Respond(c, service.ErrInvalidRefresh)
		return
	}
	resp, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	h.setRefreshCookie(c, resp.RefreshToken)
	c.JSON(http.StatusOK, resp)
}

// Logout best-effort revokes the refreshToken if the body provides one.
// Per contract it always returns { ok: true }.
func (h *AuthHandler) Logout(c *gin.Context) {
	var req refreshReq
	_ = c.ShouldBindJSON(&req) // body is optional
	if req.RefreshToken == "" {
		if cookie, err := c.Cookie(refreshCookieName); err == nil {
			req.RefreshToken = cookie
		}
	}
	_ = h.svc.Logout(c.Request.Context(), req.RefreshToken)
	h.clearRefreshCookie(c)
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

// verifyLoginEmailCode stands in for verifyCaptcha when a password sign-in
// brings a login code sent to the account's verified email. The code is
// checked, and used up, before the password is looked at: without the mailbox
// this path says nothing about the password, and with it every guess at the
// password costs a new code.
func (h *AuthHandler) verifyLoginEmailCode(c *gin.Context, username, email, code string) bool {
	if h.emailCodes == nil {
		errcode.Respond(c, service.ErrEmailNotConfigured)
		return false
	}
	ctx := c.Request.Context()
	if err := h.svc.EnsureUsernameEmailMatch(ctx, username, email); err != nil {
		errcode.Respond(c, err)
		return false
	}
	if err := h.emailCodes.Verify(ctx, service.EmailPurposeLogin, email, code); err != nil {
		errcode.Respond(c, err)
		return false
	}
	return true
}

func (h *AuthHandler) setRefreshCookie(c *gin.Context, token string) {
	maxAge := int(h.svc.RefreshTTL().Seconds())
	if maxAge <= 0 {
		maxAge = 7 * 24 * 60 * 60
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     "/api/auth",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPSRequest(c),
	})
}

func (h *AuthHandler) clearRefreshCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/auth",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPSRequest(c),
	})
}

func isHTTPSRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https")
}
