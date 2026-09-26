package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/room-service/internal/server"
)

type denyLivePermission struct{}

func (denyLivePermission) HasApprovedLivePermission(context.Context, string) (bool, error) {
	return false, nil
}

func requestJSON(router http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// Appointments end in a stream key, so they must be gated by the same creator
// approval as POST /rooms/live.
func TestAppointmentHTTP_RequiresApprovedLivePermission(t *testing.T) {
	router := server.NewRouter(server.Deps{
		JWTSecret:  jwtSecret,
		Permission: denyLivePermission{},
	})
	token := signedToken(t, "not-a-creator")
	body := `{"scheduledAt":"2030-01-01T00:00:00Z","title":"Sneaky","description":"no approval","cover":"/c.jpg"}`

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/rooms/live", `{"title":"Sneaky","category":"Gaming"}`},
		{http.MethodPost, "/rooms/appointments", body},
		{http.MethodPatch, "/rooms/appointments/appt-1", body},
		{http.MethodPost, "/rooms/appointments/appt-1/start", ""},
	}
	for _, tc := range cases {
		rec := requestJSON(router, tc.method, tc.path, token, tc.body)
		require.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", tc.method, tc.path, rec.Body.String())
	}
}
