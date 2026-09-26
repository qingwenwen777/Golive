package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/chat-service/internal/model"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

// FanBadgeSource looks up a user's fan badge for a room's owner.
type FanBadgeSource interface {
	FanBadge(ctx context.Context, roomID, userID string) (*model.FanBadgePayload, error)
}

// FanBadgeHandler serves the internal lookup im-gateway uses to decorate
// live chat with the sender's real fan badge. It is mounted under /internal,
// which api-gateway does not proxy (only /api/chat/* reaches chat-service)
// and which requires the internal token.
type FanBadgeHandler struct {
	src FanBadgeSource
}

func NewFanBadgeHandler(src FanBadgeSource) *FanBadgeHandler {
	return &FanBadgeHandler{src: src}
}

// Get serves GET /internal/rooms/:id/fan-badges/:userId →
// {"fanBadge": {"creatorId","level"} | null}.
func (h *FanBadgeHandler) Get(c *gin.Context) {
	roomID := strings.TrimSpace(c.Param("id"))
	userID := strings.TrimSpace(c.Param("userId"))
	if roomID == "" || userID == "" {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "missing room or user id"))
		return
	}
	badge, err := h.src.FanBadge(c.Request.Context(), roomID, userID)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"fanBadge": badge})
}
