package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type LiveHandler struct {
	svc *service.LiveService
}

func NewLiveHandler(svc *service.LiveService) *LiveHandler {
	return &LiveHandler{svc: svc}
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
