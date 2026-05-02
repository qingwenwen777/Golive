package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"github.com/qingwenwen777/golive/app/user-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type CreatorHandler struct {
	users *repo.UserRepo
}

func NewCreatorHandler(users *repo.UserRepo) *CreatorHandler {
	return &CreatorHandler{users: users}
}

type submitCreatorApplicationReq struct {
	Reason string `json:"reason"`
}

func (h *CreatorHandler) SubmitApplication(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}

	var req submitCreatorApplicationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "application reason is required"))
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "application reason is required"))
		return
	}

	app, user, created, err := h.users.SubmitCreatorApplication(c.Request.Context(), uid, reason)
	if err != nil {
		errcode.Respond(c, err)
		return
	}

	status := user.LivePermissionStatus
	if status == "" {
		status = model.LivePermissionNone
	}
	message := "Application submitted."
	if status == model.LivePermissionPending && !created {
		message = "Application is already pending."
	}
	if status == model.LivePermissionApproved {
		message = "Live permission already approved."
	}

	code := http.StatusOK
	if created {
		code = http.StatusCreated
	}
	var application any = app
	if app == nil || app.ID == "" {
		application = nil
	}
	c.JSON(code, gin.H{
		"application":          application,
		"livePermissionStatus": status,
		"user":                 user.Public(),
		"message":              message,
	})
}

type AdminHandler struct {
	users *repo.UserRepo
}

func NewAdminHandler(users *repo.UserRepo) *AdminHandler {
	return &AdminHandler{users: users}
}

func (h *AdminHandler) ListCreatorApplications(c *gin.Context) {
	items, err := h.users.ListCreatorApplications(c.Request.Context())
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *AdminHandler) ListLiveCreators(c *gin.Context) {
	items, err := h.users.ListLiveCreators(c.Request.Context())
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *AdminHandler) ApproveCreatorApplication(c *gin.Context) {
	h.reviewCreatorApplication(c, model.LivePermissionApproved)
}

func (h *AdminHandler) RejectCreatorApplication(c *gin.Context) {
	h.reviewCreatorApplication(c, model.LivePermissionRejected)
}

func (h *AdminHandler) reviewCreatorApplication(c *gin.Context, status string) {
	reviewerID := UserIDFromCtx(c)
	rejectReason := ""
	if status == model.LivePermissionRejected {
		var req struct {
			Reason string `json:"reason"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			errcode.Respond(c, errcode.New(http.StatusBadRequest, "reject reason is required"))
			return
		}
		rejectReason = strings.TrimSpace(req.Reason)
		if rejectReason == "" {
			errcode.Respond(c, errcode.New(http.StatusBadRequest, "reject reason is required"))
			return
		}
	}
	app, user, err := h.users.ReviewCreatorApplication(c.Request.Context(), c.Param("id"), reviewerID, status, rejectReason)
	if errors.Is(err, repo.ErrApplicationNotFound) {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "creator application not found"))
		return
	}
	if errors.Is(err, repo.ErrApplicationAlreadyReviewed) {
		errcode.Respond(c, errcode.New(http.StatusConflict, "creator application already reviewed"))
		return
	}
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"application": app,
		"user":        user.Public(),
	})
}

type updateLivePermissionReq struct {
	Status string `json:"status" binding:"required"`
}

func (h *AdminHandler) UpdateLivePermission(c *gin.Context) {
	var req updateLivePermissionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "live permission status is required"))
		return
	}
	status := strings.TrimSpace(req.Status)
	if status != model.LivePermissionApproved && status != model.LivePermissionRejected {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid live permission status"))
		return
	}
	user, err := h.users.SetLivePermissionStatus(c.Request.Context(), c.Param("id"), status)
	if errors.Is(err, repo.ErrUserNotFound) {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "user not found"))
		return
	}
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user.Public()})
}

type createAdminReq struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	DisplayName string `json:"displayName"`
}

func (h *AdminHandler) CreateAdmin(c *gin.Context) {
	var req createAdminReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, service.ErrInvalidRegister)
		return
	}
	username := strings.TrimSpace(req.Username)
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = username
	}
	if len(username) < 3 || len(req.Password) < 3 || displayName == "" {
		errcode.Respond(c, service.ErrInvalidRegister)
		return
	}
	if _, err := h.users.FindByUsername(c.Request.Context(), username); err == nil {
		errcode.Respond(c, service.ErrUsernameTaken)
		return
	} else if !errors.Is(err, repo.ErrUserNotFound) {
		errcode.Respond(c, err)
		return
	}
	hash, err := service.HashPassword(req.Password)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	u := &model.User{
		ID:                   uuid.NewString(),
		Username:             username,
		DisplayName:          displayName,
		PasswordHash:         hash,
		Avatar:               "https://api.dicebear.com/7.x/avataaars/svg?seed=" + url.QueryEscape(displayName),
		CoinBalance:          1200,
		Verified:             true,
		Role:                 model.RoleAdmin,
		LivePermissionStatus: model.LivePermissionApproved,
	}
	if err := h.users.Create(c.Request.Context(), u); err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": u.Public()})
}

type InternalHandler struct {
	users *repo.UserRepo
}

func NewInternalHandler(users *repo.UserRepo) *InternalHandler {
	return &InternalHandler{users: users}
}

func (h *InternalHandler) UserPermission(c *gin.Context) {
	u, err := h.users.FindByID(c.Request.Context(), c.Param("id"))
	if errors.Is(err, repo.ErrUserNotFound) {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "user not found"))
		return
	}
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"userId":               u.ID,
		"role":                 u.Public().Role,
		"livePermissionStatus": u.Public().LivePermissionStatus,
	})
}
