# GoLive Backend

The GoLive backend is a Go microservices project providing `../golive-web` with HTTP APIs,
WebSockets, stream callbacks, gift orders, chat processing, uploads, and admin console data.

## Technology stack

- Go 1.25+, Gin, gorilla/websocket, gRPC + Protobuf
- MySQL 8 + GORM, Redis 7, optional Kafka (Compose profile `kafka`; off by default)
- Viper, zap, OpenTelemetry, Jaeger, Prometheus, Grafana
- SRS 5(RTMP → HTTP-FLV/HLS), Docker Compose, nginx

## Services

| Service | Entry point | Description |
| ---- | ---- | ---- |
| api-gateway | `app/api-gateway/cmd` | Public HTTP gateway, authentication, CSRF, rate limiting, reverse proxy |
| user-service | `app/user-service/cmd` | Sign-in, Google sign-in, profiles, avatars/covers, coins, creator applications |
| room-service | `app/room-service/cmd` | Rooms, going live, appointments, channel posts, follows/likes, replays, moderation |
| chat-service | `app/chat-service/cmd` | Chat rate limiting, sensitive-word filtering, persistence, chat history |
| gift-service | `app/gift-service/cmd` | Gifts, SuperChat, betting, idempotent orders, transactional outbox |
| im-gateway | `app/im-gateway/cmd` | Persistent WebSocket connections, room fanout, Redis Pub/Sub |

## Data ownership

All services share one MySQL database, but each table is owned by the service whose models
and `AutoMigrate` define it. Only the owner writes its tables; another service that needs a
change calls the owner's `/internal/...` HTTP API. Those routes are never proxied by nginx or
api-gateway (which only forwards `/api/<prefix>/*` to `/<prefix>/*` and drops client-sent
`X-Internal-Token`), and every call must carry `X-Internal-Token` matching the shared
`GOLIVE_INTERNAL_TOKEN` (compared in constant time; an unset token rejects all calls).

| Internal API | Owner | Caller |
| ---- | ---- | ---- |
| `POST /internal/users/:id/restriction` (ban/unban/mute/unmute; a ban revokes refresh tokens) | user-service | room-service report moderation |
| `GET /internal/users/:id/permission` | user-service | room-service (HTTP fallback to gRPC) |
| `POST /internal/super-chats/:id/moderation` (sets `moderated_at`, keeps `status`, no refund) | gift-service | room-service report moderation |
| `DELETE /internal/rooms/:id/danmus/:danmuId` (soft delete) | chat-service | room-service report moderation |
| `GET /internal/rooms/:id/fan-badges/:userId` | chat-service | im-gateway |

Cross-service reads remain (direct SQL on the shared database):

| Table | Owner | Also read by |
| ---- | ---- | ---- |
| `users` | user-service | room-service, gift-service, chat-service |
| `user_moderation_states` | user-service | room-service (ban/mute checks) |
| `coin_transactions` | user-service + gift-service (wallet) | room-service (admin overview), chat-service (user level) |
| `rooms` | room-service | user-service, gift-service, chat-service |
| `room_watch_events` | room-service | user-service (daily tasks) |
| `content_reports` | room-service | user-service (admin user detail) |
| `fan_badges` | gift-service | room-service, chat-service |
| `gift_orders` | gift-service | room-service (analytics, recommendations) |
| `super_chat_orders` | gift-service | room-service (analytics, report targets), chat-service (history, hides `moderated_at`) |
| `danmus_<n>` | chat-service | room-service (report targets, chat counts) |

Cross-service writes that remain, by design or pending a decision:

- Coin wallet: `users.coin_balance` / `users.frozen_coins` and `coin_transactions` are written by
  both user-service and gift-service inside single transactions (undecided; not moved).
- `rooms.avatar` is synced by user-service when a creator changes their avatar.
- `user_moderation_states`, `unban_appeals` and `admin_audit_logs` are migrated by both
  user-service and room-service; room-service still creates unban appeals and appends its own
  admin audit entries.
- Redis ban/mute flags (`contentpolicy` keys) are refreshed by both user-service and
  room-service from `user_moderation_states`.

## Port conventions

| Component | Port | Description |
| ---- | ---- | ---- |
| nginx | 80 / 443 | Static files and `/api`, `/ws`, `/live` entry points |
| api-gateway | 8080 | HTTP JSON with the public `/api` path prefix |
| im-gateway | 8081 | WebSocket at `/ws` |
| SRS RTMP | 1935 | OBS publishing |
| SRS HTTP | 8080 inside the container | Proxied by nginx as `/live` |
| MySQL / Redis / Kafka (optional) | Compose internal network | Must not be publicly exposed |
| Grafana / Prometheus / Jaeger | Bound to 127.0.0.1 | Access through an SSH tunnel |

## Directory structure

```text
golive-backend/
  api/
    proto/              Protobuf definitions
    gen/go/             Generated Go code
  app/
    <service>/
      cmd/              Service entry point
      configs/          Local example configuration
      internal/         handler/server/service/repo/model/config
      README.md         Service documentation
  deploy/
    docker-compose.yml  Local and server runtime stack
    configs/            Service configurations used by Compose
    nginx*.conf         HTTP/HTTPS entry points
    srs.conf            SRS configuration
    observability/      Prometheus, OTel, Grafana
  pkg/                  Shared packages: JWT, error codes, logging, HTTP server timeouts, uploads, content policies, etc.
```

## Run locally

The recommended approach is to use the one-command script from the repository root:

```bash
bash scripts/dev.sh
# Or on Windows:
powershell -File scripts/dev.ps1
```

Start only the backend stack:

```bash
cd golive-backend/deploy
docker compose up -d
```

Compose builds the Go services into container images using `deploy/Dockerfile.service`.
After changing code, run `docker compose up -d --build` to rebuild and update the service containers.

## Tests

```bash
go test ./...
```

You can also limit the scope to individual services:

```bash
go test ./app/api-gateway/...
go test ./app/room-service/...
go test ./app/user-service/...
go test ./app/chat-service/...
go test ./app/gift-service/...
go test ./app/im-gateway/...
```

## Coin wallet

Balances live in the shared database (`users.coin_balance`, `users.frozen_coins`) with the
`coin_transactions` ledger. All changes go through `pkg/wallet` (`Debit`, `AdminDebit`, `Credit`,
`Freeze`, `Unfreeze`), called with the caller's GORM transaction so the balance change, its ledger
row and the caller's own writes (order, outbox) commit together. No other code may `UPDATE` these
columns or write `coin_transactions`; `go test ./pkg/wallet/` scans the module and fails on direct
writes. Creating a user row with an opening balance (sign-up bonus, seeded accounts) is not a
balance change and stays in user-service.

## Frontend contract

- The frontend uses same-origin requests by default: `VITE_API_BASE=/api`, `VITE_WS_BASE=/ws`,
  `VITE_FLV_BASE=/live`.
- JWT authentication uses `Authorization: Bearer <token>` and does not depend on cookies.
- Write operations require a CSRF token; the frontend fetches it from `/api/csrf-token` and retries once automatically.
- `/api/gifts/send` and `/api/super-chats` are idempotent by `X-Request-Id`; replay responses include
  `Idempotent-Replayed: true`.
- Insufficient balance returns HTTP 402 with `reason=insufficient_coin`.
- Stream information returned to viewers must not expose `streamKey`.
- WebSocket handshake: `/ws?roomId=<id>&token=<jwt>`.

See `../docs/integration.md` for integration details and `../docs/deploy-git-bare.md` for deployment instructions.
