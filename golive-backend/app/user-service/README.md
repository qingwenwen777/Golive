# user-service

负责认证（login / refresh / logout）和用户资料（GET /users/me）。
**自身路由不带 `/api` 前缀**，由 api-gateway 反代时附加。

## 端口

- HTTP: `:8090`
- pprof: `:6062`

## 配置

`configs/config.yaml`，可用 `--config` 覆盖路径。
JWT secret、demo 用户开关等全在里面。

## 启动

依赖：MySQL / Redis 已起（`deploy/docker-compose.yml`）。

```bash
# 1. 启动依赖
cd deploy && docker compose up -d mysql redis

# 2. 启动 user-service（监听 :8090）
cd ../app/user-service
go run ./cmd

# 3. 启动 api-gateway（监听 :8080，反代 /api/auth/*、/api/users/me 到 user-service）
cd ../../
go run ./cmd/api-gateway
```

启动时自动 `AutoMigrate users` 表，并按 `bootstrap.demo_user` 配置插入
`username=demo / password=demo / coinBalance=100000` 的 demo 账号
（已存在则跳过）。

## 跑测试

```bash
cd app/user-service
go test ./internal/service/...
```

覆盖：登录成功 / 密码错误 / 用户不存在 / refresh 轮换 + 旧 token 失效 /
refresh 无效 / logout 撤销 / JWT round-trip。

## 联调（前端关掉 MSW）

在 `golive-web/` 创建 `.env.development.local`：

```env
VITE_ENABLE_MSW=false
VITE_API_BASE=http://localhost:8080/api
VITE_WS_BASE=ws://localhost:8081/ws
VITE_FLV_BASE=http://localhost:8082
```

并在 `src/main.tsx` 把 MSW 启动包一层判断（如果还没做）：

```ts
if (import.meta.env.VITE_ENABLE_MSW !== 'false') {
  await worker.start()
}
```

然后 `npm run dev`，登录页用 `demo / demo` 直接打到这个后端。

## 端到端 curl

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

# 3) refresh —— 旧 refreshToken 必须作废
curl -s -X POST http://localhost:8080/api/auth/refresh \
  -H 'Content-Type: application/json' \
  -d "{\"refreshToken\":\"$RTOKEN\"}" | tee /tmp/refresh.json
# → { "token":"NEW", "refreshToken":"NEW" }

# 4) 用旧 refreshToken 再 refresh，应该 401
curl -i -X POST http://localhost:8080/api/auth/refresh \
  -H 'Content-Type: application/json' \
  -d "{\"refreshToken\":\"$RTOKEN\"}"
# → HTTP/1.1 401  { "message":"Invalid refresh token" }

# 5) 错误密码
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

## 对照前端 MSW 自查

| 项目             | MSW 行为                              | 本服务行为                                    |
| ---------------- | ------------------------------------- | --------------------------------------------- |
| `demo/demo` 登录 | 200 + token/refreshToken/user         | ✅ bootstrap 插入 demo 用户                   |
| 错误凭据         | 401 `{ message: "Invalid username..." }` | ✅ `service.ErrInvalidCredentials`           |
| /users/me 401    | `{ message: "Unauthorized" }`         | ✅ `service.ErrUnauthorized`                  |
| refresh 轮换     | 旧 refreshToken 立即失效              | ✅ Redis pipeline DEL + SET                   |
| refresh 无效     | 401 `{ message: "Invalid refresh token" }` | ✅                                       |
| User 字段        | id/username/avatar/coinBalance/verified | ✅ `model.PublicUser`                       |
| LoginResp 字段   | token/refreshToken/user               | ✅ `service.LoginResp`                        |
| logout 响应      | `{ ok: true }`                        | ✅                                            |
