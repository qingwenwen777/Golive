package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/chat-service/internal/model"
)

type badgeMap map[string]*model.FanBadgePayload

func (m badgeMap) FanBadge(_ context.Context, roomID, userID string) (*model.FanBadgePayload, error) {
	return m[roomID+"/"+userID], nil
}

func TestFanBadgeHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewFanBadgeHandler(badgeMap{"live-1/u-1": {CreatorID: "owner-1", Level: 4}})
	r.GET("/internal/rooms/:id/fan-badges/:userId", h.Get)

	for path, want := range map[string]string{
		"/internal/rooms/live-1/fan-badges/u-1": `{"fanBadge":{"creatorId":"owner-1","level":4}}`,
		"/internal/rooms/live-1/fan-badges/u-2": `{"fanBadge":null}`,
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, w.Code, path)
		require.JSONEq(t, want, w.Body.String(), path)
	}
}
