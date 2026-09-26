package httpserver

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNew_SetsEveryTimeout(t *testing.T) {
	h := http.NewServeMux()
	srv := New(":8080", h)

	require.Equal(t, ":8080", srv.Addr)
	require.Same(t, h, srv.Handler)
	require.Equal(t, ReadHeaderTimeout, srv.ReadHeaderTimeout)
	require.Equal(t, ReadTimeout, srv.ReadTimeout)
	require.Equal(t, WriteTimeout, srv.WriteTimeout)
	require.Equal(t, IdleTimeout, srv.IdleTimeout)
	for name, d := range map[string]time.Duration{
		"ReadHeaderTimeout": srv.ReadHeaderTimeout,
		"ReadTimeout":       srv.ReadTimeout,
		"WriteTimeout":      srv.WriteTimeout,
		"IdleTimeout":       srv.IdleTimeout,
	} {
		require.Positive(t, d, name)
	}
	// A body can only be read after the headers.
	require.Greater(t, srv.ReadTimeout, srv.ReadHeaderTimeout)
}

// WebSocket connections live for hours: a ReadTimeout or WriteTimeout would
// be armed on the upgrade request's connection.
func TestNewLongLived_OnlyHeaderAndIdleTimeouts(t *testing.T) {
	srv := NewLongLived(":8081", http.NewServeMux())

	require.Equal(t, ":8081", srv.Addr)
	require.Equal(t, ReadHeaderTimeout, srv.ReadHeaderTimeout)
	require.Equal(t, IdleTimeout, srv.IdleTimeout)
	require.Zero(t, srv.ReadTimeout)
	require.Zero(t, srv.WriteTimeout)
}

func TestStartPprof_EmptyAddrStartsNothing(t *testing.T) {
	for _, addr := range []string{"", "   "} {
		require.Nil(t, StartPprof(addr, zap.NewNop()), "addr %q", addr)
	}
}

func TestStartPprof_ServesOnAddr(t *testing.T) {
	addr := freeLoopbackAddr(t)
	srv := StartPprof(addr, zap.NewNop())
	require.NotNil(t, srv)
	t.Cleanup(func() { _ = srv.Close() })

	// The listener starts in a goroutine; retry until it accepts.
	var (
		resp *http.Response
		err  error
	)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if resp, err = http.Get("http://" + addr + "/debug/pprof/cmdline"); err == nil {
			break
		}
	}
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestStartPprof_WarnsWhenNotLoopback(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	// 192.0.2.0/24 is TEST-NET-1: never assigned locally, so the listener
	// fails instead of binding a public interface during the test.
	srv := StartPprof("192.0.2.1:6060", zap.New(core))
	require.NotNil(t, srv)
	t.Cleanup(func() { _ = srv.Close() })

	require.Equal(t, 1, logs.FilterMessageSnippet("beyond loopback").Len())
}

func TestNewPprof_ServesOnlyPprofHandlers(t *testing.T) {
	srv := NewPprof("127.0.0.1:0")
	require.Zero(t, srv.ReadTimeout)
	require.Zero(t, srv.WriteTimeout, "profiles stream for ?seconds=N; pprof rejects N >= WriteTimeout")
	require.Equal(t, ReadHeaderTimeout, srv.ReadHeaderTimeout)

	for path, want := range map[string]int{
		"/debug/pprof/":        http.StatusOK,
		"/debug/pprof/cmdline": http.StatusOK,
		"/debug/pprof/heap":    http.StatusOK,
		"/":                    http.StatusNotFound,
		"/healthz":             http.StatusNotFound,
		"/metrics":             http.StatusNotFound,
	} {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, want, rec.Code, path)
	}
}

func TestNewPprof_ServesCPUProfile(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := NewPprof(ln.Addr().String())
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	resp, err := http.Get("http://" + ln.Addr().String() + "/debug/pprof/profile?seconds=1")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	require.NotEmpty(t, body)
}

func TestIsLoopback(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:6060": true,
		"127.0.0.2:6060": true,
		"localhost:6060": true,
		"[::1]:6060":     true,
		":6060":          false,
		"0.0.0.0:6060":   false,
		"[::]:6060":      false,
		"10.0.0.5:6060":  false,
		"example.com:80": false,
		"6060":           false,
		"":               false,
	} {
		require.Equal(t, want, IsLoopback(addr), addr)
	}
}

func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}
