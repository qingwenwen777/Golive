// Package obs (observability) provides shared tracing + metrics helpers for
// every service. The design goals:
//
//   - one-line bootstrap from main.go:  shutdown := obs.InitTracing(name)
//   - graceful no-op when the OTLP endpoint is unset (so unit tests don't
//     try to connect anywhere)
//   - Prometheus exposition piggy-backed on each service's existing HTTP mux
//   - Gin HTTP middleware that records standard request metrics + a span
//
// Tracing is exported via OTLP/gRPC to an OTel collector (default
// 127.0.0.1:4317). The collector forwards to Jaeger.
package obs

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// InitTracing wires the global tracer provider. Pass the service name; the
// returned func should be deferred from main() to flush spans on shutdown.
//
// Endpoint resolution order:
//
//	OTEL_EXPORTER_OTLP_ENDPOINT  (e.g. "127.0.0.1:4317" — host:port, no scheme)
//	default → "127.0.0.1:4317"
//
// If the collector is unreachable we still return a working tracer (spans
// just get dropped) so a missing observability stack never blocks startup.
func InitTracing(serviceName string) func(context.Context) error {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "127.0.0.1:4317"
	}
	endpoint = normalizeOTLPEndpoint(endpoint)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	exp, err := otlptrace.New(ctx,
		otlptracegrpc.NewClient(
			otlptracegrpc.WithEndpoint(endpoint),
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		),
	)
	if err != nil {
		// Non-fatal: install a no-op-but-named tracer provider so callers
		// can still create spans without nil pointer panics.
		otel.SetTracerProvider(sdktrace.NewTracerProvider(
			sdktrace.WithResource(makeResource(serviceName)),
		))
		otel.SetTextMapPropagator(propagation.TraceContext{})
		return func(context.Context) error { return nil }
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp,
			sdktrace.WithMaxQueueSize(2048),
			sdktrace.WithMaxExportBatchSize(512),
			sdktrace.WithBatchTimeout(2*time.Second),
		),
		sdktrace.WithResource(makeResource(serviceName)),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	return func(ctx context.Context) error {
		shutdownCtx, c := context.WithTimeout(ctx, 5*time.Second)
		defer c()
		if err := tp.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	}
}

func normalizeOTLPEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "127.0.0.1:4317"
	}
	u, err := url.Parse(endpoint)
	if err == nil && u.Host != "" {
		return u.Host
	}
	return strings.TrimSuffix(endpoint, "/")
}

func makeResource(serviceName string) *sdkresource.Resource {
	return sdkresource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.DeploymentEnvironment(os.Getenv("DEPLOY_ENV")),
	)
}

// Tracer returns a named tracer scoped to the package emitting the spans.
// Convention: use a stable instrumentation name like "gift-service/handler".
func Tracer(instrumentation string) trace.Tracer {
	return otel.Tracer(instrumentation)
}
