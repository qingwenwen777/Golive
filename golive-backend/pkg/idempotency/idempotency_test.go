package idempotency_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/pkg/idempotency"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type memoryStore struct {
	mu     sync.Mutex
	values map[string]string
}

func newMemoryStore() *memoryStore {
	return &memoryStore{values: map[string]string{}}
}

func (s *memoryStore) Get(_ context.Context, key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.values[key]
	return v, ok, nil
}

func (s *memoryStore) SetNX(_ context.Context, key, val string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.values[key]; ok {
		return false, nil
	}
	s.values[key] = val
	return true, nil
}

func (s *memoryStore) Set(_ context.Context, key, val string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = val
	return nil
}

func TestMiddleware_ReplaysCachedResponseAndDoesNotCallHandlerAgain(t *testing.T) {
	store := newMemoryStore()
	router, calls := newIdempotentRouter(store)

	first := postJSON(router, "/pay", `{"requestId":"req-1","amount":10}`, "")
	require.Equal(t, http.StatusCreated, first.Code)
	require.Empty(t, first.Header().Get(idempotency.HeaderReplayed))
	require.JSONEq(t, `{"ok":true,"body":{"requestId":"req-1","amount":10}}`, first.Body.String())
	require.Equal(t, 1, *calls)

	replay := postJSON(router, "/pay", `{"requestId":"req-1","amount":10}`, "")
	require.Equal(t, http.StatusCreated, replay.Code)
	require.Equal(t, "true", replay.Header().Get(idempotency.HeaderReplayed))
	require.JSONEq(t, first.Body.String(), replay.Body.String())
	require.Equal(t, 1, *calls, "replay must not reach the downstream handler")
}

func TestMiddleware_UsesHeaderRequestIDBeforeBody(t *testing.T) {
	store := newMemoryStore()
	router, calls := newIdempotentRouter(store)

	first := postJSON(router, "/pay", `{"requestId":"body-id","amount":10}`, "header-id")
	require.Equal(t, http.StatusCreated, first.Code)

	replay := postJSON(router, "/pay", `{"requestId":"different-body","amount":20}`, "header-id")
	require.Equal(t, http.StatusCreated, replay.Code)
	require.Equal(t, "true", replay.Header().Get(idempotency.HeaderReplayed))
	require.JSONEq(t, first.Body.String(), replay.Body.String())
	require.Equal(t, 1, *calls)
}

func TestMiddleware_IsolatesSameRequestIDBySubject(t *testing.T) {
	store := newMemoryStore()
	router, calls := newIdempotentRouter(store)

	first := postJSONWithUser(router, "/pay", `{"requestId":"req-1","amount":10}`, "u1")
	secondUser := postJSONWithUser(router, "/pay", `{"requestId":"req-1","amount":10}`, "u2")
	replayFirstUser := postJSONWithUser(router, "/pay", `{"requestId":"req-1","amount":10}`, "u1")

	require.Equal(t, http.StatusCreated, first.Code)
	require.Equal(t, http.StatusCreated, secondUser.Code)
	require.Empty(t, secondUser.Header().Get(idempotency.HeaderReplayed))
	require.Equal(t, http.StatusCreated, replayFirstUser.Code)
	require.Equal(t, "true", replayFirstUser.Header().Get(idempotency.HeaderReplayed))
	require.Equal(t, 2, *calls)
}

func TestMiddleware_RejectsMissingRequestIDAndPendingDuplicate(t *testing.T) {
	store := newMemoryStore()
	router, calls := newIdempotentRouter(store)

	missing := postJSON(router, "/pay", `{"amount":10}`, "")
	require.Equal(t, http.StatusBadRequest, missing.Code)
	require.JSONEq(t, `{"message":"missing requestId"}`, missing.Body.String())
	require.Equal(t, 0, *calls)

	ok, err := store.SetNX(context.Background(), "test::busy", "__pending__", time.Minute)
	require.NoError(t, err)
	require.True(t, ok)

	pending := postJSON(router, "/pay", `{"requestId":"busy","amount":10}`, "")
	require.Equal(t, http.StatusConflict, pending.Code)
	require.JSONEq(t, `{"message":"duplicate requestId in-flight"}`, pending.Body.String())
	require.Equal(t, 0, *calls)
}

func newIdempotentRouter(store idempotency.Store) (*gin.Engine, *int) {
	calls := 0
	router := gin.New()
	router.POST(
		"/pay",
		idempotency.Middleware(idempotency.Config{
			Store:     store,
			KeyPrefix: "test:",
			SubjectFn: func(c *gin.Context) string {
				return c.GetHeader("X-User-Id")
			},
		}),
		func(c *gin.Context) {
			calls++
			var body map[string]any
			if err := c.ShouldBindJSON(&body); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
				return
			}
			c.JSON(http.StatusCreated, gin.H{"ok": true, "body": body})
		},
	)
	return router, &calls
}

func postJSON(router *gin.Engine, path, body, requestID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if requestID != "" {
		req.Header.Set(idempotency.HeaderRequestID, requestID)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func postJSONWithUser(router *gin.Engine, path, body, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", userID)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
