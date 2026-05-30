package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/gift-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type LuckyBagHandler struct {
	svc *service.LuckyBagService
}

func NewLuckyBagHandler(s *service.LuckyBagService) *LuckyBagHandler {
	return &LuckyBagHandler{svc: s}
}

func (h *LuckyBagHandler) Latest(c *gin.Context) {
	roomID := strings.TrimSpace(c.Query("roomId"))
	if roomID == "" {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "roomId required"))
		return
	}
	view, err := h.svc.Latest(c.Request.Context(), roomID, UserIDFromCtx(c))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	if view == nil {
		c.JSON(http.StatusOK, gin.H{"bag": nil, "participantCount": 0})
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h *LuckyBagHandler) Open(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(http.StatusUnauthorized, "Unauthorized"))
		return
	}
	var req service.OpenLuckyBagReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad request"))
		return
	}
	view, err := h.svc.Open(c.Request.Context(), uid, req)
	if err != nil {
		respondLuckyBagError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h *LuckyBagHandler) Join(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(http.StatusUnauthorized, "Unauthorized"))
		return
	}
	bagID := strings.TrimSpace(c.Param("id"))
	var body struct {
		RoomID string `json:"roomId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad request"))
		return
	}
	if bagID == "" || strings.TrimSpace(body.RoomID) == "" {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad request"))
		return
	}
	view, err := h.svc.Join(c.Request.Context(), uid, strings.TrimSpace(body.RoomID), bagID)
	if err != nil {
		respondLuckyBagError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h *LuckyBagHandler) Cancel(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(http.StatusUnauthorized, "Unauthorized"))
		return
	}
	bagID := strings.TrimSpace(c.Param("id"))
	view, err := h.svc.Cancel(c.Request.Context(), uid, bagID)
	if err != nil {
		respondLuckyBagError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func respondLuckyBagError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInsufficientCoin):
		errcode.Respond(c, errcode.ErrInsufficientCoin)
	case errors.Is(err, service.ErrLuckyBagUnauthorized):
		errcode.Respond(c, errcode.New(http.StatusForbidden, "Forbidden").WithReason("forbidden"))
	case errors.Is(err, service.ErrLuckyBagActive):
		errcode.Respond(c, errcode.New(http.StatusConflict, "A lucky bag is already active").WithReason("active_lucky_bag_exists"))
	case errors.Is(err, service.ErrLuckyBagClosed):
		errcode.Respond(c, errcode.New(http.StatusConflict, "Lucky bag is closed").WithReason("lucky_bag_closed"))
	case errors.Is(err, service.ErrLuckyBagAlreadyJoin):
		errcode.Respond(c, errcode.New(http.StatusConflict, "Already joined").WithReason("lucky_bag_already_joined"))
	case errors.Is(err, service.ErrLuckyBagNotEligible):
		errcode.Respond(c, errcode.New(http.StatusForbidden, "Not eligible").WithReason("lucky_bag_not_eligible"))
	case errors.Is(err, service.ErrLuckyBagNotFound):
		errcode.Respond(c, errcode.New(http.StatusNotFound, "Lucky bag not found").WithReason("lucky_bag_not_found"))
	case errors.Is(err, service.ErrLuckyBagBad):
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad lucky bag").WithReason("bad_lucky_bag"))
	default:
		errcode.Respond(c, err)
	}
}
