package server

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOriginAllowed_AllowsConfiguredOrigin(t *testing.T) {
	req := httptest.NewRequest("GET", "http://localhost:8081/ws", nil)
	req.Header.Set("Origin", "http://localhost:5173")

	require.True(t, originAllowed(req, []string{"http://localhost:5173"}))
}

func TestOriginAllowed_AllowsSameOriginBehindProxy(t *testing.T) {
	req := httptest.NewRequest("GET", "http://im-gateway:8081/ws", nil)
	req.Host = "golive.us.ci"
	req.Header.Set("Origin", "https://golive.us.ci")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "golive.us.ci")

	require.True(t, originAllowed(req, nil))
}

func TestOriginAllowed_RejectsUnexpectedOrigin(t *testing.T) {
	req := httptest.NewRequest("GET", "http://localhost:8081/ws", nil)
	req.Header.Set("Origin", "http://evil.example.com")

	require.False(t, originAllowed(req, []string{"http://localhost:5173"}))
}

func TestOriginAllowed_AllowsNonBrowserClients(t *testing.T) {
	req := httptest.NewRequest("GET", "http://localhost:8081/ws", nil)

	require.True(t, originAllowed(req, []string{"http://localhost:5173"}))
}
