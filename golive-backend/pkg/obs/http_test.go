package obs

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	require.NoError(t, c.Write(&m))
	return m.GetCounter().GetValue()
}

func TestHTTPMiddlewareBoundsMethodLabel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(HTTPMiddleware("obs-test"))
	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	// The counters are process-wide, so compare against their values before
	// the requests (go test -count=N reruns this in the same process).
	other := httpRequests.WithLabelValues("obs-test", "unmatched", "OTHER", "404")
	get := httpRequests.WithLabelValues("obs-test", "/ok", http.MethodGet, "200")
	otherBefore, getBefore := counterValue(t, other), counterValue(t, get)
	for _, m := range []string{"FOO", "BAR", "BAZ", http.MethodGet} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(m, "/ok", nil))
	}

	require.Equal(t, 3.0, counterValue(t, other)-otherBefore)
	require.Equal(t, 1.0, counterValue(t, get)-getBefore)
}
