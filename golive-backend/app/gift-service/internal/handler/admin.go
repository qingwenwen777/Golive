package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
	"github.com/qingwenwen777/golive/app/gift-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type adminAuthChecker interface {
	IsAdmin(context.Context, string) (bool, error)
}

func AdminRequired(checker adminAuthChecker) gin.HandlerFunc {
	forbidden := errcode.New(http.StatusForbidden, "Admin permission required")
	return func(c *gin.Context) {
		uid := UserIDFromCtx(c)
		if uid == "" {
			errcode.Respond(c, errcode.New(http.StatusUnauthorized, "Unauthorized"))
			return
		}
		ok, err := checker.IsAdmin(c.Request.Context(), uid)
		if err != nil {
			errcode.Respond(c, err)
			return
		}
		if !ok {
			errcode.Respond(c, forbidden)
			return
		}
		c.Next()
	}
}

type AdminHandler struct {
	svc *service.AdminService
}

func NewAdminHandler(svc *service.AdminService) *AdminHandler {
	return &AdminHandler{svc: svc}
}

func (h *AdminHandler) IsAdmin(ctx context.Context, userID string) (bool, error) {
	return h.svc.IsAdmin(ctx, userID)
}

func (h *AdminHandler) Summary(c *gin.Context) {
	out, err := h.svc.EconomySummary(c.Request.Context())
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *AdminHandler) Gifts(c *gin.Context) {
	items, stats, err := h.svc.ListGifts(c.Request.Context())
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "stats": stats})
}

type updateGiftReq struct {
	PriceCoin *int64 `json:"priceCoin"`
	Enabled   *bool  `json:"enabled"`
}

func (h *AdminHandler) UpdateGift(c *gin.Context) {
	var req updateGiftReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad request"))
		return
	}
	gift, err := h.svc.UpdateGift(c.Request.Context(), c.Param("id"), repo.AdminGiftPatch{
		PriceCoin: req.PriceCoin,
		Enabled:   req.Enabled,
	})
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gift)
	case errors.Is(err, repo.ErrGiftNotFound):
		errcode.Respond(c, errcode.New(http.StatusNotFound, "Gift not found"))
	case errors.Is(err, service.ErrInvalidGiftPrice):
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Invalid gift price"))
	default:
		errcode.Respond(c, err)
	}
}

func (h *AdminHandler) Orders(c *gin.Context) {
	items, total, page, size, err := h.svc.ListOrders(c.Request.Context(), repo.AdminOrderFilter{
		Type:   c.Query("type"),
		Status: c.Query("status"),
		Q:      c.Query("q"),
		Page:   queryInt(c, "page", 1),
		Size:   queryInt(c, "size", 20),
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
	})
}

func (h *AdminHandler) Coins(c *gin.Context) {
	items, total, page, size, stats, err := h.svc.CoinLedger(c.Request.Context(), repo.AdminCoinFilter{
		Type: c.Query("type"),
		Q:    c.Query("q"),
		Page: queryInt(c, "page", 1),
		Size: queryInt(c, "size", 20),
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

func (h *AdminHandler) Bets(c *gin.Context) {
	items, total, page, size, stats, err := h.svc.ListBetRounds(c.Request.Context(), repo.AdminBetFilter{
		Status: c.Query("status"),
		Q:      c.Query("q"),
		Page:   queryInt(c, "page", 1),
		Size:   queryInt(c, "size", 20),
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

type settleBetReq struct {
	Option string `json:"option"`
}

func (h *AdminHandler) SettleBet(c *gin.Context) {
	var req settleBetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad request"))
		return
	}
	item, err := h.svc.SettleBetRound(c.Request.Context(), strings.TrimSpace(c.Param("id")), strings.TrimSpace(req.Option))
	if err != nil {
		respondBetError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *AdminHandler) CancelBet(c *gin.Context) {
	item, err := h.svc.CancelBetRound(c.Request.Context(), strings.TrimSpace(c.Param("id")))
	if err != nil {
		respondBetError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *AdminHandler) Reports(c *gin.Context) {
	items, err := h.svc.RevenueReport(
		c.Request.Context(),
		c.DefaultQuery("period", "day"),
		queryInt(c, "limit", 0),
	)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"period": c.DefaultQuery("period", "day"),
		"items":  items,
	})
}

func queryInt(c *gin.Context, key string, fallback int) int {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
