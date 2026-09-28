package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"github.com/qingwenwen777/golive/app/user-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
	"github.com/qingwenwen777/golive/pkg/logger"
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

// SessionRevoker revokes all refresh tokens of a user.
type SessionRevoker interface {
	RevokeUserSessions(ctx context.Context, userID string) error
}

// LiveEnder ends a user's live rooms. Rooms belong to room-service, which a
// ban asks through POST /internal/users/:id/end-live.
type LiveEnder interface {
	EndUserLive(ctx context.Context, userID string) (int, error)
}

type AdminHandler struct {
	users    *repo.UserRepo
	sessions SessionRevoker
	lives    LiveEnder
}

func NewAdminHandler(users *repo.UserRepo, sessions SessionRevoker, lives LiveEnder) *AdminHandler {
	return &AdminHandler{users: users, sessions: sessions, lives: lives}
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
	page, size := adminPageSize(c, 1, 20)
	items, total, stats, err := h.users.ListInviteCodesPage(c.Request.Context(), page, size)
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
	if conflict := service.NameConflict(err); conflict != nil {
		errcode.Respond(c, conflict)
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

type adminUpdateUserEmailReq struct {
	Email string `json:"email" binding:"required"`
}

func (h *AdminHandler) UpdateUserEmail(c *gin.Context) {
	var req adminUpdateUserEmailReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, service.ErrInvalidRegister.WithReason("invalid_email"))
		return
	}
	email, ok := service.NormalizeEmail(req.Email)
	if !ok {
		errcode.Respond(c, service.ErrInvalidRegister.WithReason("invalid_email"))
		return
	}
	u, err := h.users.UpdateEmail(c.Request.Context(), c.Param("id"), email)
	if errors.Is(err, repo.ErrEmailTaken) {
		errcode.Respond(c, service.ErrEmailTaken)
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
	h.logAdminAudit(c, model.AdminAuditCategoryPermission, "user_email_update", "user", u.ID, u.DisplayName, u.ID, u.DisplayName, "updated user email")
	c.JSON(http.StatusOK, gin.H{"user": u.Public()})
}

type adminUpdateUserPasswordReq struct {
	Password string `json:"password" binding:"required"`
}

func (h *AdminHandler) UpdateUserPassword(c *gin.Context) {
	var req adminUpdateUserPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, service.ErrInvalidPassword)
		return
	}
	if err := service.ValidatePasswordPolicy(req.Password); err != nil {
		errcode.Respond(c, err)
		return
	}
	u, err := h.users.FindByID(c.Request.Context(), c.Param("id"))
	if errors.Is(err, repo.ErrUserNotFound) {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "user not found"))
		return
	}
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	hash, err := service.HashPassword(req.Password)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	if err := h.users.UpdatePasswordHash(c.Request.Context(), u.ID, hash); err != nil {
		errcode.Respond(c, err)
		return
	}
	h.logAdminAudit(c, model.AdminAuditCategoryPermission, "user_password_update", "user", u.ID, u.DisplayName, u.ID, u.DisplayName, "updated user password")
	c.JSON(http.StatusOK, gin.H{"ok": true})
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
	u, warning, err := setUserBan(c.Request.Context(), h.users, h.sessions, h.lives, c.Param("id"), req.Banned, req.Reason, UserIDFromCtx(c))
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
	resp := gin.H{"user": u.Public()}
	if warning != nil {
		resp["warning"] = warning
	}
	c.JSON(http.StatusOK, resp)
}

// banWarning is added to a successful ban response when a follow-up step
// failed; the ban itself stands.
type banWarning struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

var liveNotEndedWarning = banWarning{
	Reason:  "live_end_failed",
	Message: "User banned, but their live stream could not be ended right away. It will be ended automatically within a few minutes.",
}

// setUserBan is the single ban path for the admin API and the internal API
// other services call: it updates the ban state and, on a ban, ends the
// user's live rooms and revokes every refresh token so existing sessions
// must log in again, which hands the client the banned flag and routes it to
// the appeal page. The warning is set when the ban stands but the live rooms
// could not be ended right away.
func setUserBan(ctx context.Context, users *repo.UserRepo, sessions SessionRevoker, lives LiveEnder, userID string, banned bool, reason, operatorID string) (*model.User, *banWarning, error) {
	u, err := users.AdminSetUserBan(ctx, userID, banned, reason, operatorID)
	if err != nil {
		return nil, nil, err
	}
	if !banned {
		return u, nil, nil
	}
	warning := endBannedUserLive(ctx, lives, u.ID)
	if sessions != nil {
		if err := sessions.RevokeUserSessions(ctx, u.ID); err != nil {
			return nil, nil, err
		}
	}
	return u, warning, nil
}

