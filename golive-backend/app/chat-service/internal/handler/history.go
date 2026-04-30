package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/chat-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type HistoryHandler struct {
	svc          *service.ChatService
	defaultLimit int
	maxLimit     int
}

func NewHistoryHandler(svc *service.ChatService, defaultLimit, maxLimit int) *HistoryHandler {
	if defaultLimit <= 0 {
		defaultLimit = 50
	}
	if maxLimit <= 0 {
		maxLimit = 200
	}
	return &HistoryHandler{svc: svc, defaultLimit: defaultLimit, maxLimit: maxLimit}
}

// Get serves GET /rooms/:id/danmus?before=<ms>&limit=<n>
func (h *HistoryHandler) Get(c *gin.Context) {
	roomID := c.Param("id")
	before, _ := strconv.ParseInt(c.Query("before"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(h.defaultLimit)))
	if limit > h.maxLimit {
		limit = h.maxLimit
	}
	rows, err := h.svc.History(c.Request.Context(), roomID, before, limit)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}
