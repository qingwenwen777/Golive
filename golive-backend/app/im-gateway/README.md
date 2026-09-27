# im-gateway

Gateway for persistent real-time messaging connections. Handles all live-room WebSockets, targeting approximately 50k concurrent connections per instance.

- HTTP/WS: `:8081`
- /metrics + /healthz + /debug/rooms share the same port.
- pprof: `127.0.0.1:6066` (loopback only; an empty `service.pprof_addr` disables it)

## Purpose and design rationale

| Concern | Design |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Connection management | `Hub` (rooms map) → `Room` (conns map) → `Conn` (per-connection send channel + two goroutines for read/write pumps). Two RWMutex layers; fanout takes a snapshot and releases the lock before network operations. |
| Cross-instance distribution | Redis Pub/Sub. `im-gateway` does not process chat business logic directly; chat-service / gift-service publish to `room:<id>`, and all im-gateway instances subscribe and fan out. |
| **Lazy subscriptions** | First connection enters a room → SUBSCRIBE; last connection leaves → UNSUBSCRIBE. All rooms share **one** Redis PubSub connection (go-redis re-subscribes after reconnects), so the number of rooms never multiplies Redis connections. The hub lock is not held across SUBSCRIBE; the reaper re-checks the connection count (owners included) under the room lock and marks the room closed so late joiners retry. |
| Slow consumers | Each `Conn.send` channel has capacity 256; **evict immediately when full** (nonblocking). One slow client cannot stall the other 50k viewers in a room. |
| Client→server | With `kafka.enabled=false` (production) the gateway itself moderates chat — per-user Redis rate limit (`pkg/chatlimit`), mute/ban state, admin blocked words, sensitive-word masking (`pkg/chatfilter`) — then publishes to `room:<id>`; chat-service persists passively. With Kafka on, `chat` goes to topic `danmu` (partition=roomId) and chat-service moderates, persists, and republishes. |
| Authentication | The client offers its access token as a second subprotocol, `auth.<jwt>`, next to `golive.v1`: browsers can't set other handshake headers, and URLs (query string included) end up in access logs. Missing/invalid → 401; a `token` query parameter → 400; valid → populate `Identity`. The handshake selects `golive.v1`, never echoing the token. |
| Handshake limits | `roomId` must match `^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$` (400) and, with `room.require_known`, exist (404): Redis `room:owner:<id>` (written by room-service when a room goes live; also the trusted owner id — the client's `ownerId` is ignored for known rooms), else a cached, rate-limited room-service `GET /rooms/<id>`. Concurrent connections are capped per user (`ws.max_conns_per_user`) and per client IP (`ws.max_conns_per_ip`, X-Real-IP honoured only from `ws.trusted_proxies`) → 429. The caps are per instance. |
| Heartbeats/timeouts | `ReadIdleTimeout=60s`, renewed by any frame including heartbeat and pong; `WriteDeadline=10s`; `ReadLimit=4KB` prevents abuse. |
| Metrics | Prometheus: connection/room counts, sent/received counts by type, drop reasons, and broadcast latency histogram. Top-N data lives separately at `/debug/rooms` to avoid high-cardinality roomId labels. |

## Protocol (matching frontend ws-server.ts)

Handshake: `ws://host:8081/ws?roomId=<id>` with `Sec-WebSocket-Protocol: golive.v1, auth.<jwt>`
(browser: `new WebSocket(url, ['golive.v1', 'auth.' + jwt])`); the response selects `golive.v1`.

| Direction | type | Fields |
| ---------- | --------------- | --------------------------------------------------------------------- |
| S→C        | `system`        | `text, ts(ms)`                                                        |
| S→C        | `viewer_count`  | `count`                                                               |
| S→C        | `chat`          | `id, userId, user, avatar?, text, role?, fanBadge?, userLevel?, ts` — all but `text` server-derived (id = server uuid; name/avatar/level from user-service, fan badge from chat-service, cached 30s) |
| S→C        | `chat_ack`      | `clientId, id` — only to the sender, mapping its `clientId` to the server id |
| S→C        | `super_chat`    | `id, user, avatar?, amount, tier(0–5), text, ts`                      |
| S→C        | `gift`          | `user, giftName, ts`                                                  |
| C→S        | `heartbeat`     | —                                                                     |
| C→S | `resume` | `lastMessageId?` (MVP only replies with `system: "resumed"`; no replay buffer) |
| C→S | `chat` | `text` (≤200 chars), `clientId?`. Identity/display fields a client sends (`user`, `avatar`, `userLevel`, `fanBadge`, ...) are ignored. Per-user limit `chat_ratelimit`; every frame type also counts against `ws.max_message_rate` per connection. |
| C→S | `viewer_profile` | — (payload ignored; asks the gateway to refetch the user's server-side profile, at most once per 5s per user). Identity is resolved when it is used (chat, viewer list), not kept per connection; a fallback name from a failed lookup is retried on a later frame. |

Immediately after the handshake, send one `system: "Welcome to the live room!"` and one `viewer_count: 1`, matching the frontend mock.

## Startup

```bash
docker compose -f deploy/docker-compose.yml up -d redis
go run ./app/im-gateway/cmd      # :8081

# Inspect metrics.
curl http://localhost:8081/metrics | head
curl http://localhost:8081/debug/rooms
```

## Tests

```bash
go test ./app/im-gateway/...
```

Coverage:

- **hub**: lazy subscription only on first entry / unsubscribe on last departure / same-room fanout / room isolation /
  slow-consumer eviction / periodic viewer_count pushes / Snapshot Top-N sorting.
- **dispatch**: silent heartbeat / resume acknowledgment / reject unauthenticated chat / send authenticated chat to the producer /
  rate-limit cap / discard oversized chat / discard unknown types.
- **auth**: reject empty tokens / authenticate valid tokens / reject invalid signatures / reject incorrect secrets.
- **handshake**: token only via the `auth.<jwt>` subprotocol (URL token → 400, bad token → 401, guests → 401) /
  `golive.v1` selected, token never echoed / malformed and unknown rooms / owner from the room directory / connection caps.

## Load testing

### k6 (recommended)

```bash
k6 run \
  -e WS_URL=ws://localhost:8081/ws \
  -e ROOMS=20 \
  -e PER_VU=1000 \
  -u 50 -d 5m \
  app/im-gateway/loadtest/connect.js
```

50 VUs × 1000 connections = 50k concurrent connections for 5 minutes. Threshold: handshake p95 < 200ms.

### Native Go (when k6 is unavailable)

```bash
go run -tags loadtest ./app/im-gateway/loadtest \
  -url ws://localhost:8081/ws \
  -conns 50000 -rooms 20 -hold 60s -dial 512
```

Reports handshake p50/p95/p99, success/failure counts, and total messages received.

### System tuning (required before testing 50k connections)

Linux:
```bash
sysctl -w fs.file-max=2000000
sysctl -w net.ipv4.ip_local_port_range="10000 65535"
sysctl -w net.core.somaxconn=65535
ulimit -n 1048576
```

Service settings: `GOMAXPROCS=$(nproc)`; enable pprof and inspect goroutine and heap profiles.

## Frontend integration

Set `golive-web/.env.development.local` to:

```env
VITE_ENABLE_MSW=false
VITE_WS_BASE=ws://localhost:8081/ws
```

Skip `startMockWsServer()` in `src/main.tsx`. LiveRoomPage connects directly to the real WebSocket.

## Integration with gift-service / chat-service

Two next steps:

1. **chat-service**: consume Kafka `danmu`, filter sensitive words, write to the MySQL `danmaku` table,
   then `PUBLISH room:<id>` a `chat` message. im-gateway receives it through its subscription and fans it out.
2. **gift-service**: after a successful debit, `PUBLISH room:<id>` a `gift` or
   `super_chat` message. im-gateway fans it out in the same way.

Complete call chain:
```
client → ws → im-gateway →(produce) kafka → chat-service →(publish) redis
                                                              ↓
                                  im-gateway[N instances] (subscribe & fanout)
                                                              ↓
                                                          All WS clients
```
