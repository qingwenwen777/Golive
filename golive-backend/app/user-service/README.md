# user-service

Handles authentication (login / refresh / logout) and user profiles (GET /users/me).
**Its own routes omit the `/api` prefix**; api-gateway exposes them under that prefix through its reverse proxy.

## Ports

- HTTP: `:8090`
- pprof: `:6062`

## Configuration

Use `configs/config.yaml`; override its path with `--config`.
It contains the JWT secret, demo-user toggle, and other settings.

## Startup

Dependencies: MySQL / Redis must be running (`deploy/docker-compose.yml`).

```bash
# 1. Start dependencies.
cd deploy && docker compose up -d mysql redis

# 2. Start user-service (listening on :8090).
cd ../app/user-service
go run ./cmd

# 3. Start api-gateway (listening on :8080; proxies /api/auth/* and /api/users/me to user-service).
cd ../../
go run ./cmd/api-gateway
```

At startup, `AutoMigrate` creates or updates the `users` table. Depending on `bootstrap.demo_user`, it inserts
a demo account with `username=demo / password=demo / coinBalance=100000`
(skipping insertion if the account already exists).

## Run tests

```bash
cd app/user-service
go test ./internal/service/...
```

Coverage: successful login / wrong password / missing user / refresh rotation and old-token invalidation /
invalid refresh / logout revocation / JWT round-trip.

## Integration (disable frontend MSW)

Create `.env.development.local` under `golive-web/`:

```env
VITE_ENABLE_MSW=false
VITE_API_BASE=http://localhost:8080/api
VITE_WS_BASE=ws://localhost:8081/ws
VITE_FLV_BASE=http://localhost:8082
```

Wrap MSW startup in `src/main.tsx` in a condition if it is not already guarded:

```ts
if (import.meta.env.VITE_ENABLE_MSW !== 'false') {
  await worker.start()
}
```

Then run `npm run dev` and sign in with `demo / demo` to call this backend directly.

## End-to-end curl examples

```bash
# 1) login
curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"demo"}' | tee /tmp/login.json
# → { "token": "...", "refreshToken": "...", "user": { ... } }

TOKEN=$(jq -r .token /tmp/login.json)
RTOKEN=$(jq -r .refreshToken /tmp/login.json)

# 2) /users/me
curl -s http://localhost:8080/api/users/me -H "Authorization: Bearer $TOKEN"
# → { "id":"...", "username":"demo", "avatar":"...", "coinBalance":100000, "verified":true }

# 3) Refresh: the old refreshToken must become invalid.
curl -s -X POST http://localhost:8080/api/auth/refresh \
  -H 'Content-Type: application/json' \
  -d "{\"refreshToken\":\"$RTOKEN\"}" | tee /tmp/refresh.json
# → { "token":"NEW", "refreshToken":"NEW" }

# 4) Refresh again with the old refreshToken; expect 401.
curl -i -X POST http://localhost:8080/api/auth/refresh \
  -H 'Content-Type: application/json' \
  -d "{\"refreshToken\":\"$RTOKEN\"}"
# → HTTP/1.1 401  { "message":"Invalid refresh token" }

# 5) Wrong password.
curl -i -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"wrong"}'
# → HTTP/1.1 401  { "message":"Invalid username or password" }

# 6) logout
curl -s -X POST http://localhost:8080/api/auth/logout \
  -H 'Content-Type: application/json' \
  -d "{\"refreshToken\":\"$RTOKEN\"}"
# → { "ok": true }
```

## Verify against frontend MSW

| Item | MSW behavior | Service behavior |
| ---------------- | ------------------------------------- | --------------------------------------------- |
| `demo/demo` login | 200 + token/refreshToken/user | ✅ Demo user inserted by bootstrap |
| Invalid credentials | 401 `{ message: "Invalid username..." }` | ✅ `service.ErrInvalidCredentials` |
| /users/me 401    | `{ message: "Unauthorized" }`         | ✅ `service.ErrUnauthorized`                  |
| Refresh rotation | Old refreshToken becomes invalid immediately | ✅ Redis pipeline DEL + SET |
| Invalid refresh | 401 `{ message: "Invalid refresh token" }` | ✅ |
| User fields | id/username/avatar/coinBalance/verified | ✅ `model.PublicUser` |
| LoginResp fields | token/refreshToken/user | ✅ `service.LoginResp` |
| Logout response | `{ ok: true }` | ✅ |
