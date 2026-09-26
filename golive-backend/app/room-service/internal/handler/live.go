package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type LiveHandler struct {
	svc        *service.LiveService
	permission service.LivePermissionChecker
}

func NewLiveHandler(svc *service.LiveService, permission service.LivePermissionChecker) *LiveHandler {
	return &LiveHandler{svc: svc, permission: permission}
}

// GoLive: POST /rooms/live (auth required).
// Frontend sends { title, description, category, cover, channelName }; we return the Stream including
// streamKey (which the publisher uses as the RTMP path on `rtmp://srs/live/<streamKey>`).
func (h *LiveHandler) GoLive(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(401, "Unauthorized"))
		return
	}
	if !requireLivePermission(c, h.permission, uid) {
		return
	}
	var req service.GoLiveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(400, "invalid body"))
		return
	}
	st, err := h.svc.GoLive(c.Request.Context(), uid, req)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, st)
}

// UpdateLive: PATCH /rooms/live (auth required).
// Lets the publisher edit the active live room's title, description, and cover.
func (h *LiveHandler) UpdateLive(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(401, "Unauthorized"))
		return
	}
	var req service.UpdateLiveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.New(400, "invalid body"))
		return
	}
	st, err := h.svc.UpdateLiveMetadata(c.Request.Context(), uid, req)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, st)
}

// StopLive: DELETE /rooms/live (auth required).
func (h *LiveHandler) StopLive(c *gin.Context) {
	uid := UserIDFromCtx(c)
	if uid == "" {
		errcode.Respond(c, errcode.New(401, "Unauthorized"))
		return
	}
	if err := h.svc.StopLive(c.Request.Context(), uid); err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// requireLivePermission rejects users whose creator application has not been
// approved. Every path that ends in a stream key (instant live, appointment
// create/update/start) must go through it. Responds and returns false when the
// request must stop.
func requireLivePermission(c *gin.Context, permission service.LivePermissionChecker, uid string) bool {
	if permission == nil {
		return true
	}
	approved, err := permission.HasApprovedLivePermission(c.Request.Context(), uid)
	if err != nil {
		errcode.Respond(c, err)
		return false
	}
	if !approved {
		errcode.Respond(c, errcode.New(http.StatusForbidden, "Live permission is not approved. Please submit a creator application and wait for admin approval."))
		return false
	}
	return true
}
