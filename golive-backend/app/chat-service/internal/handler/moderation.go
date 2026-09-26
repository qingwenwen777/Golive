package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/chat-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

// MessageHider removes a chat message from history.
type MessageHider interface {
	HideMessage(ctx context.Context, roomID, messageID string) error
}

// ModerationHandler serves the internal endpoint room-service's report
// moderation calls to delete a reported chat message, so only chat-service
// writes its danmus_* shards.
type ModerationHandler struct {
	src MessageHider
}

func NewModerationHandler(src MessageHider) *ModerationHandler {
	return &ModerationHandler{src: src}
}

// DeleteDanmu serves DELETE /internal/rooms/:id/danmus/:danmuId →
// {"hidden": true}. Deleting an already hidden message succeeds; an unknown
// one is 404 with reason message_not_found, which room-service takes as
// "nothing left to hide" (a 404 without it, e.g. for a missing route, is an
// error there).
func (h *ModerationHandler) DeleteDanmu(c *gin.Context) {
	roomID := strings.TrimSpace(c.Param("id"))
	danmuID := strings.TrimSpace(c.Param("danmuId"))
	if roomID == "" || danmuID == "" {
		errcode.Respond(c, errcode.New(http.StatusBadRequest, "missing room or message id"))
		return
	}
	err := h.src.HideMessage(c.Request.Context(), roomID, danmuID)
	if errors.Is(err, service.ErrMessageNotFound) {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "message not found").WithReason("message_not_found"))
		return
	}
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"hidden": true})
}
