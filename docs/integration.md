# Frontend and Backend Integration Notes

Current integration model: `golive-web` connects directly to the real Go backend by default.
Both the Vite development server and production nginx use the same-origin `/api`, `/ws`, and `/live` entry points.

## Topology

```text
Browser
  ├─ /              -> golive-web/dist
  ├─ /api/*         -> api-gateway:8080
  ├─ /ws            -> im-gateway:8081
  └─ /live/*        -> srs:8080/live/*

docker compose:
  nginx, api-gateway, user-service, room-service, chat-service,
  gift-service, im-gateway, SRS, MySQL, Redis, Kafka, etcd, MinIO,
  Prometheus, Grafana, Jaeger, OTel Collector
```

## One-command startup

Run from the repository root:

```bash
# Windows PowerShell
powershell -File scripts/dev.ps1

# Linux/macOS/WSL/Git Bash
bash scripts/dev.sh
```

The scripts build the frontend, start Compose, poll health checks, and print local URLs.

## Frontend environment variables

| Variable | Development and production default | Description |
| ---- | ---------------- | ---- |
| `VITE_API_BASE` | `/api` | axios baseURL |
| `VITE_WS_BASE` | `/ws` | Live-room WebSocket |
| `VITE_FLV_BASE` | `/live` | HTTP-FLV playback |
| `VITE_RTMP_BASE` | `rtmp://localhost/live` or the production domain | OBS publishing hint |

Vite `server.proxy` forwards these relative paths to local backend ports. In production, nginx
proxies the same paths, so the code does not need two sets of base URLs.

## Common pitfalls

### Relative WebSocket paths

Browsers cannot directly use `new WebSocket('/ws')`. When `VITE_WS_BASE` starts with `/`,
the frontend `useRoomRealtime.ts` resolves it using the current page protocol to `ws://host/ws` or
`wss://host/ws`.

### HTTP-FLV paths

The frontend requests `/live/<streamKey>.flv`. The nginx `location /live/` forwards requests
to `http://srs:8080/live/` in the SRS container. Keep the trailing slash on both paths
to avoid accidentally producing `/live/live/...`.

### Tokens and CSRF

`useAuthStore` persists token and refreshToken in `localStorage`. The axios interceptor
adds `Authorization: Bearer <token>` and automatically calls `/api/auth/refresh` on a 401.

Write operations first fetch `/api/csrf-token` and include the CSRF header. On
`reason=csrf_invalid`, the frontend clears the cache and retries once automatically.

### Idempotent requests

Gift and SuperChat requests require a stable `X-Request-Id`. For a replay, the backend returns
`Idempotent-Replayed: true`, allowing the frontend to distinguish new execution from an idempotent replay.

## User journey checklist

| Step | Action | Success indicator | Troubleshooting entry point |
| ---- | ---- | -------- | -------- |
| 1 Sign in | `/login` or the login dialog | 200; token stored in localStorage | `api-gateway` and `user-service` logs |
| 2 Home feed | `/` | Live cards render | `room-service` logs; MySQL/Redis status |
| 3 Enter a live room | `/live/:id` | Room details load; WS readyState=1 | `im-gateway` logs; token; roomId |
| 4 Play stream | Load the player | FLV request returns 200 | Check for an SRS stream and nginx `/live/` configuration |
| 5 Send chat | Send from the input field | Own message appears; room fanout | im-gateway -> Kafka -> chat-service -> Redis |
| 6 Send a gift | Gift panel | Order succeeds; animation appears | `gift-service` logs; coin balance; requestId |
| 7 Follow/like | Room actions | Optimistic UI update agrees with the API | `room-service` social endpoints |
| 8 Creator goes live | `/studio/prepare` | streamKey and RTMP URL returned | room-service; SRS on_publish callback |
| 9 Admin console | `/admin/dashboard` | Metrics and moderation lists load | api-gateway; room/user/gift service |
| 10 Reconnect | Enable DevTools Offline, then restore connectivity | WS reconnects automatically; messages recover | Browser console; im-gateway close/reconnect |

## Common troubleshooting commands

```bash
cd golive-backend/deploy
docker compose ps
docker compose logs --tail=200 nginx
docker compose logs --tail=200 api-gateway
docker compose logs --tail=200 im-gateway
docker compose logs --tail=200 room-service
docker compose logs --tail=200 chat-service
docker compose logs --tail=200 gift-service
```

Health checks:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8081/healthz
curl http://localhost:8082/api/v1/versions
```
