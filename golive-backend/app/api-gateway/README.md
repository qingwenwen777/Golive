# api-gateway

Edge gateway. All `http://localhost:8080/api/*` requests are routed through it to downstream services.

- HTTP: `:8080`
- pprof: `127.0.0.1:6060` (loopback only; an empty `service.pprof_addr` disables it)

## Middleware chain

```
Recovery → CORS → RequestID → RateLimit → JWT → ReverseProxy
```

| Middleware | Purpose |
| ---------- | ------------------------------------------------------------------------------------- |
| CORS | `localhost:5173` + a placeholder production domain; `OPTIONS` preflight returns 204 directly; exposes `Idempotent-Replayed` |
| RequestID | Generates a UUID when `X-Request-Id` is missing, writes it to the response header, and injects it into the forwarded request |
| RateLimit | Per-IP token bucket (`golang.org/x/time/rate`), default 100 req/s, burst 200 |
| JWT | Required outside the allowlist; injects `X-User-Id` into the forwarded request after validation; strips client-forged X-User-Id |
| Proxy | Strips `/api`; 10s timeout; wraps 5xx in the standard error body; passes 4xx through unchanged |

## Routes

| Path                                  | Upstream      | Auth   |
| ------------------------------------- | ------------- | ------ |
| `/api/auth/*` | user-service | Public |
| `/api/users/*`                        | user-service  | JWT    |
| `/api/rooms`, `/api/rooms/*` | room-service | Public GET / JWT for writes |
| `/api/gifts` | gift-service | Public |
| `/api/gifts/*`, `/api/super-chats`    | gift-service  | JWT    |

SRS HTTP hooks are intentionally not exposed through the public gateway.
`deploy/srs.conf` calls room-service on the Compose internal network instead.

## Error response rules

- Downstream `2xx` / `3xx` / `4xx`: **pass through unchanged**, including the body. In particular,
  402 `{message:"insufficient coin", reason:"insufficient_coin"}` must pass through the gateway.
- Downstream `5xx` with a body already shaped as `{message,...}`: preserve it unchanged.
- Downstream `5xx` with a nonconforming body (HTML, empty, or plain text): wrap it as
  `{"message":"Internal error","reason":"upstream_unavailable"}`.
- Unreachable or timed-out upstream: `502` / `504` with the same body as above.

## Configuration

Use `configs/config.yaml`, with an optional `--config` override. Upstream addresses are initially static;
this is the only place to change when integrating etcd service discovery later.

## Startup

```bash
cd deploy && docker compose up -d mysql redis     # Dependencies
cd ../app/user-service && go run ./cmd            # :8090
cd ../room-service && go run ./cmd                # :8091
# gift-service is planned for the next iteration; its absence does not affect auth/rooms.
cd ../api-gateway && go run ./cmd                 # :8080
```

## Tests

```bash
cd app/api-gateway

# Middleware unit tests
go test ./internal/middleware/...

# Router integration: spy upstream; verify routing, JWT injection, RequestId forwarding, and 5xx wrapping.
go test ./internal/router/...
```

Coverage:

- `RequestID`: generate when missing; forward when supplied.
- `JWT`: reject missing/forged tokens; accept valid tokens; strip forged `X-User-Id`; skip public routes.
- `CORS`: preflight returns 204 and correct headers; origins outside the allowlist receive no ACAO.
- Routing: strip `/api`; preserve query strings; inject `X-User-Id` after JWT validation.
- Errors: pass through 4xx; wrap non-JSON 5xx; preserve conforming 5xx; return 502/504 for unreachable upstreams.

## End-to-end curl examples

```bash
# Sign in (public).
curl -i -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"demo"}' | tee /tmp/login.txt
# Check response headers: Access-Control-Allow-Origin / X-Request-Id.

TOKEN=$(jq -r .token < <(grep -A100 '^{' /tmp/login.txt))

# Protected: follow a channel (the gateway injects X-User-Id into the room-service request after JWT validation).
curl -i -X POST http://localhost:8080/api/rooms/luna/follow \
  -H "Authorization: Bearer $TOKEN"

# A protected route without a token returns 401 at the gateway; room-service never receives the request.
curl -i -X POST http://localhost:8080/api/rooms/luna/follow

# X-Request-Id is forwarded if supplied, otherwise generated.
curl -sD - -X POST http://localhost:8080/api/rooms/luna/follow \
  -H "Authorization: Bearer $TOKEN" -o /dev/null | grep -i X-Request-Id

# CORS preflight
curl -i -X OPTIONS http://localhost:8080/api/auth/login \
  -H 'Origin: http://localhost:5173' \
  -H 'Access-Control-Request-Method: POST' \
  -H 'Access-Control-Request-Headers: Authorization, Content-Type, X-Request-Id'
# → HTTP/1.1 204
# Access-Control-Allow-Origin: http://localhost:5173
# Access-Control-Allow-Methods: GET, POST, DELETE, OPTIONS
# Access-Control-Expose-Headers: Idempotent-Replayed, X-Request-Id
```

## Frontend axios integration

Key expectations in `golive-web/src/lib/axios.ts`:

| Expectation | Gateway behavior |
| -------------------------------- | ----------------------------------------------------- |
| Cross-origin requests with `Authorization` succeed | CORS Allow-Headers includes `Authorization` ✅ |
| 401 triggers `/auth/refresh` | The gateway returns 401 directly for invalid tokens, without calling downstream ✅ |
| A 401 from `/auth/login` or `/auth/refresh` does not trigger refresh, avoiding a loop | Both routes are public and untouched by the gateway; actual downstream authentication failures also pass through as 401 ✅ |
| Application code can read `Idempotent-Replayed` | Included in `Access-Control-Expose-Headers` ✅ |
| 10s timeout | Proxy timeout = 10s, matching axios ✅ |
