package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

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

func (h *CreatorHandler) SubmitPlatformApplication(c *gin.Context) {
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

	app, user, created, err := h.users.SubmitPlatformApplication(c.Request.Context(), uid, reason)
	if errors.Is(err, repo.ErrLivePermissionRequired) {
		errcode.Respond(c, errcode.New(http.StatusConflict, "live permission must be approved first"))
		return
	}
	if err != nil {
		errcode.Respond(c, err)
		return
	}

	status := user.PlatformVerificationStatus
	if status == "" {
		status = model.PlatformVerificationNone
	}
	message := "Platform certification application submitted."
	if status == model.PlatformVerificationPending && !created {
		message = "Platform certification application is already pending."
	}
	if status == model.PlatformVerificationApproved {
		message = "Platform certification already approved."
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
		"application":                application,
		"platformVerificationStatus": status,
		"user":                       user.Public(),
		"message":                    message,
	})
}

type AdminHandler struct {
	users *repo.UserRepo
}

func NewAdminHandler(users *repo.UserRepo) *AdminHandler {
	return &AdminHandler{users: users}
}

func (h *AdminHandler) CreateInviteCode(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	invite, err := h.users.CreateInviteCode(c.Request.Context(), uid)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	h.logAdminAudit(c, model.AdminAuditCategorySystem, "invite_create", "invite_code", invite.ID, invite.Code, "", "", "created invite code")
	c.JSON(http.StatusCreated, gin.H{"inviteCode": invite})
}

func (h *AdminHandler) ListInviteCodes(c *gin.Context) {
	items, err := h.users.ListInviteCodes(c.Request.Context())
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *AdminHandler) DeleteInviteCode(c *gin.Context) {
	invite, err := h.users.DeleteUnusedInviteCode(c.Request.Context(), c.Param("id"))
	if errors.Is(err, repo.ErrInviteNotFound) {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "invite code not found"))
		return
	}
	if errors.Is(err, repo.ErrInviteUsed) {
		errcode.Respond(c, errcode.New(http.StatusConflict, "used invite codes cannot be deleted"))
		return
	}
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	h.logAdminAudit(c, model.AdminAuditCategorySystem, "invite_delete", "invite_code", invite.ID, invite.Code, "", "", "deleted unused invite code")
	c.JSON(http.StatusOK, gin.H{"inviteCode": invite})
}

func (h *AdminHandler) ListUsers(c *gin.Context) {
	page, size := adminPageSize(c, 1, 20)
	items, total, stats, err := h.users.AdminListUsers(c.Request.Context(), repo.AdminUserListFilter{
		Query:  c.Query("q"),
		Role:   c.DefaultQuery("role", "all"),
		Status: c.DefaultQuery("status", "all"),
		Page:   page,
		Size:   size,
	})
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items": items,
		"total": total,
		"page":  page,
		"size":  size,
		"stats": stats,
	})
}

func (h *AdminHandler) UserDetail(c *gin.Context) {
	detail, err := h.users.AdminUserDetail(c.Request.Context(), c.Param("id"))
	if errors.Is(err, repo.ErrUserNotFound) {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "user not found"))
		return
	}
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

type adminUpdateUserProfileReq struct {
	Username    *string `json:"username"`
	DisplayName *string `json:"displayName"`
}

func (h *AdminHandler) UpdateUserProfile(c *gin.Context) {
	var req adminUpdateUserProfileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid body"))
		return
	}
	username, ok := cleanUsernamePatch(req.Username, c)
	if !ok {
		return
	}
	displayName, ok := cleanDisplayNamePatch(req.DisplayName, c)
	if !ok {
		return
	}
	if username == nil && displayName == nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "nothing to update"))
		return
	}
	u, err := h.users.AdminUpdateProfile(c.Request.Context(), c.Param("id"), username, displayName)
	if errors.Is(err, repo.ErrUsernameTaken) {
		errcode.Respond(c, service.ErrUsernameTaken.WithReason("username_taken"))
		return
	}
	if errors.Is(err, repo.ErrUserNotFound) {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "user not found"))
		return
	}
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	h.logAdminAudit(c, model.AdminAuditCategoryPermission, "user_profile_update", "user", u.ID, u.DisplayName, u.ID, u.DisplayName, "updated user profile")
	c.JSON(http.StatusOK, gin.H{"user": u.Public()})
}

