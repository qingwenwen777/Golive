# Observability Stack

Three pillars wired:

| Tool                        | Endpoint                              | What it does                              |
| --------------------------- | ------------------------------------- | ----------------------------------------- |
| **Prometheus**              | http://localhost:9090                 | Scrapes `/metrics` from every service.    |
| **Grafana**                 | http://localhost:3000 (admin/admin)   | Dashboards. Auto-loads `golive.json`.     |
| **Jaeger**                  | http://localhost:16686                | Trace search by service / operation.      |
| **OTel Collector**          | grpc :4317  http :4318                | Receives spans → forwards to Jaeger.      |

## 启动

```bash
cd deploy
docker compose up -d prometheus grafana jaeger otel-collector
```

各业务服务（go run）默认走 `OTEL_EXPORTER_OTLP_ENDPOINT=127.0.0.1:4317`；
collector down 也不会卡启动（pkg/obs.InitTracing 自动降级为 noop）。

## Prometheus 指标矩阵

| 指标                                | 标签                       | 来源                  |
| ----------------------------------- | -------------------------- | --------------------- |
| `http_requests_total`               | service, route, method, code | 所有 Gin 服务         |
| `http_request_duration_seconds`     | service, route, method     | 所有 Gin 服务         |
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

**有意不做** roomId / userId 维度的 label —— Prometheus 高基数会很快把 TSDB 撑爆。
高基数的"Top N 房间消息速率"放在 im-gateway `/debug/rooms` JSON 里。

## 端到端 trace

走一条 `用户点送礼 → 扣款 → 本地消息表 → Kafka` 的链路。Jaeger 里搜 service=`api-gateway` operation=`POST /api/gifts/*action`，能看到：

```
api-gateway: POST /api/gifts/*action   ← 网关入口（HTTP 中间件起 span）
 └─ (HTTP 反代，traceparent 头透传)
    └─ gift-service: POST /gifts/send
        └─ gift.send                   ← service.GiftService.Send 起 span
            (attributes: user.id, room.id, gift.id, gift.count, request.id,
                          idempotent.replayed, order.id, order.total_coin)
```

**outbox worker** 是异步的，独立起 trace（`outbox.publish`），通过 attribute `biz.id` (= orderId) 与上面那条 trace 关联。Jaeger 用 "Find traces by tag" `biz.id=gift-xxx` 即可串起来。

## 添加新服务的 instrumentation

```go
import "github.com/qingwenwen777/golive/pkg/obs"

shutdown := obs.InitTracing("my-service")
defer func() { _ = shutdown(ctx) }()

r := gin.New()
r.Use(gin.Recovery())
r.Use(obs.HTTPMiddleware("my-service"))
obs.MountMetrics(r)
```

业务侧手动 span：

```go
ctx, span := obs.Tracer("my-service/handler").Start(ctx, "my.op")
defer span.End()
span.SetAttributes(attribute.String("k", "v"))
```

## 跑一遍验证

1. `docker compose up -d`
2. `go run ./app/api-gateway/cmd` etc.
3. `curl http://localhost:8080/api/auth/login -d ... -X POST`
4. http://localhost:9090 → `http_requests_total` 应有数据
5. http://localhost:3000 → "GoLive Overview" 自动出现
6. http://localhost:16686 → service `api-gateway` 能搜到 traces
