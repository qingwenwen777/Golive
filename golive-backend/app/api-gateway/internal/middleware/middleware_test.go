package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/api-gateway/internal/middleware"
)

func init() { gin.SetMode(gin.TestMode) }

// helper: HS256 access token with sub=uid, exp=+1h
func signToken(t *testing.T, secret, uid string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": uid,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte(secret))
	require.NoError(t, err)
	return s
}

// requestid -------------------------------------------------------------

func TestRequestID_Generates(t *testing.T) {
	r := gin.New()
	r.Use(middleware.RequestID())
	var seen string
	r.GET("/x", func(c *gin.Context) {
		seen = c.GetHeader("X-Request-Id") // request-side
		c.String(200, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/x", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	require.NotEmpty(t, seen, "should generate id when client omits it")
	require.Equal(t, seen, w.Header().Get("X-Request-Id"), "echo same id back")
}

func TestRequestID_PassesThrough(t *testing.T) {
	r := gin.New()
	r.Use(middleware.RequestID())
	r.GET("/x", func(c *gin.Context) {
		require.Equal(t, "client-supplied", c.GetHeader("X-Request-Id"))
		c.String(200, "ok")
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/x", nil)
	req.Header.Set("X-Request-Id", "client-supplied")
	r.ServeHTTP(w, req)
	require.Equal(t, "client-supplied", w.Header().Get("X-Request-Id"))
}

// jwt -------------------------------------------------------------------

func TestJWT_RejectsMissingToken(t *testing.T) {
	r := gin.New()
	r.Use(middleware.JWT("secret", nil))
	r.GET("/api/protected", func(c *gin.Context) { c.String(200, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/protected", nil))
	require.Equal(t, 401, w.Code)
	require.Contains(t, w.Body.String(), "Unauthorized")
}

func TestJWT_AcceptsValidAndInjectsHeader(t *testing.T) {
	r := gin.New()
	r.Use(middleware.JWT("secret", nil))
	r.GET("/api/me", func(c *gin.Context) {
		// The middleware should have rewritten the inbound header, NOT set
		// it on the response. Read off c.Request.Header to confirm.
		require.Equal(t, "user-42", c.Request.Header.Get(middleware.HeaderUserID))
		c.String(200, "ok")
	})
	tok := signToken(t, "secret", "user-42")
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
}

func TestJWT_StripsForgedUserIDHeader(t *testing.T) {
	r := gin.New()
	r.Use(middleware.JWT("secret", nil))
	r.GET("/api/me", func(c *gin.Context) {
		require.Equal(t, "real-user", c.Request.Header.Get(middleware.HeaderUserID))
		c.String(200, "ok")
	})
	tok := signToken(t, "secret", "real-user")
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("X-User-Id", "attacker") // should be removed
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
}

func TestJWT_PublicRouteSkipsValidation(t *testing.T) {
	r := gin.New()
	r.Use(middleware.JWT("secret", []middleware.PublicRoute{
		{Method: "POST", Path: "/api/auth/*action"},
	}))
	r.POST("/api/auth/*action", func(c *gin.Context) { c.String(200, "ok") })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/auth/login", nil))
	require.Equal(t, 200, w.Code)
}

// cors ------------------------------------------------------------------

func TestCORS_Preflight(t *testing.T) {
	r := gin.New()
	r.Use(middleware.CORS([]string{"http://localhost:5173"}, 600))
	r.POST("/api/x", func(c *gin.Context) { c.String(200, "ok") })

	req := httptest.NewRequest("OPTIONS", "/api/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, "http://localhost:5173", w.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	require.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	require.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
	require.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "X-Request-Id")
	require.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "X-CSRF-Token")
	require.Contains(t, w.Header().Get("Access-Control-Expose-Headers"), "Idempotent-Replayed")
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	r := gin.New()
	r.Use(middleware.CORS([]string{"http://localhost:5173"}, 600))
	r.GET("/x", func(c *gin.Context) { c.String(200, "ok") })

	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// Request still 200 (CORS isn't enforced server-side) but no ACAO header.
	require.Equal(t, 200, w.Code)
	require.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

// csrf ------------------------------------------------------------------

func TestCSRF_AllowsBrowserUnsafeRequestWithToken(t *testing.T) {
	r := gin.New()
	csrf, err := middleware.NewCSRFProtector("csrf-secret", time.Hour, []string{"http://localhost:5173"})
	require.NoError(t, err)
	r.Use(csrf.Guard())
	r.GET("/api/csrf-token", csrf.Token)
	r.POST("/api/x", func(c *gin.Context) { c.String(200, "ok") })

	tokenReq := httptest.NewRequest("GET", "/api/csrf-token", nil)
	tokenReq.Header.Set("Origin", "http://localhost:5173")
	tokenResp := httptest.NewRecorder()
	r.ServeHTTP(tokenResp, tokenReq)
	require.Equal(t, http.StatusOK, tokenResp.Code)
	token := tokenResp.Body.String()
	require.Contains(t, token, "token")

	req := httptest.NewRequest("POST", "/api/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set(middleware.HeaderCSRFToken, extractJSONToken(token))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestCSRF_RejectsBrowserUnsafeRequestWithoutToken(t *testing.T) {
	r := gin.New()
	csrf, err := middleware.NewCSRFProtector("csrf-secret", time.Hour, []string{"http://localhost:5173"})
	require.NoError(t, err)
	r.Use(csrf.Guard())
	r.POST("/api/x", func(c *gin.Context) { c.String(200, "ok") })

	req := httptest.NewRequest("POST", "/api/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "csrf_invalid")
}

func TestCSRF_AllowsServerToServerWithoutOrigin(t *testing.T) {
	r := gin.New()
	csrf, err := middleware.NewCSRFProtector("csrf-secret", time.Hour, []string{"http://localhost:5173"})
	require.NoError(t, err)
	r.Use(csrf.Guard())
	r.POST("/api/srs/callback", func(c *gin.Context) { c.String(200, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/srs/callback", nil))

	require.Equal(t, http.StatusOK, w.Code)
}

func extractJSONToken(body string) string {
	var payload struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal([]byte(body), &payload)
	return payload.Token
}
