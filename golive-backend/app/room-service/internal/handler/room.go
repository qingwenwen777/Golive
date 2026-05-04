package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type RoomHandler struct {
	svc *service.RoomService
}

func NewRoomHandler(svc *service.RoomService) *RoomHandler {
	return &RoomHandler{svc: svc}
}

func (h *RoomHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "24"))
	resp, err := h.svc.List(c.Request.Context(), c.Query("category"), page, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RoomHandler) Get(c *gin.Context) {
	id := c.Param("id")
	st, err := h.svc.Get(c.Request.Context(), id, UserIDFromCtx(c))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, st)
}

func (h *RoomHandler) ChannelHistory(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "24"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 24
	}
	replaysOnly := c.Query("mode") == "replay" || c.Query("replaysOnly") == "1" || c.Query("replaysOnly") == "true"
	resp, err := h.svc.HistoryByChannel(c.Request.Context(), c.Param("channel"), UserIDFromCtx(c), page, size, replaysOnly)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RoomHandler) ChannelAnalytics(c *gin.Context) {
	resp, err := h.svc.CreatorAnalytics(c.Request.Context(), c.Param("channel"), UserIDFromCtx(c))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RoomHandler) LiveAnalysis(c *gin.Context) {
	resp, err := h.svc.LiveAnalysis(c.Request.Context(), c.Param("channel"), c.Param("recordID"), UserIDFromCtx(c))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}