// endBannedUserLive asks room-service to end a banned user's live rooms. It
// never fails the ban: an error is logged and returned as a warning, and
// room-service's live reconciler ends the rooms of banned owners anyway.
func endBannedUserLive(ctx context.Context, lives LiveEnder, userID string) *banWarning {
	if lives == nil {
		return nil
	}
	if _, err := lives.EndUserLive(ctx, userID); err != nil {
		logger.L().Warn("end live rooms of banned user", zap.String("user_id", userID), zap.Error(err))
		warning := liveNotEndedWarning
		return &warning
	}
	return nil
}

type adminReviewUnbanAppealReq struct {
	Status string `json:"status" binding:"required"`
	Note   string `json:"note"`
}

func (h *AdminHandler) ReviewUnbanAppeal(c *gin.Context) {
	var req adminReviewUnbanAppealReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid body"))
		return
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status != model.UnbanAppealApproved && status != model.UnbanAppealRejected && status != model.UnbanAppealReviewing {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid appeal status").WithReason("invalid_appeal_status"))
		return
	}
	appeal, u, err := h.users.AdminReviewUnbanAppeal(c.Request.Context(), c.Param("id"), c.Param("appealID"), UserIDFromCtx(c), status, req.Note)
	if errors.Is(err, repo.ErrUnbanAppealNotFound) {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "appeal not found"))
		return
	}
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	action := "unban_appeal_" + status
	note := strings.TrimSpace(req.Note)
	h.logAdminAudit(c, model.AdminAuditCategoryPermission, action, "unban_appeal", appeal.ID, u.DisplayName, u.ID, u.DisplayName, note)
	c.JSON(http.StatusOK, gin.H{"appeal": appeal, "user": u.Public()})
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
	page, size := adminPageSize(c, 1, 20)
	items, total, stats, err := h.users.ListCreatorApplicationsPage(c.Request.Context(), page, size)
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

func (h *AdminHandler) ListPlatformApplications(c *gin.Context) {
	page, size := adminPageSize(c, 1, 20)
	items, total, stats, err := h.users.ListPlatformApplicationsPage(c.Request.Context(), page, size)
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

func (h *AdminHandler) ListLiveCreators(c *gin.Context) {
	page, size := adminPageSize(c, 1, 20)
	items, total, err := h.users.ListLiveCreatorsPage(c.Request.Context(), page, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items": items,
		"total": total,
		"page":  page,
		"size":  size,
	})
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
		DisplayName:          displayName,
		PasswordHash:         hash,
		CoinBalance:          1200,
		Verified:             false,
		Role:                 model.RoleAdmin,
		LivePermissionStatus: model.LivePermissionApproved,
	}
	if err := h.users.Create(c.Request.Context(), u); err != nil {
		if conflict := service.NameConflict(err); conflict != nil {
			errcode.Respond(c, conflict)
			return
		}
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

// InternalHandler serves /internal/*, the service-to-service API. Routes are
// guarded by the shared internal token (see server.NewRouter).
type InternalHandler struct {
	users    *repo.UserRepo
	sessions SessionRevoker
	lives    LiveEnder
}

func NewInternalHandler(users *repo.UserRepo, sessions SessionRevoker, lives LiveEnder) *InternalHandler {
	return &InternalHandler{users: users, sessions: sessions, lives: lives}
}

const (
	RestrictionBan    = "ban"
	RestrictionUnban  = "unban"
	RestrictionMute   = "mute"
	RestrictionUnmute = "unmute"
)

type internalRestrictionReq struct {
	Action     string     `json:"action"`
	Reason     string     `json:"reason"`
	MutedUntil *time.Time `json:"mutedUntil"`
	OperatorID string     `json:"operatorId"`
}

// SetRestriction serves POST /internal/users/:id/restriction. room-service's
// report moderation calls it instead of writing users /
// user_moderation_states itself; a ban goes through the same path as the
// admin ban, ending the user's live rooms and revoking their refresh tokens.
func (h *InternalHandler) SetRestriction(c *gin.Context) {
	var req internalRestrictionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid body"))
		return
	}
	ctx := c.Request.Context()
	userID := strings.TrimSpace(c.Param("id"))
	var warning *banWarning
	var err error
	switch req.Action {
	case RestrictionBan, RestrictionUnban:
		_, warning, err = setUserBan(ctx, h.users, h.sessions, h.lives, userID, req.Action == RestrictionBan, req.Reason, req.OperatorID)
	case RestrictionMute:
		if req.MutedUntil == nil || !req.MutedUntil.After(time.Now()) {
			errcode.Respond(c, errcode.New(http.StatusBadRequest, "mutedUntil must be in the future"))
			return
		}
		err = h.users.SetUserMute(ctx, userID, req.MutedUntil, req.Reason, req.OperatorID)
	case RestrictionUnmute:
		err = h.users.SetUserMute(ctx, userID, nil, "", req.OperatorID)
	default:
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid action"))
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
	resp := gin.H{"userId": userID, "action": req.Action}
	if warning != nil {
		resp["warning"] = warning
	}
	c.JSON(http.StatusOK, resp)
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
