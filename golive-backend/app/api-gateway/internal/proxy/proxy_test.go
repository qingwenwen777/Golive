package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/api-gateway/internal/proxy"
)

func TestReverseProxy_StripsAPIPrefixAndForwardsClientAddress(t *testing.T) {
	var seenPath string
	var seenRawQuery string
	var seenHost string
	var seenForwardedFor string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenRawQuery = r.URL.RawQuery
		seenHost = r.Host
		seenForwardedFor = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)

	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	rp := proxy.New(target, proxy.Options{})

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/rooms/live?page=2", nil)
	req.RemoteAddr = "203.0.113.10:54321"
	req.Header.Set("X-Forwarded-For", "198.51.100.1")
	rec := httptest.NewRecorder()

	rp.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, "/rooms/live", seenPath)
	require.Equal(t, "page=2", seenRawQuery)
	require.Equal(t, target.Host, seenHost)
	require.Equal(t, "198.51.100.1, 203.0.113.10", seenForwardedFor)
}

func TestReverseProxy_RootAPIPathForwardsAsSlash(t *testing.T) {
	var seenPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	rp := proxy.New(target, proxy.Options{})

	rec := httptest.NewRecorder()
	rp.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://gateway.local/api", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "/", seenPath)
}

func TestReverseProxy_WrapsPlainUpstream5xxIntoUnifiedError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("database down"))
	}))
	t.Cleanup(upstream.Close)

	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	proxy.New(target, proxy.Options{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/users/me", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
	require.JSONEq(t, `{"message":"Internal error","reason":"upstream_unavailable"}`, rec.Body.String())
	require.Equal(t, rec.Header().Get("Content-Length"), intToStrForTest(rec.Body.Len()))
}

func TestReverseProxy_PreservesStructured5xxAndClient4xxBodies(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantBody   string
	}{
		{
			name:       "structured 5xx",
			statusCode: http.StatusBadGateway,
			body:       `{"message":"upstream said no","reason":"db_unavailable"}`,
			wantBody:   `{"message":"upstream said no","reason":"db_unavailable"}`,
		},
		{
			name:       "client 4xx",
			statusCode: http.StatusConflict,
			body:       `{"message":"duplicate request","reason":"duplicate"}`,
			wantBody:   `{"message":"duplicate request","reason":"duplicate"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(upstream.Close)

			target, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			rec := httptest.NewRecorder()
			proxy.New(target, proxy.Options{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/x", nil))

			require.Equal(t, tt.statusCode, rec.Code)
			require.JSONEq(t, tt.wantBody, rec.Body.String())
		})
	}
}

func intToStrForTest(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[n:])
}
