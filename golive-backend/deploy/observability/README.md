# Observability Stack

Three pillars wired:

| Tool                        | Endpoint                              | What it does                              |
| --------------------------- | ------------------------------------- | ----------------------------------------- |
| **Prometheus**              | http://localhost:9090                 | Scrapes `/metrics` from every service.    |
| **Grafana**                 | http://localhost:3000 (admin/admin)   | Dashboards. Auto-loads `golive.json`.     |
| **Jaeger**                  | http://localhost:16686                | Trace search by service / operation.      |
| **OTel Collector**          | grpc :4317  http :4318                | Receives spans → forwards to Jaeger.      |

## Startup

```bash
cd deploy
docker compose up -d prometheus grafana jaeger otel-collector
```

Business services started with `go run` default to `OTEL_EXPORTER_OTLP_ENDPOINT=127.0.0.1:4317`.
An unavailable collector does not block startup (`pkg/obs.InitTracing` falls back to a no-op automatically).

## Prometheus metrics matrix

| Metric | Labels | Source |
| ----------------------------------- | -------------------------- | --------------------- |
| `http_requests_total` | service, route, method, code | All Gin services |
| `http_request_duration_seconds` | service, route, method | All Gin services |
| `im_connections_active`             | —                          | im-gateway            |
| `im_rooms_active`                   | —                          | im-gateway            |
| `im_messages_sent_total`            | type                       | im-gateway            |
| `im_messages_received_total`        | type                       | im-gateway            |
| `im_messages_dropped_total`         | reason (slow_consumer / rate_limited / anonymous_chat / bad_json …) | im-gateway |
| `im_broadcast_latency_seconds`      | (histogram)                | im-gateway            |
| `gift_revenue_coin_total`           | status (success/failed)    | gift-service          |
| `super_chat_revenue_coin_total`     | tier, status               | gift-service          |
| `outbox_published_total`            | topic                      | gift-service outbox   |
| `outbox_failed_total`               | topic                      | gift-service outbox   |
| `outbox_rows`                       | status (pending/sent/dead) | gift-service          |
| `danmu_processed_total`             | outcome                    | chat-service          |
| `kafka_produced_total`              | service, topic, outcome    | gift / im / chat      |
| `kafka_consumed_total`              | service, topic, outcome    | chat-service          |

roomId / userId labels are **deliberately omitted**: high cardinality can quickly overwhelm the Prometheus TSDB.
High-cardinality data such as Top-N room message rates is exposed in im-gateway `/debug/rooms` JSON instead.

## End-to-end tracing

Follow the `user sends a gift → debit → transactional outbox → Kafka` flow. In Jaeger, search for service=`api-gateway`, operation=`POST /api/gifts/*action` to see:

```
api-gateway: POST /api/gifts/*action   ← Gateway entry point (HTTP middleware starts a span)
 └─ (HTTP reverse proxy forwards the traceparent header)
    └─ gift-service: POST /gifts/send
        └─ gift.send                   ← service.GiftService.Send starts a span
            (attributes: user.id, room.id, gift.id, gift.count, request.id,
                          idempotent.replayed, order.id, order.total_coin)
```

The **outbox worker** runs asynchronously and starts a separate trace (`outbox.publish`). Attribute `biz.id` (= orderId) links it to the trace above. In Jaeger, use "Find traces by tag" with `biz.id=gift-xxx` to find the related traces.

## Instrument a new service

```go
import "github.com/qingwenwen777/golive/pkg/obs"

shutdown := obs.InitTracing("my-service")
defer func() { _ = shutdown(ctx) }()

r := gin.New()
r.Use(gin.Recovery())
r.Use(obs.HTTPMiddleware("my-service"))
obs.MountMetrics(r)
```

Create a span manually in business logic:

```go
ctx, span := obs.Tracer("my-service/handler").Start(ctx, "my.op")
defer span.End()
span.SetAttributes(attribute.String("k", "v"))
```

## Verification walkthrough

1. `docker compose up -d`
2. `go run ./app/api-gateway/cmd` etc.
3. `curl http://localhost:8080/api/auth/login -d ... -X POST`
4. http://localhost:9090 → `http_requests_total` should contain data.
5. http://localhost:3000 → "GoLive Overview" appears automatically.
6. http://localhost:16686 → traces can be found for service `api-gateway`.
