package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/gift-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type BetHandler struct {
	svc *service.BetService
}

func NewBetHandler(s *service.BetService) *BetHandler {
	return &BetHandler{svc: s}
}

type openBetBody struct {
	RoomID   string `json:"roomId"`
	Amount   int64  `json:"amount"`
	Question string `json:"question"`
}

type wagerBetBody struct {
	RoomID string `json:"roomId"`
	Option string `json:"option"`
}

type settleBetBody struct {
	Option string `json:"option"`
}

func (h *BetHandler) Latest(c *gin.Context) {
	roomID := strings.TrimSpace(c.Query("roomId"))
	if roomID == "" {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "roomId required"))
		return
	}
	view, err := h.svc.Latest(c.Request.Context(), roomID, optionalUserID(c))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	if view == nil {
		c.JSON(http.StatusOK, gin.H{"round": nil, "summary": []any{}})
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h *BetHandler) Open(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(http.StatusUnauthorized, "Unauthorized"))
		return
	}
	var body openBetBody
	if err := c.ShouldBindJSON(&body); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad request"))
		return
	}
	roomID := strings.TrimSpace(body.RoomID)
	question := strings.TrimSpace(body.Question)
	if roomID == "" || body.Amount <= 0 || question == "" || len([]rune(question)) > service.MaxBetQuestionRunes {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad request"))
		return
	}
	view, err := h.svc.Open(c.Request.Context(), uid, roomID, body.Amount, question)
	if err != nil {
		respondBetError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h *BetHandler) Wager(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(http.StatusUnauthorized, "Unauthorized"))
		return
	}
	roundID := strings.TrimSpace(c.Param("id"))
	var body wagerBetBody
	if err := c.ShouldBindJSON(&body); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad request"))
		return
	}
	if roundID == "" || strings.TrimSpace(body.RoomID) == "" {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad request"))
		return
	}
	wager, err := h.svc.Wager(c.Request.Context(), uid, strings.TrimSpace(body.RoomID), roundID, body.Option)
	if err != nil {
		respondBetError(c, err)
		return
	}
	c.JSON(http.StatusOK, wager)
}

func (h *BetHandler) Settle(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(http.StatusUnauthorized, "Unauthorized"))
		return
	}
	roundID := strings.TrimSpace(c.Param("id"))
	var body settleBetBody
	if err := c.ShouldBindJSON(&body); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad request"))
		return
	}
	view, err := h.svc.Settle(c.Request.Context(), uid, roundID, body.Option)
	if err != nil {
		respondBetError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h *BetHandler) Cancel(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(http.StatusUnauthorized, "Unauthorized"))
		return
	}
	roundID := strings.TrimSpace(c.Param("id"))
	view, err := h.svc.Cancel(c.Request.Context(), uid, roundID)
	if err != nil {
		respondBetError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func optionalUserID(c *gin.Context) string {
	return UserIDFromCtx(c)
}

func respondBetError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInsufficientCoin):
		errcode.Respond(c, errcode.ErrInsufficientCoin)
	case errors.Is(err, service.ErrBetUnauthorized):
		errcode.Respond(c, errcode.New(http.StatusForbidden, "Forbidden").WithReason("forbidden"))
	case errors.Is(err, service.ErrBetActive):
		errcode.Respond(c, errcode.New(http.StatusConflict, "A bet is already active").WithReason("active_bet_exists"))
	case errors.Is(err, service.ErrBetClosed):
		errcode.Respond(c, errcode.New(http.StatusConflict, "Bet is closed").WithReason("bet_closed"))
	case errors.Is(err, service.ErrBetAlready):
		errcode.Respond(c, errcode.New(http.StatusConflict, "Bet already placed").WithReason("bet_already_placed"))
	case errors.Is(err, service.ErrBetNoWinners):
		errcode.Respond(c, errcode.New(http.StatusConflict, "No winners for this result").WithReason("bet_no_winners"))
	case errors.Is(err, service.ErrBetNotFound):
		errcode.Respond(c, errcode.New(http.StatusNotFound, "Bet not found").WithReason("bet_not_found"))
	case errors.Is(err, service.ErrBetBadOption):
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad bet option").WithReason("bad_bet_option"))
	case errors.Is(err, service.ErrBetBadQuestion):
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad bet question").WithReason("bad_bet_question"))
	default:
		errcode.Respond(c, err)
	}
}
