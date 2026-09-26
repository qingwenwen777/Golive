package obs

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP request count.",
	}, []string{"service", "route", "method", "code"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 14), // 1ms .. ~16s
	}, []string{"service", "route", "method"})
)

// HTTPMiddleware returns a Gin middleware that:
//   - extracts W3C trace context from the inbound request and starts a span
//   - records request count + duration metrics on completion
//
// Use route templates (FullPath()) as the label, NOT the raw URL — the
// latter blows up Prometheus cardinality on path parameters.
func HTTPMiddleware(serviceName string) gin.HandlerFunc {
	tracer := otel.Tracer("gin." + serviceName)
	prop := otel.GetTextMapPropagator()
	return func(c *gin.Context) {
		ctx := prop.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		method := metricMethod(c.Request.Method)
		spanName := method + " " + route
		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(method),
				semconv.HTTPRoute(route),
				semconv.URLPath(c.Request.URL.Path),
				semconv.UserAgentOriginal(c.Request.UserAgent()),
			),
		)
		c.Request = c.Request.WithContext(ctx)

		start := time.Now()
		c.Next()
		elapsed := time.Since(start).Seconds()

		status := c.Writer.Status()
		span.SetAttributes(semconv.HTTPResponseStatusCode(status))
		if status >= 500 {
			span.SetAttributes(attribute.String("error", "true"))
		}
		span.End()

		httpRequests.WithLabelValues(serviceName, route, method, strconv.Itoa(status)).Inc()
		httpDuration.WithLabelValues(serviceName, route, method).Observe(elapsed)
	}
}

// metricMethod maps the client-controlled HTTP method onto a fixed set so
// arbitrary method tokens can't create unbounded metric series.
func metricMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return method
	default:
		return "OTHER"
	}
}

// MountMetrics adds /metrics to the given Gin engine. Call once per service.
func MountMetrics(r *gin.Engine) {
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
}
