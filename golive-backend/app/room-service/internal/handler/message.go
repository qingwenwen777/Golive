package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type MessageHandler struct {
	svc *service.MessageService
}

func NewMessageHandler(svc *service.MessageService) *MessageHandler {
	return &MessageHandler{svc: svc}
}

func (h *MessageHandler) ListDirectThreads(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	page, size := pageSize(c, 1, 20)
	resp, err := h.svc.ListDirectThreads(c.Request.Context(), uid, page, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *MessageHandler) DirectDraft(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	resp, err := h.svc.DirectDraft(c.Request.Context(), uid, c.Param("creatorID"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *MessageHandler) SendDirect(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	var req service.SendDirectReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid body"))
		return
	}
	resp, err := h.svc.SendDirect(c.Request.Context(), uid, req)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *MessageHandler) DirectMessages(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	page, size := pageSize(c, 1, 100)
	resp, err := h.svc.DirectMessages(c.Request.Context(), uid, c.Param("threadID"), page, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *MessageHandler) SendThreadMessage(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid body"))
		return
	}
	resp, err := h.svc.SendThreadMessage(c.Request.Context(), uid, c.Param("threadID"), req.Content)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *MessageHandler) UpdateThreadOptions(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	var req service.UpdateThreadOptionsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid body"))
		return
	}
	resp, err := h.svc.UpdateThreadOptions(c.Request.Context(), uid, c.Param("threadID"), req)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *MessageHandler) ListBlocks(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	page, size := pageSize(c, 1, 50)
	resp, err := h.svc.ListBlocks(c.Request.Context(), uid, page, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *MessageHandler) BlockUser(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	var req service.BlockUserReq
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.BlockUser(c.Request.Context(), uid, c.Param("userID"), req); err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *MessageHandler) UnblockUser(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	if err := h.svc.UnblockUser(c.Request.Context(), uid, c.Param("userID")); err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *MessageHandler) Preference(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	resp, err := h.svc.Preference(c.Request.Context(), uid)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *MessageHandler) UpdatePreference(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	var req service.MessagePreferenceDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid body"))
		return
	}
	resp, err := h.svc.UpdatePreference(c.Request.Context(), uid, req)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *MessageHandler) ListFanGroups(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	resp, err := h.svc.ListFanGroups(c.Request.Context(), uid)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *MessageHandler) SyncFanGroups(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	resp, err := h.svc.SyncFanGroups(c.Request.Context(), uid)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *MessageHandler) UpdateFanGroupMember(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	var req service.UpdateFanGroupMemberReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "invalid body"))
		return
	}
	resp, err := h.svc.UpdateFanGroupMember(c.Request.Context(), uid, c.Param("groupID"), c.Param("userID"), req)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}
