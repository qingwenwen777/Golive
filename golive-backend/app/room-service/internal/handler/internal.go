package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/app/room-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

// InternalHandler serves /internal/*, the service-to-service API. Routes are
// guarded by the shared internal token (see server.NewRouter).
type InternalHandler struct {
	live *service.LiveService
}

func NewInternalHandler(live *service.LiveService) *InternalHandler {
	return &InternalHandler{live: live}
}

// endUserLiveTimeout bounds ending a user's rooms once the caller has gone.
const endUserLiveTimeout = 10 * time.Second

// EndUserLive serves POST /internal/users/:id/end-live. user-service calls it
// when it bans a user, so a live that is already running ends (and its SRS
// publisher is kicked) instead of only the next GoLive being refused. It is
// idempotent and never calls back into user-service. The stop runs to the end
// even if the caller stops waiting, so a room is not left ended but still
// streaming.
func (h *InternalHandler) EndUserLive(c *gin.Context) {
	userID := strings.TrimSpace(c.Param("id"))
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), endUserLiveTimeout)
	defer cancel()
	ended, err := h.live.ForceStopOwnerRooms(ctx, userID)
	if err != nil {
		errcode.Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"userId": userID, "endedRooms": ended})
}
