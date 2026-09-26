# GoLive Backend

The GoLive backend is a Go microservices project providing `../golive-web` with HTTP APIs,
WebSockets, stream callbacks, gift orders, chat processing, uploads, and admin console data.

## Technology stack

- Go 1.24+, Gin, gorilla/websocket, gRPC + Protobuf
- MySQL 8 + GORM, Redis 7, Kafka, etcd, MinIO
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

## Port conventions

| Component | Port | Description |
| ---- | ---- | ---- |
| nginx | 80 / 443 | Static files and `/api`, `/ws`, `/live` entry points |
| api-gateway | 8080 | HTTP JSON with the public `/api` path prefix |
| im-gateway | 8081 | WebSocket at `/ws` |
| SRS RTMP | 1935 | OBS publishing |
| SRS HTTP | 8080 inside the container | Proxied by nginx as `/live` |
| MySQL / Redis / Kafka / etcd / MinIO | Compose internal network | Must not be publicly exposed |
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
  pkg/                  Shared packages: JWT, idempotency, error codes, logging, uploads, content policies, etc.
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
