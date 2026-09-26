package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// X-Forwarded-For used to be read from the left, and the left end is
// whatever the client sent: behind a proxy that appends to the header, a
// client could pick its IP and dodge the per-IP cap.
func TestClientIP_TakesLastUntrustedForwardedFor(t *testing.T) {
	_, proxyNet, err := net.ParseCIDR("10.0.0.0/8")
	require.NoError(t, err)
	trusted := []*net.IPNet{proxyNet}

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.RemoteAddr = "10.1.2.3:4000"
	// The client sent 1.2.3.4, the edge proxy appended the address it saw
	// and an inner proxy appended the edge's.
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 198.51.100.7, 10.9.9.9")
	require.Equal(t, "198.51.100.7", clientIP(req, trusted))

	// The same list split over several header lines.
	req.Header.Del("X-Forwarded-For")
	req.Header.Add("X-Forwarded-For", "1.2.3.4")
	req.Header.Add("X-Forwarded-For", "198.51.100.7, 10.9.9.9")
	require.Equal(t, "198.51.100.7", clientIP(req, trusted))

	// A garbled hop ends the walk: nothing left of it can be trusted.
	req.Header.Set("X-Forwarded-For", "1.2.3.4, garbage")
	require.Equal(t, "10.1.2.3", clientIP(req, trusted))

	// X-Real-IP, which the proxy overwrites, still comes first.
	req.Header.Set("X-Real-IP", "203.0.113.5")
	require.Equal(t, "203.0.113.5", clientIP(req, trusted))
}
