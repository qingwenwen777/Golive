# api-gateway

边界网关。所有 `http://localhost:8080/api/*` 请求经此分发到下游 service。

- HTTP: `:8080`
- pprof: `:6060`

## 中间件链

```
Recovery → CORS → RequestID → RateLimit → JWT → ReverseProxy
```

| 中间件     | 作用                                                                                  |
| ---------- | ------------------------------------------------------------------------------------- |
| CORS       | `localhost:5173` + 占位生产域名；`OPTIONS` 预检直接 204；暴露 `Idempotent-Replayed`   |
| RequestID  | 没带 `X-Request-Id` 自动生成 UUID，回写响应头 + 注入到 forward 请求                  |
| RateLimit  | per-IP 令牌桶（`golang.org/x/time/rate`），默认 100 req/s, burst 200                  |
| JWT        | 白名单外强制校验；通过后写 `X-User-Id` 到 forward 请求；剥除 client 伪造的 X-User-Id |
| Proxy      | 去掉 `/api` 前缀；10s 超时；5xx 包装统一错误体；4xx 原样透传                         |

## 路由

| Path                                  | Upstream      | Auth   |
| ------------------------------------- | ------------- | ------ |
| `/api/auth/*`                         | user-service  | 公开   |
| `/api/users/*`                        | user-service  | JWT    |
| `/api/rooms`, `/api/rooms/*`          | room-service  | GET 公开 / 写操作 JWT |
| `/api/srs/*`                          | room-service  | 公开（S2S）|
| `/api/gifts`                          | gift-service  | 公开   |
| `/api/gifts/*`, `/api/super-chats`    | gift-service  | JWT    |

## 错误响应规则

- 下游 `2xx` / `3xx` / `4xx`：**原样透传**，不动 body。这条很关键 ——
  402 `{message:"insufficient coin", reason:"insufficient_coin"}` 必须穿过网关。
- 下游 `5xx` 且 body 已是 `{message,...}` 形状：保留原样。
- 下游 `5xx` 且 body 不合规（HTML、空、纯文本）：包装为
  `{"message":"Internal error","reason":"upstream_unavailable"}`。
- 下游不可达 / 超时：`502` / `504` + 同上 body。

## 配置

`configs/config.yaml`，可 `--config` 覆盖。upstream 地址先写死，
后续接 etcd 服务发现时改这一处即可。

## 启动

```bash
cd deploy && docker compose up -d mysql redis     # 依赖
cd ../app/user-service && go run ./cmd            # :8090
cd ../room-service && go run ./cmd                # :8091
# gift-service 下一轮做；现在缺它不影响 auth/rooms
cd ../api-gateway && go run ./cmd                 # :8080
```

## 测试

```bash
cd app/api-gateway

# 中间件单元
go test ./internal/middleware/...

# router 集成（启 spy upstream，断言路由 + JWT 注入 + RequestId 透传 + 5xx 包装）
go test ./internal/router/...
```

覆盖：

- `RequestID` 缺失 → 生成；存在 → 透传
- `JWT` 拒绝缺/伪 token；接受合法 token；剥除伪造的 `X-User-Id`；公开路由跳过
- `CORS` 预检 204 + 正确响应头；非白名单 origin 不返 ACAO
- 路由：去 `/api`；query string 保留；JWT 通过后注入 `X-User-Id`
- 错误：4xx 透传；5xx 非 JSON 包装；5xx 已合规保留；upstream 不可达 502/504

## 端到端 curl

```bash
# 登录（公开）
curl -i -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"demo"}' | tee /tmp/login.txt
# 注意响应头：Access-Control-Allow-Origin / X-Request-Id

TOKEN=$(jq -r .token < <(grep -A100 '^{' /tmp/login.txt))

# 受保护：跟随频道（JWT 由 gateway 注入 X-User-Id 到 room-service）
curl -i -X POST http://localhost:8080/api/rooms/luna/follow \
  -H "Authorization: Bearer $TOKEN"

# 不带 token 命中受保护路由 → gateway 直接 401，请求不会到达 room-service
curl -i -X POST http://localhost:8080/api/rooms/luna/follow

# X-Request-Id 透传：不带就生成
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

## 与前端 axios 的对接

`golive-web/src/lib/axios.ts` 的关键期望：

| 期望                             | 网关行为                                              |
| -------------------------------- | ----------------------------------------------------- |
| 跨域请求带 `Authorization` 通过  | CORS Allow-Headers 含 `Authorization` ✅              |
| 401 触发 `/auth/refresh`          | 网关在 token 失效时直接返 401（不到下游）✅           |
| `/auth/login` `/auth/refresh` 自身 401 不触发 refresh（避免死循环） | 这两条是公开路由，gateway 不动；下游真正认证失败时返回的 401 也透传 ✅ |
| 业务侧能读 `Idempotent-Replayed` | `Access-Control-Expose-Headers` 含此项 ✅             |
| 10s 超时                         | proxy timeout = 10s，与 axios 一致 ✅                 |
