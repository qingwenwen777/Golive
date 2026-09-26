package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/qingwenwen777/golive/pkg/internalauth"
)

// roomServiceTimeout keeps a ban quick when room-service is slow or down; its
// live reconciler ends a banned owner's rooms within a reconcile interval.
const roomServiceTimeout = 2 * time.Second

// RoomServiceClient calls room-service's /internal API. room-service owns
// rooms, so a ban asks it to end the user's live rooms.
type RoomServiceClient struct{ c *internalauth.Client }

// NewRoomServiceClient builds a client for baseURL (e.g.
// http://room-service:8091), authenticated with the shared internal token.
func NewRoomServiceClient(baseURL, token string) *RoomServiceClient {
	return &RoomServiceClient{c: internalauth.NewClient(baseURL, token, roomServiceTimeout)}
}

// EndUserLive ends every active room of userID, kicking its publisher, and
// returns how many rooms were ended.
func (r *RoomServiceClient) EndUserLive(ctx context.Context, userID string) (int, error) {
	var out struct {
		EndedRooms int `json:"endedRooms"`
	}
	path := "/internal/users/" + url.PathEscape(userID) + "/end-live"
	if err := r.c.Do(ctx, http.MethodPost, path, nil, &out); err != nil {
		return 0, fmt.Errorf("room-service end live of user %s: %w", userID, err)
	}
	return out.EndedRooms, nil
}