type adminUpdateUserRoleReq struct {
	Role string `json:"role" binding:"required"`
}

func (h *AdminHandler) UpdateUserRole(c *gin.Context) {
	var req adminUpdateUserRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "role is required"))
		return
	}
	role := strings.TrimSpace(req.Role)
	if role != model.RoleUser && role != model.RoleAdmin && role != model.RoleModerator {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid role"))
		return
	}
	if c.Param("id") == UserIDFromCtx(c) && role != model.RoleAdmin {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "cannot remove your own admin role"))
		return
	}
	u, err := h.users.AdminUpdateRole(c.Request.Context(), c.Param("id"), role)
	if errors.Is(err, repo.ErrUserNotFound) {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "user not found"))
		return
	}
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	h.logAdminAudit(c, model.AdminAuditCategoryPermission, "user_role_update", "user", u.ID, u.DisplayName, u.ID, u.DisplayName, "role="+role)
	c.JSON(http.StatusOK, gin.H{"user": u.Public()})
}

type adminBanUserReq struct {
	Banned bool   `json:"banned"`
	Reason string `json:"reason"`
}

func (h *AdminHandler) SetUserBan(c *gin.Context) {
	var req adminBanUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid body"))
		return
	}
	if c.Param("id") == UserIDFromCtx(c) && req.Banned {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "cannot ban your own account"))
		return
	}
	u, err := h.users.AdminSetUserBan(c.Request.Context(), c.Param("id"), req.Banned, req.Reason)
	if errors.Is(err, repo.ErrUserNotFound) {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "user not found"))
		return
	}
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	action := "user_unban"
	if req.Banned {
		action = "user_ban"
	}
	h.logAdminAudit(c, model.AdminAuditCategoryPermission, action, "user", u.ID, u.DisplayName, u.ID, u.DisplayName, strings.TrimSpace(req.Reason))
	c.JSON(http.StatusOK, gin.H{"user": u.Public()})
}

type adminAdjustCoinsReq struct {
	Action string `json:"action" binding:"required"`
	Amount int64  `json:"amount" binding:"required"`
	Note   string `json:"note"`
}

func (h *AdminHandler) AdjustUserCoins(c *gin.Context) {
	var req adminAdjustCoinsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid coin adjustment"))
		return
	}
	action := strings.TrimSpace(req.Action)
	if action != "add" && action != "deduct" && action != "freeze" && action != "unfreeze" {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid coin action"))
		return
	}
	u, coinTx, err := h.users.AdminAdjustCoins(c.Request.Context(), c.Param("id"), action, req.Amount, req.Note, UserIDFromCtx(c))
	if errors.Is(err, repo.ErrUserNotFound) {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "user not found"))
		return
	}
	if errors.Is(err, repo.ErrInsufficientCoins) {
		errcode.Respond(c, errcode.New(http.StatusConflict, "insufficient available coins").WithReason("insufficient_coins"))
		return
	}
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	note := strings.TrimSpace(req.Note)
	if note == "" {
		note = "amount=" + strconv.FormatInt(req.Amount, 10)
	} else {
		note = note + "; amount=" + strconv.FormatInt(req.Amount, 10)
	}
	h.logAdminAudit(c, model.AdminAuditCategorySystem, "coins_"+action, "user", u.ID, u.DisplayName, u.ID, u.DisplayName, note)
	c.JSON(http.StatusOK, gin.H{"user": u.Public(), "transaction": coinTx})
}

