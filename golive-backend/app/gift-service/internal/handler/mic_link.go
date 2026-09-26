package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/gift-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type MicLinkHandler struct {
	svc *service.MicLinkService
}

func NewMicLinkHandler(s *service.MicLinkService) *MicLinkHandler {
	return &MicLinkHandler{svc: s}
}

func (h *MicLinkHandler) Latest(c *gin.Context) {
	roomID := strings.TrimSpace(c.Query("roomId"))
	if roomID == "" {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "roomId required"))
		return
	}
	view, err := h.svc.Latest(c.Request.Context(), roomID, UserIDFromCtx(c))
	if err != nil {
		respondMicLinkError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h *MicLinkHandler) Config(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(http.StatusUnauthorized, "Unauthorized"))
		return
	}
	var req service.MicConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad request"))
		return
	}
	view, err := h.svc.Config(c.Request.Context(), uid, req)
	if err != nil {
		respondMicLinkError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h *MicLinkHandler) Request(c *gin.Context) {
	h.selfAction(c, h.svc.Request)
}

func (h *MicLinkHandler) Cancel(c *gin.Context) {
	h.selfAction(c, h.svc.Cancel)
}

func (h *MicLinkHandler) Leave(c *gin.Context) {
	h.selfAction(c, h.svc.Leave)
}

func (h *MicLinkHandler) Mute(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(http.StatusUnauthorized, "Unauthorized"))
		return
	}
	var body struct {
		RoomID string `json:"roomId"`
		Muted  bool   `json:"muted"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad request"))
		return
	}
	view, err := h.svc.Mute(c.Request.Context(), uid, strings.TrimSpace(body.RoomID), body.Muted)
	if err != nil {
		respondMicLinkError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h *MicLinkHandler) Approve(c *gin.Context) {
	h.ownerAction(c, h.svc.Approve)
}

func (h *MicLinkHandler) Reject(c *gin.Context) {
	h.ownerAction(c, h.svc.Reject)
}

func (h *MicLinkHandler) Remove(c *gin.Context) {
	h.ownerAction(c, h.svc.Remove)
}

// selfAction handles viewer/guest self-service calls that take only a roomId.
func (h *MicLinkHandler) selfAction(
	c *gin.Context,
	fn func(ctx context.Context, userID, roomID string) (*service.MicLinkView, error),
) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(http.StatusUnauthorized, "Unauthorized"))
		return
	}
	var body struct {
		RoomID string `json:"roomId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad request"))
		return
	}
	roomID := strings.TrimSpace(body.RoomID)
	if roomID == "" {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad request"))
		return
	}
	view, err := fn(c.Request.Context(), uid, roomID)
	if err != nil {
		respondMicLinkError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// ownerAction handles owner moderation calls that take roomId + targetId.
func (h *MicLinkHandler) ownerAction(
	c *gin.Context,
	fn func(ctx context.Context, ownerID, roomID, targetID string) (*service.MicLinkView, error),
) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(http.StatusUnauthorized, "Unauthorized"))
		return
	}
	var body struct {
		RoomID   string `json:"roomId"`
		TargetID string `json:"targetId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "Bad request"))
		return
	}
	view, err := fn(c.Request.Context(), uid, strings.TrimSpace(body.RoomID), strings.TrimSpace(body.TargetID))
	if err != nil {
		respondMicLinkError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func respondMicLinkError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrMicLinkForbidden):
		errcode.Respond(c, errcode.New(http.StatusForbidden, "Forbidden").WithReason("forbidden"))
	case errors.Is(err, service.ErrMicLinkDisabled):
		errcode.Respond(c, errcode.New(http.StatusConflict, "Mic link disabled").WithReason("mic_link_disabled"))
	case errors.Is(err, service.ErrMicLinkNotEligible):
		errcode.Respond(c, errcode.New(http.StatusForbidden, "Not eligible").WithReason("mic_link_not_eligible"))
	case errors.Is(err, service.ErrMicLinkRequestExists):
		errcode.Respond(c, errcode.New(http.StatusConflict, "Request exists").WithReason("mic_link_request_exists"))
	case errors.Is(err, service.ErrMicLinkSlotFull):
		errcode.Respond(c, errcode.New(http.StatusConflict, "Mic is full").WithReason("mic_link_slot_full"))
	case errors.Is(err, service.ErrMicLinkRequestNotFound):
		errcode.Respond(c, errcode.New(http.StatusNotFound, "Request not found").WithReason("mic_link_request_not_found"))
	case errors.Is(err, service.ErrMicLinkBusy):
		errcode.Respond(c, errcode.New(http.StatusConflict, "Try again").WithReason("mic_link_busy"))
	default:
		errcode.Respond(c, err)
	}
}
