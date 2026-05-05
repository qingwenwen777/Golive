// Package proxy builds a reverse proxy with gateway-aware behavior:
//
//   - Strips the /api prefix on the way out (upstreams mount routes at root).
//   - Forces X-User-Id and X-Request-Id headers onto the outbound request
//     (already set by middleware on c.Request.Header).
//   - Wraps upstream 5xx into the unified {message, reason} shape.
//   - Bounded per-request timeout so a hung upstream can't pile up sockets.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/qingwenwen777/golive/pkg/logger"
)

const apiPrefix = "/api"

// Options tunes the underlying transport.
type Options struct {
	Timeout             time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
}

// New builds a *httputil.ReverseProxy targeting upstream. prefix is stripped
// from the inbound path before forwarding (e.g. /api/rooms → /rooms).
func New(upstream *url.URL, opts Options) *httputil.ReverseProxy {
	if opts.Timeout == 0 {
		opts.Timeout = 10 * time.Second
	}
	if opts.MaxIdleConns == 0 {
		opts.MaxIdleConns = 100
	}
	if opts.MaxIdleConnsPerHost == 0 {
		opts.MaxIdleConnsPerHost = 32
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   3 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        opts.MaxIdleConns,
		MaxIdleConnsPerHost: opts.MaxIdleConnsPerHost,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
		// timeoutTransport applies an overall deadline:
	}
	rt := &timeoutTransport{base: transport, timeout: opts.Timeout}

	rp := &httputil.ReverseProxy{
		Transport: rt,
		Director: func(req *http.Request) {
			req.URL.Scheme = upstream.Scheme
			req.URL.Host = upstream.Host
			req.Host = upstream.Host
			req.URL.Path = strings.TrimPrefix(req.URL.Path, apiPrefix)
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
		},
		ModifyResponse: wrapUpstreamErrors,
		ErrorHandler:   handleProxyError,
	}
	return rp
}

// wrapUpstreamErrors rewrites 5xx bodies that are NOT already in the unified
// shape. Non-5xx pass through untouched. We never re-wrap a 4xx — those are
// the upstream's deliberate signals (401 / 402 / 404 / 409) and the frontend
// branches on their reason fields.
func wrapUpstreamErrors(resp *http.Response) error {
	if resp.StatusCode < 500 {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return err
	}

	// If body already parses to {message,...}, keep it. Otherwise replace.
	var probe struct {
		Message string `json:"message"`
	}
	keep := json.Unmarshal(body, &probe) == nil && probe.Message != ""

	var out []byte
	if keep {
		out = body
	} else {
		out, _ = json.Marshal(map[string]string{
			"message": "Internal error",
			"reason":  "upstream_unavailable",
		})
	}
	resp.Body = io.NopCloser(bytes.NewReader(out))
	resp.ContentLength = int64(len(out))
	resp.Header.Set("Content-Length", intToStr(len(out)))
	resp.Header.Set("Content-Type", "application/json; charset=utf-8")
	return nil
}

// handleProxyError fires when the upstream is unreachable / times out. The
// frontend sees a 502 with the unified body so the axios interceptor can
// surface a normal error (NOT a 401 → no refresh storm).
func handleProxyError(w http.ResponseWriter, r *http.Request, err error) {
	logger.L().Warn("upstream error",
		zap.String("path", r.URL.Path), zap.Error(err))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	status := http.StatusBadGateway
	if errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusGatewayTimeout
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"message":"Internal error","reason":"upstream_unavailable"}`))
}

// timeoutTransport enforces an overall timeout per round-trip, including
// reading the response body header. Body streaming after that point relies
// on the upstream not stalling indefinitely.
type timeoutTransport struct {
	base    http.RoundTripper
	timeout time.Duration
}

func (t *timeoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(req.Context(), t.timeout)
	// Cancel is forwarded onto the response Body close. We deliberately do
	// not cancel here in a defer because we must keep ctx alive while the
	// caller reads Body.
	req = req.WithContext(ctx)
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	c.cancel()
	return c.ReadCloser.Close()
}

func intToStr(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = digits[i%10]
		i /= 10
	}
	return string(buf[n:])
}
