package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type ModerationHandler struct {
	svc *service.ModerationService
}

func NewModerationHandler(svc *service.ModerationService) *ModerationHandler {
	return &ModerationHandler{svc: svc}
}

func (h *ModerationHandler) ListFollowers(c *gin.Context) {
	uid := UserIDFromCtx(c)
	page, size := pageSize(c, 1, 10)
	resp, err := h.svc.ListFollowers(c.Request.Context(), uid, c.Query("q"), page, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ModerationHandler) ListModerators(c *gin.Context) {
	uid := UserIDFromCtx(c)
	page, size := pageSize(c, 1, 20)
	resp, err := h.svc.ListModerators(c.Request.Context(), uid, page, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ModerationHandler) AddModerator(c *gin.Context) {
	uid := UserIDFromCtx(c)
	resp, err := h.svc.AddModerator(c.Request.Context(), uid, c.Param("userID"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ModerationHandler) RemoveModerator(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if err := h.svc.RemoveModerator(c.Request.Context(), uid, c.Param("userID")); err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *ModerationHandler) Logs(c *gin.Context) {
	uid := UserIDFromCtx(c)
	page, size := pageSize(c, 1, 10)
	resp, err := h.svc.Logs(c.Request.Context(), uid, page, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ModerationHandler) RoomState(c *gin.Context) {
	resp, err := h.svc.RoomState(c.Request.Context(), c.Param("id"), UserIDFromCtx(c))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ModerationHandler) MuteState(c *gin.Context) {
	resp, err := h.svc.MuteState(c.Request.Context(), c.Param("id"), UserIDFromCtx(c), c.Param("userID"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ModerationHandler) Mute(c *gin.Context) {
	var req service.MuteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "bad request"))
		return
	}
	resp, err := h.svc.Mute(c.Request.Context(), c.Param("id"), UserIDFromCtx(c), req)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ModerationHandler) Unmute(c *gin.Context) {
	resp, err := h.svc.Unmute(c.Request.Context(), c.Param("id"), UserIDFromCtx(c), c.Param("userID"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}
