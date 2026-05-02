package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type AppointmentHandler struct {
	svc *service.AppointmentService
}

func NewAppointmentHandler(svc *service.AppointmentService) *AppointmentHandler {
	return &AppointmentHandler{svc: svc}
}

type appointmentReq struct {
	ScheduledAt string `json:"scheduledAt" binding:"required"`
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	Cover       string `json:"cover"`
	ChannelName string `json:"channelName"`
	Avatar      string `json:"avatar"`
}

func (h *AppointmentHandler) Create(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	payload, ok := bindAppointmentPayload(c)
	if !ok {
		return
	}
	resp, err := h.svc.Create(c.Request.Context(), uid, payload)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *AppointmentHandler) Update(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	payload, ok := bindAppointmentPayload(c)
	if !ok {
		return
	}
	resp, err := h.svc.Update(c.Request.Context(), uid, c.Param("id"), payload)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *AppointmentHandler) Cancel(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	resp, err := h.svc.Cancel(c.Request.Context(), uid, c.Param("id"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *AppointmentHandler) Start(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	resp, err := h.svc.Start(c.Request.Context(), uid, c.Param("id"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *AppointmentHandler) ListOwner(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	page, size := pageSize(c, 1, 10)
	resp, err := h.svc.ListOwner(c.Request.Context(), uid, page, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *AppointmentHandler) ListChannel(c *gin.Context) {
	page, size := pageSize(c, 1, 6)
	resp, err := h.svc.ListChannel(c.Request.Context(), c.Param("channel"), UserIDFromCtx(c), page, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *AppointmentHandler) ListSubscriptionAppointments(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	page, size := pageSize(c, 1, 8)
	resp, err := h.svc.ListSubscriptionAppointments(c.Request.Context(), uid, page, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *AppointmentHandler) ListReserved(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	page, size := pageSize(c, 1, 8)
	resp, err := h.svc.ListReserved(c.Request.Context(), uid, page, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *AppointmentHandler) Reserve(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	resp, err := h.svc.Reserve(c.Request.Context(), uid, c.Param("id"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *AppointmentHandler) Unreserve(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	resp, err := h.svc.Unreserve(c.Request.Context(), uid, c.Param("id"))
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *AppointmentHandler) Notifications(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	page, size := pageSize(c, 1, 20)
	resp, err := h.svc.Notifications(c.Request.Context(), uid, page, size)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *AppointmentHandler) MarkNotificationRead(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	if err := h.svc.MarkNotificationRead(c.Request.Context(), uid, c.Param("id")); err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AppointmentHandler) MarkAllNotificationsRead(c *gin.Context) {
	uid, ok := requireUser(c)
	if !ok {
		return
	}
	if err := h.svc.MarkAllNotificationsRead(c.Request.Context(), uid); err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func bindAppointmentPayload(c *gin.Context) (service.AppointmentPayload, bool) {
	var req appointmentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(400, "invalid body"))
		return service.AppointmentPayload{}, false
	}
	scheduledAt, err := time.Parse(time.RFC3339, req.ScheduledAt)
	if err != nil {
		errcode.Respond(c, errcode.New(400, "scheduledAt must be RFC3339"))
		return service.AppointmentPayload{}, false
	}
	return service.AppointmentPayload{
		ScheduledAt: scheduledAt.UTC(),
		Title:       req.Title,
		Description: req.Description,
		Cover:       req.Cover,
		ChannelName: req.ChannelName,
		Avatar:      req.Avatar,
	}, true
}

func requireUser(c *gin.Context) (string, bool) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(401, "Unauthorized"))
		return "", false
	}
	return uid, true
}

func pageSize(c *gin.Context, defaultPage, defaultSize int) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", strconv.Itoa(defaultPage)))
	size, _ := strconv.Atoi(c.DefaultQuery("size", strconv.Itoa(defaultSize)))
	if page < 1 {
		page = defaultPage
	}
	if size < 1 {
		size = defaultSize
	}
	if size > 100 {
		size = 100
	}
	return page, size
}
