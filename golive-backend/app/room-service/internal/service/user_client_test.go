package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/pkg/internalauth"
)

func TestUserPermissionClientHTTPFallbackSendsInternalToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(internalauth.Header) != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/internal/users/u1/permission" {
			w.WriteHeader(http.StatusTeapot)
			return
		}
		_, _ = w.Write([]byte(`{"livePermissionStatus":"approved"}`))
	}))
	t.Cleanup(srv.Close)

	c, err := NewUserPermissionClient("", srv.URL, "tok")
	require.NoError(t, err)
	ok, err := c.HasApprovedLivePermission(context.Background(), "u1")
	require.NoError(t, err)
	require.True(t, ok)

	unauth, err := NewUserPermissionClient("", srv.URL, "")
	require.NoError(t, err)
	_, err = unauth.HasApprovedLivePermission(context.Background(), "u1")
	require.ErrorContains(t, err, "401")
}
