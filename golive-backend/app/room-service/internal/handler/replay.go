package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type ReplayHandler struct {
	svc *service.ReplayService
}

func NewReplayHandler(svc *service.ReplayService) *ReplayHandler {
	return &ReplayHandler{svc: svc}
}

func (h *ReplayHandler) UpdateActiveSettings(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	var req service.UpdateLiveReplaySettingsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid body"))
		return
	}
	replay, err := h.svc.UpdateActiveSettings(c.Request.Context(), uid, req)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, replay)
}

func (h *ReplayHandler) ListMine(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	page, size := pageQuery(c, 1, 24)
	resp, err := h.svc.ListMine(c.Request.Context(), uid, page, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ReplayHandler) Update(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	var req service.UpdateReplayReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid body"))
		return
	}
	replay, err := h.svc.UpdateReplay(c.Request.Context(), uid, c.Param("id"), req)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, replay)
}

func (h *ReplayHandler) Delete(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	if err := h.svc.DeleteReplay(c.Request.Context(), uid, c.Param("id")); err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