func (h *AdminHandler) ListCreatorApplications(c *gin.Context) {
	items, err := h.users.ListCreatorApplications(c.Request.Context())
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *AdminHandler) ListPlatformApplications(c *gin.Context) {
	items, err := h.users.ListPlatformApplications(c.Request.Context())
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

func (h *AdminHandler) ApprovePlatformApplication(c *gin.Context) {
	h.reviewPlatformApplication(c, model.PlatformVerificationApproved)
}

func (h *AdminHandler) RejectPlatformApplication(c *gin.Context) {
	h.reviewPlatformApplication(c, model.PlatformVerificationRejected)
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
	h.logAdminAudit(c, model.AdminAuditCategoryPermission, "creator_application_"+status, "creator_application", app.ID, user.DisplayName, user.ID, user.DisplayName, rejectReason)
	c.JSON(http.StatusOK, gin.H{
		"application": app,
		"user":        user.Public(),
	})
}

func (h *AdminHandler) reviewPlatformApplication(c *gin.Context, status string) {
	reviewerID := UserIDFromCtx(c)
	rejectReason := ""
	if status == model.PlatformVerificationRejected {
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
	app, user, err := h.users.ReviewPlatformApplication(c.Request.Context(), c.Param("id"), reviewerID, status, rejectReason)
	if errors.Is(err, repo.ErrPlatformApplicationNotFound) {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "platform application not found"))
		return
	}
	if errors.Is(err, repo.ErrPlatformApplicationAlreadyReviewed) {
		errcode.Respond(c, errcode.New(http.StatusConflict, "platform application already reviewed"))
		return
	}
	if errors.Is(err, repo.ErrLivePermissionRequired) {
		errcode.Respond(c, errcode.New(http.StatusConflict, "live permission must be approved first"))
		return
	}
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	h.logAdminAudit(c, model.AdminAuditCategoryPermission, "platform_application_"+status, "platform_application", app.ID, user.DisplayName, user.ID, user.DisplayName, rejectReason)
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
	h.logAdminAudit(c, model.AdminAuditCategoryPermission, "live_permission_"+status, "user", user.ID, user.DisplayName, user.ID, user.DisplayName, "live permission="+status)
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
		Email:                strings.ToLower(username) + "@gmail.com",
		DisplayName:          displayName,
		PasswordHash:         hash,
		Avatar:               "https://api.dicebear.com/7.x/avataaars/svg?seed=" + url.QueryEscape(displayName),
		CoinBalance:          1200,
		Verified:             false,
		Role:                 model.RoleAdmin,
		LivePermissionStatus: model.LivePermissionApproved,
	}
	if err := h.users.Create(c.Request.Context(), u); err != nil {
		errcode.Respond(c, err)
		return
	}
	h.logAdminAudit(c, model.AdminAuditCategoryPermission, "admin_create", "user", u.ID, u.DisplayName, u.ID, u.DisplayName, "created admin account")
	c.JSON(http.StatusCreated, gin.H{"user": u.Public()})
}

func (h *AdminHandler) logAdminAudit(c *gin.Context, category, action, targetType, targetID, targetTitle, targetUserID, targetUserName, note string) {
	if h == nil || h.users == nil {
		return
	}
	actorID := UserIDFromCtx(c)
	if strings.TrimSpace(actorID) == "" {
		return
	}
	_ = h.users.CreateAdminAuditLog(c.Request.Context(), &model.AdminAuditLog{
		ID:             uuid.NewString(),
		Category:       strings.TrimSpace(category),
		Action:         strings.TrimSpace(action),
		ActorID:        actorID,
		TargetType:     strings.TrimSpace(targetType),
		TargetID:       strings.TrimSpace(targetID),
		TargetTitle:    trimAuditText(targetTitle, 240),
		TargetUserID:   strings.TrimSpace(targetUserID),
		TargetUserName: trimAuditText(targetUserName, 128),
		Note:           strings.TrimSpace(note),
		CreatedAt:      timeNowUTC(),
	})
}

func trimAuditText(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func timeNowUTC() time.Time {
	return time.Now().UTC()
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

func adminPageSize(c *gin.Context, defaultPage, defaultSize int) (int, int) {
	page, err := strconv.Atoi(c.DefaultQuery("page", strconv.Itoa(defaultPage)))
	if err != nil || page < 1 {
		page = defaultPage
	}
	size, err := strconv.Atoi(c.DefaultQuery("size", strconv.Itoa(defaultSize)))
	if err != nil || size < 1 {
		size = defaultSize
	}
	if size > 100 {
		size = 100
	}
	return page, size
}
