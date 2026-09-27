package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/chat-service/internal/handler"
	"github.com/qingwenwen777/golive/app/chat-service/internal/model"
	"github.com/qingwenwen777/golive/app/chat-service/internal/server"
	"github.com/qingwenwen777/golive/app/chat-service/internal/service"
	"github.com/qingwenwen777/golive/pkg/internalauth"
)

const testToken = "internal-test-token"

type fakeChat struct {
	hidden map[string]bool
}

func (f *fakeChat) FanBadge(_ context.Context, roomID, userID string) (*model.FanBadgePayload, error) {
	if roomID == "live-1" && userID == "u-1" {
		return &model.FanBadgePayload{CreatorID: "owner-1", Level: 3}, nil
	}
	return nil, nil
}

func (f *fakeChat) HideMessage(_ context.Context, roomID, messageID string) error {
	key := roomID + "/" + messageID
	if _, ok := f.hidden[key]; !ok {
		return service.ErrMessageNotFound
	}
	f.hidden[key] = true
	return nil
}

func newRouter(token string) (*gin.Engine, *fakeChat) {
	gin.SetMode(gin.TestMode)
	chat := &fakeChat{hidden: map[string]bool{"live-1/m-1": false}}
	r := server.NewRouter(handler.NewHistoryHandler(nil, 0, 0), handler.NewFanBadgeHandler(chat), handler.NewModerationHandler(chat), token)
	return r, chat
}

func serve(r http.Handler, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set(internalauth.Header, token)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestInternalRoutesRequireToken(t *testing.T) {
	r, chat := newRouter(testToken)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/internal/rooms/live-1/fan-badges/u-1"},
		{http.MethodDelete, "/internal/rooms/live-1/danmus/m-1"},
	} {
		for _, token := range []string{"", "wrong"} {
			rec := serve(r, tc.method, tc.path, token)
			require.Equal(t, http.StatusUnauthorized, rec.Code, "%s %s token=%q", tc.method, tc.path, token)
		}
	}
	require.False(t, chat.hidden["live-1/m-1"])

	closed, _ := newRouter("")
	rec := serve(closed, http.MethodGet, "/internal/rooms/live-1/fan-badges/u-1", "")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestInternalFanBadgeAndDeleteDanmu(t *testing.T) {
	r, chat := newRouter(testToken)

	rec := serve(r, http.MethodGet, "/internal/rooms/live-1/fan-badges/u-1", testToken)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"fanBadge":{"creatorId":"owner-1","level":3}}`, rec.Body.String())

	rec = serve(r, http.MethodDelete, "/internal/rooms/live-1/danmus/m-1", testToken)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"hidden":true}`, rec.Body.String())
	require.True(t, chat.hidden["live-1/m-1"])

	// room-service treats only this reason as "nothing left to hide".
	rec = serve(r, http.MethodDelete, "/internal/rooms/live-1/danmus/missing", testToken)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"message":"message not found","reason":"message_not_found"}`, rec.Body.String())
}

// api-gateway forwards /api/chat/<rest> as /chat/<rest> without cleaning
// dot segments; such a path must not reach an /internal route.
func TestDotSegmentsDoNotReachInternalRoutes(t *testing.T) {
	r, chat := newRouter(testToken)
	for _, path := range []string{
		"/chat/../internal/rooms/live-1/danmus/m-1",
		"/chat/%2e%2e/internal/rooms/live-1/danmus/m-1",
	} {
		rec := serve(r, http.MethodDelete, path, testToken)
		require.Equal(t, http.StatusNotFound, rec.Code, path)
	}
	require.False(t, chat.hidden["live-1/m-1"])
}
