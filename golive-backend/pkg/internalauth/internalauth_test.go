package internalauth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/pkg/errcode"
	"github.com/qingwenwen777/golive/pkg/internalauth"
)

func init() { gin.SetMode(gin.TestMode) }

func guarded(token string) *gin.Engine {
	r := gin.New()
	r.GET("/internal/ping", internalauth.Middleware(token), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func serve(r http.Handler, token string, setHeader bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/internal/ping", nil)
	if setHeader {
		req.Header.Set(internalauth.Header, token)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestMiddlewareRequiresMatchingToken(t *testing.T) {
	r := guarded("s3cret")
	require.Equal(t, http.StatusUnauthorized, serve(r, "", false).Code)
	require.Equal(t, http.StatusUnauthorized, serve(r, "wrong", true).Code)
	require.Equal(t, http.StatusUnauthorized, serve(r, "s3cret-and-more", true).Code)
	rec := serve(r, "s3cret", true)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestMiddlewareFailsClosedWithoutConfiguredToken(t *testing.T) {
	r := guarded("")
	require.Equal(t, http.StatusUnauthorized, serve(r, "", false).Code)
	require.Equal(t, http.StatusUnauthorized, serve(r, "", true).Code)
	rec := serve(r, "anything", true)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Contains(t, rec.Body.String(), "internal_auth_required")
}

func TestClientSendsTokenAndJSON(t *testing.T) {
	var gotToken, gotType, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get(internalauth.Header)
		gotType = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	c := internalauth.NewClient(srv.URL+"/", "s3cret", time.Second)
	var out struct {
		OK bool `json:"ok"`
	}
	require.NoError(t, c.Do(context.Background(), http.MethodPost, "/internal/x", map[string]string{"a": "b"}, &out))
	require.True(t, out.OK)
	require.Equal(t, "s3cret", gotToken)
	require.Equal(t, "application/json", gotType)
	require.Equal(t, "/internal/x", gotPath)
	require.Equal(t, "b", gotBody["a"])
}

func TestClientReturnsStatusErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	err := internalauth.NewClient(srv.URL, "t", time.Second).Do(context.Background(), http.MethodDelete, "/internal/x", nil, nil)
	require.Error(t, err)
	require.True(t, internalauth.IsStatus(err, http.StatusServiceUnavailable))
	require.False(t, internalauth.IsStatus(err, http.StatusNotFound))
}

// Only the handler's errcode body carries a reason; the router's own 404 for
// a route the service does not have carries none, so callers can tell "the
// item does not exist" from "this endpoint does not exist".
func TestClientReportsErrcodeReason(t *testing.T) {
	r := gin.New()
	r.DELETE("/internal/items/:id", func(c *gin.Context) {
		errcode.Respond(c, errcode.New(http.StatusNotFound, "item not found").WithReason("item_not_found"))
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	c := internalauth.NewClient(srv.URL, "t", time.Second)

	err := c.Do(context.Background(), http.MethodDelete, "/internal/items/1", nil, nil)
	require.True(t, internalauth.IsReason(err, http.StatusNotFound, "item_not_found"))
	require.False(t, internalauth.IsReason(err, http.StatusNotFound, "other_reason"))
	require.ErrorContains(t, err, "item_not_found")

	err = c.Do(context.Background(), http.MethodDelete, "/internal/old-route/1", nil, nil)
	require.True(t, internalauth.IsStatus(err, http.StatusNotFound))
	require.False(t, internalauth.IsReason(err, http.StatusNotFound, "item_not_found"))
	require.False(t, internalauth.IsReason(err, http.StatusNotFound, ""))
}

func TestClientTimesOutAndRejectsMissingBaseURL(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(block) })

	err := internalauth.NewClient(srv.URL, "t", 50*time.Millisecond).Do(context.Background(), http.MethodGet, "/internal/x", nil, nil)
	require.Error(t, err)

	err = internalauth.NewClient("", "t", time.Second).Do(context.Background(), http.MethodGet, "/internal/x", nil, nil)
	require.ErrorContains(t, err, "not configured")
}
