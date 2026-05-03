package handler

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"github.com/qingwenwen777/golive/app/user-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type UserHandler struct {
	users *repo.UserRepo
}

const usernameChangeCooldown = 7 * 24 * time.Hour

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{2,31}$`)

func NewUserHandler(users *repo.UserRepo) *UserHandler {
	return &UserHandler{users: users}
}

// Me returns the authenticated user's profile. AuthRequired middleware has
// already validated the JWT and stashed the userId in the context.
func (h *UserHandler) Me(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	u, err := h.users.FindByID(c.Request.Context(), uid)
	if err != nil {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	c.JSON(http.StatusOK, u.Public())
}

// PublicProfile returns a public user profile by id, with username as a
// compatibility fallback for channel links that may carry a handle.
func (h *UserHandler) PublicProfile(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "missing user id"))
		return
	}

	u, err := h.users.FindByID(c.Request.Context(), id)
	if errors.Is(err, repo.ErrUserNotFound) {
		u, err = h.users.FindByUsername(c.Request.Context(), id)
	}
	if errors.Is(err, repo.ErrUserNotFound) {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "user not found"))
		return
	}
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, u.Public())
}

type updateProfileReq struct {
	Username    *string `json:"username"`
	DisplayName *string `json:"displayName"`
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	var req updateProfileReq
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

	u, err := h.users.UpdateProfile(c.Request.Context(), uid, username, displayName, time.Now().UTC(), usernameChangeCooldown)
	if err != nil {
		var cooldown *repo.UsernameCooldownError
		if errors.As(err, &cooldown) {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{
				"message":     "Username can be changed once every 7 days",
				"reason":      "username_cooldown",
				"availableAt": cooldown.AvailableAt.UTC().Format(time.RFC3339),
			})
			return
		}
		if errors.Is(err, repo.ErrUsernameTaken) {
			errcode.Respond(c, service.ErrUsernameTaken.WithReason("username_taken"))
			return
		}
		if errors.Is(err, repo.ErrUserNotFound) {
			errcode.Respond(c, service.ErrUnauthorized)
			return
		}
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, u.Public())
}

type changePasswordReq struct {
	CurrentPassword string `json:"currentPassword" binding:"required"`
	NewPassword     string `json:"newPassword" binding:"required"`
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	var req changePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid body"))
		return
	}
	if err := service.ValidatePasswordPolicy(req.NewPassword); err != nil {
		errcode.Respond(c, err)
		return
	}
	u, err := h.users.FindByID(c.Request.Context(), uid)
	if err != nil {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		errcode.Respond(c, service.ErrInvalidCredentials.WithReason("invalid_current_password"))
		return
	}
	hash, err := service.HashPassword(req.NewPassword)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	if err := h.users.UpdatePasswordHash(c.Request.Context(), uid, hash); err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func cleanUsernamePatch(raw *string, c *gin.Context) (*string, bool) {
	if raw == nil {
		return nil, true
	}
	username := strings.TrimSpace(*raw)
	if !usernamePattern.MatchString(username) {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "username must be 3-32 letters, numbers, dots, hyphens, or underscores").WithReason("invalid_username"))
		return nil, false
	}
	return &username, true
}

func cleanDisplayNamePatch(raw *string, c *gin.Context) (*string, bool) {
	if raw == nil {
		return nil, true
	}
	displayName := trimRunes(strings.TrimSpace(*raw), 64)
	if displayName == "" {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "display name is required").WithReason("invalid_display_name"))
		return nil, false
	}
	return &displayName, true
}

func trimRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

type topupReq struct {
	Amount int64 `json:"amount"`
}

// TopupCoins increments the authenticated user's coin balance by the given
// amount and returns the updated public user. This is a stub for development;
// real billing integration is out of scope.
func (h *UserHandler) TopupCoins(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	var req topupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid amount"})
		return
	}
	if req.Amount < 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "minimum top-up is 1,000 coins"})
		return
	}
	u, _, err := h.users.IncrementCoinsWithTransaction(
		c.Request.Context(),
		uid,
		req.Amount,
		model.CoinTxTopup,
		"充值获得",
		"模拟充值成功，后续接入真实支付接口。",
		"topup",
		"",
		"",
		"",
	)
	if err != nil {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	c.JSON(http.StatusOK, u.Public())
}

func (h *UserHandler) CoinTransactions(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "80"))
	rows, err := h.users.ListCoinTransactions(c.Request.Context(), uid, limit)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

type dailyCoinTask struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	RewardMin   int64  `json:"rewardMin"`
	RewardMax   int64  `json:"rewardMax"`
}

type dailyTaskClaimResp struct {
	User           model.PublicUser      `json:"user"`
	Transaction    model.CoinTransaction `json:"transaction"`
	Task           dailyCoinTask         `json:"task"`
	Created        bool                  `json:"created"`
	AlreadyClaimed bool                  `json:"alreadyClaimed"`
}

var dailyCoinTasks = map[string]dailyCoinTask{
	"daily-login-lottery": {
		ID:          "daily-login-lottery",
		Title:       "每日登录抽奖",
		Description: "每天登录可抽一次小额 coins。",
		RewardMin:   6,
		RewardMax:   18,
	},
	"watch-3-lives": {
		ID:          "watch-3-lives",
		Title:       "观看 3 个直播间",
		Description: "当天打开 3 个不同直播间后领取。",
		RewardMin:   18,
		RewardMax:   18,
	},
	"watch-30-minutes": {
		ID:          "watch-30-minutes",
		Title:       "观看满 30 分钟",
		Description: "当天累计观看时长达到 30 分钟后领取。",
		RewardMin:   25,
		RewardMax:   25,
	},
}

func (h *UserHandler) ClaimDailyCoinTask(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	task, ok := dailyCoinTasks[c.Param("taskID")]
	if !ok {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "daily task not found"))
		return
	}

	reward := task.RewardMin
	if task.RewardMax > task.RewardMin {
		reward += time.Now().UnixNano() % (task.RewardMax - task.RewardMin + 1)
	}
	today := time.Now().Local().Format("2006-01-02")
	sourceID := fmt.Sprintf("%s:%s", task.ID, today)
	u, tx, created, err := h.users.ClaimDailyCoinReward(
		c.Request.Context(),
		uid,
		sourceID,
		task.Title,
		task.Description,
		reward,
	)
	if err != nil {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	c.JSON(http.StatusOK, dailyTaskClaimResp{
		User:           u.Public(),
		Transaction:    *tx,
		Task:           task,
		Created:        created,
		AlreadyClaimed: !created,
	})
}
