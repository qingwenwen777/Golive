// Package internalauth guards the service-to-service HTTP API every service
// mounts under /internal, and gives callers a small JSON client for it.
//
// Two layers keep /internal private: nginx only forwards /api/* (and /ws) to
// the edge, and api-gateway only proxies its own /api/<prefix>/* routes, so
// no public path maps onto /internal. On top of that every request must carry
// the shared secret in X-Internal-Token (compared in constant time). The
// middleware fails closed: a service started without a token rejects every
// internal call instead of serving them unauthenticated.
package internalauth

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/qingwenwen777/golive/pkg/errcode"
)

// Header carries the shared secret on internal requests.
const Header = "X-Internal-Token"

var errUnauthorized = errcode.New(http.StatusUnauthorized, "internal endpoint").WithReason("internal_auth_required")

// Middleware rejects requests whose X-Internal-Token does not equal token.
// With an empty token every request is rejected.
func Middleware(token string) gin.HandlerFunc {
	want := []byte(token)
	return func(c *gin.Context) {
		got := []byte(c.GetHeader(Header))
		if len(want) == 0 || subtle.ConstantTimeCompare(got, want) != 1 {
			errcode.Respond(c, errUnauthorized)
			return
		}
		c.Next()
	}
}

// StatusError is returned by Client.Do for a non-2xx response.
type StatusError struct {
	Method string
	URL    string
	Status int
	// Reason is the reason of the errcode JSON body ({message, reason}) the
	// response carried, or "" for any other body, such as the router's 404
	// for a route the service does not have.
	Reason string
}

func (e *StatusError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("%s %s returned %d (%s)", e.Method, e.URL, e.Status, e.Reason)
	}
	return fmt.Sprintf("%s %s returned %d", e.Method, e.URL, e.Status)
}

// IsStatus reports whether err is a StatusError with the given status.
func IsStatus(err error, status int) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Status == status
}

// IsReason reports whether err is a StatusError with the given status and
// (non-empty) errcode reason, i.e. the called handler itself answered that
// way.
func IsReason(err error, status int, reason string) bool {
	var se *StatusError
	return reason != "" && errors.As(err, &se) && se.Status == status && se.Reason == reason
}

// Client calls another service's /internal API with the shared secret.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient builds a client for baseURL (e.g. http://user-service:8090).
// timeout bounds each call; zero means 3s.
func NewClient(baseURL, token string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   token,
		http:    &http.Client{Timeout: timeout},
	}
}

// Do sends body (JSON-encoded unless nil) to path and decodes a 2xx response
// into out (skipped when nil). Any other status is a *StatusError.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	if c == nil || c.baseURL == "" {
		return fmt.Errorf("internal client for %s is not configured", path)
	}
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}
	endpoint := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	SetToken(req, c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		var appErr struct {
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(raw, &appErr)
		return &StatusError{Method: method, URL: endpoint, Status: resp.StatusCode, Reason: appErr.Reason}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

// SetToken attaches the shared secret to an outbound internal request.
func SetToken(req *http.Request, token string) {
	if token != "" {
		req.Header.Set(Header, token)
	}
}
