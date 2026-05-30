package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type ReplayCommentHandler struct {
	svc *service.ReplayCommentService
}

func NewReplayCommentHandler(svc *service.ReplayCommentService) *ReplayCommentHandler {
	return &ReplayCommentHandler{svc: svc}
}

func (h *ReplayCommentHandler) List(c *gin.Context) {
	resp, err := h.svc.List(c.Request.Context(), UserIDFromCtx(c), c.Param("id"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ReplayCommentHandler) Create(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	var req service.CreateCommentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid body"))
		return
	}
	comment, err := h.svc.Create(c.Request.Context(), uid, c.Param("id"), req)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, comment)
}

func (h *ReplayCommentHandler) Delete(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	if err := h.svc.Delete(c.Request.Context(), uid, c.Param("id"), c.Param("commentID")); err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *ReplayCommentHandler) Like(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	state, err := h.svc.Like(c.Request.Context(), uid, c.Param("id"), c.Param("commentID"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}

func (h *ReplayCommentHandler) Unlike(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.ErrUnauthorized)
		return
	}
	state, err := h.svc.Unlike(c.Request.Context(), uid, c.Param("id"), c.Param("commentID"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, state)
}
