package handler

import (
	"context"
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
	users      *repo.UserRepo
	emailCodes *service.EmailCodeService
	stripe     *service.StripeService
}

const usernameChangeCooldown = 7 * 24 * time.Hour
const minTopupCoins int64 = 10

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{2,31}$`)

func NewUserHandler(users *repo.UserRepo, emailCodes *service.EmailCodeService, stripeSvc *service.StripeService) *UserHandler {
	return &UserHandler{users: users, emailCodes: emailCodes, stripe: stripeSvc}
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
	pu := u.Public()
	pu.Email = ""
	pu.EmailVerified = false
	c.JSON(http.StatusOK, pu)
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

func (h *UserHandler) SendEmailChangeCode(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	if h.emailCodes == nil {
		errcode.Respond(c, service.ErrEmailNotConfigured)
		return
	}
	u, err := h.users.FindByID(c.Request.Context(), uid)
	if err != nil {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	oldEmail, ok := service.NormalizeEmail(u.EmailAddress())
	if !ok {
		errcode.Respond(c, service.ErrInvalidRegister.WithReason("invalid_email"))
		return
	}
	resp, err := h.emailCodes.Send(c.Request.Context(), service.EmailPurposeEmailChange, oldEmail)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

type updateEmailReq struct {
	Email     string `json:"email" binding:"required"`
	EmailCode string `json:"emailCode" binding:"required"`
}

func (h *UserHandler) UpdateEmail(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	var req updateEmailReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, service.ErrInvalidRegister.WithReason("invalid_email"))
		return
	}
	cleanEmail, ok := service.NormalizeEmail(req.Email)
	if !ok {
		errcode.Respond(c, service.ErrInvalidRegister.WithReason("invalid_email"))
		return
	}
	if h.emailCodes == nil {
		errcode.Respond(c, service.ErrEmailNotConfigured)
		return
	}
	current, err := h.users.FindByID(c.Request.Context(), uid)
	if err != nil {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	oldEmail, ok := service.NormalizeEmail(current.EmailAddress())
	if !ok {
		errcode.Respond(c, service.ErrInvalidRegister.WithReason("invalid_email"))
		return
	}
	if err := h.emailCodes.Verify(c.Request.Context(), service.EmailPurposeEmailChange, oldEmail, req.EmailCode); err != nil {
		errcode.Respond(c, err)
		return
	}
	u, err := h.users.UpdateEmail(c.Request.Context(), uid, cleanEmail)
	if err != nil {
		if errors.Is(err, repo.ErrEmailTaken) {
			errcode.Respond(c, service.ErrEmailTaken)
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

type topupCheckoutResp struct {
	CheckoutURL          string `json:"checkoutUrl"`
	SessionID            string `json:"sessionId"`
	Amount               int64  `json:"amount"`
	Currency             string `json:"currency"`
	CoinsPerCurrencyUnit int64  `json:"coinsPerCurrencyUnit"`
	PublishableKey       string `json:"publishableKey,omitempty"`
}

// TopupCoins starts a Stripe Checkout flow. Coins are credited only after the
// return handler verifies the Checkout Session with Stripe.
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
	if req.Amount < minTopupCoins {
		c.JSON(http.StatusBadRequest, gin.H{"message": "minimum top-up is 10 coins"})
		return
	}
	if req.Amount > service.MaxTopupCoins {
		c.JSON(http.StatusBadRequest, gin.H{"message": fmt.Sprintf("maximum top-up is %d coins", service.MaxTopupCoins)})
		return
	}
	if h.stripe == nil || !h.stripe.Configured() {
		errcode.Respond(c, errcode.New(http.StatusServiceUnavailable, "stripe is not configured").WithReason("stripe_not_configured"))
		return
	}
	u, err := h.users.FindByID(c.Request.Context(), uid)
	if err != nil {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	origin := requestOrigin(c)
	checkout, err := h.stripe.CreateTopupCheckout(
		c.Request.Context(),
		uid,
		u.EmailAddress(),
		req.Amount,
		origin+"/coins?stripe_topup=success&session_id={CHECKOUT_SESSION_ID}",
		origin+"/coins?stripe_topup=cancelled",
	)
	if err != nil {
		if errors.Is(err, service.ErrStripeNotConfigured) {
			errcode.Respond(c, errcode.New(http.StatusServiceUnavailable, "stripe is not configured").WithReason("stripe_not_configured"))
			return
		}
		errcode.Respond(c, errcode.New(http.StatusBadGateway, "could not create stripe checkout session").WithReason("stripe_checkout_failed"))
		return
	}
	c.JSON(http.StatusOK, topupCheckoutResp{
		CheckoutURL:          checkout.URL,
		SessionID:            checkout.ID,
		Amount:               req.Amount,
		Currency:             h.stripe.Currency(),
		CoinsPerCurrencyUnit: h.stripe.CoinsPerCurrencyUnit(),
		PublishableKey:       h.stripe.PublishableKey(),
	})
}

type confirmTopupReq struct {
	SessionID string `json:"sessionId"`
}

func (h *UserHandler) ConfirmTopupCoins(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	if h.stripe == nil || !h.stripe.Configured() {
		errcode.Respond(c, errcode.New(http.StatusServiceUnavailable, "stripe is not configured").WithReason("stripe_not_configured"))
		return
	}
	var req confirmTopupReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.SessionID) == "" {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "missing stripe checkout session"))
		return
	}
	sess, err := h.stripe.RetrieveCheckoutSession(c.Request.Context(), req.SessionID)
	if err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadGateway, "could not verify stripe checkout session").WithReason("stripe_checkout_verify_failed"))
		return
	}
	if sess == nil || sess.ClientReferenceID != uid || sess.Metadata["user_id"] != uid {
		errcode.Respond(c, errcode.New(http.StatusForbidden, "stripe checkout session does not belong to this user").WithReason("stripe_session_user_mismatch"))
		return
	}
	if sess.PaymentStatus != "paid" {
		c.JSON(http.StatusConflict, gin.H{
			"message":       "stripe checkout session is not paid",
			"reason":        "stripe_payment_not_paid",
			"paymentStatus": sess.PaymentStatus,
		})
		return
	}
	amount, err := h.stripe.TopupCoinsFromSession(sess)
	if err != nil || amount < minTopupCoins {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid stripe checkout metadata").WithReason("stripe_invalid_metadata"))
		return
	}
	u, tx, credited, err := h.users.CreditStripeTopupIfNeeded(
		c.Request.Context(),
		uid,
		amount,
		sess.ID,
		sess.Metadata["currency"],
	)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user":          u.Public(),
		"transaction":   tx,
		"credited":      credited,
		"paymentStatus": sess.PaymentStatus,
	})
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

func (h *UserHandler) WithdrawCoins(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, service.ErrUnauthorized)
		return
	}
	errcode.Respond(c, errcode.New(http.StatusNotImplemented, "withdrawals are not implemented yet").WithReason("withdrawal_not_implemented"))
}

func requestOrigin(c *gin.Context) string {
	proto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if proto == "" {
		if c.Request != nil && c.Request.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	if i := strings.Index(proto, ","); i >= 0 {
		proto = strings.TrimSpace(proto[:i])
	}
	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" && c.Request != nil {
		host = strings.TrimSpace(c.Request.Host)
	}
	if i := strings.Index(host, ","); i >= 0 {
		host = strings.TrimSpace(host[:i])
	}
	if host == "" {
		host = "localhost"
	}
	return proto + "://" + host
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

func beijingDailyTaskDate(now time.Time) string {
	return now.UTC().Add(8 * time.Hour).Format("2006-01-02")
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

	now := time.Now()
	today := beijingDailyTaskDate(now)
	if err := h.ensureDailyCoinTaskComplete(c.Request.Context(), uid, task.ID, today); err != nil {
		errcode.Respond(c, err)
		return
	}

	reward := task.RewardMin
	if task.RewardMax > task.RewardMin {
		reward += now.UnixNano() % (task.RewardMax - task.RewardMin + 1)
	}
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

func (h *UserHandler) ensureDailyCoinTaskComplete(ctx context.Context, userID, taskID, today string) error {
	switch taskID {
	case "daily-login-lottery":
		return nil
	case "watch-3-lives", "watch-30-minutes":
		stats, err := h.users.DailyWatchTaskStats(ctx, userID, today)
		if err != nil {
			return err
		}
		if taskID == "watch-3-lives" && stats.Rooms >= 3 {
			return nil
		}
		if taskID == "watch-30-minutes" && stats.WatchSeconds >= 30*60 {
			return nil
		}
		return errcode.New(http.StatusConflict, "daily task is not complete").WithReason("daily_task_incomplete")
	default:
		return errcode.New(http.StatusNotFound, "daily task not found")
	}
}
