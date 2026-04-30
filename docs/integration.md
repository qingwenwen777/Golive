# 前后端联调备忘 (Golive)

本轮：让 `golive-web` 脱离 MSW，直连 `golive-backend` 的 Go 服务。

## 拓扑

```
                    ┌────────────────── docker compose (deploy/) ──────────────────┐
浏览器  ─►  :80 nginx ┐                                                              │
                     ├─► /            → /usr/share/nginx/html (golive-web/dist)     │
                     ├─► /api/        → host.docker.internal:8080  (api-gateway)    │
                     ├─► /ws          → host.docker.internal:8081  (im-gateway)     │
                     └─► /live/       → srs:8080/live/             (HTTP-FLV/HLS)   │
                                                                                    │
       MySQL / Redis / Kafka / etcd / SRS / MinIO / Jaeger / Prom / Grafana 都在容器里 │
       └────────────────────────────────────────────────────────────────────────────┘

Go 服务（api-gateway / im-gateway / room / chat / gift / user）跑在 **宿主机**，
nginx 通过 `host.docker.internal` 反向代理。
```

## 一键启动

```bash
# Linux/macOS / WSL / Git-Bash
bash scripts/dev.sh

# Windows PowerShell
powershell -File scripts/dev.ps1
```

脚本会：build SPA → `docker compose up -d` → 轮询健康检查 → 打印链接。

> Go 服务不在 compose 里。脚本检测到 8080/8081 不通会提示你手工 `go run`。

## 联调中真实踩到 / 容易踩到的坑

### 1. MSW 默认拦截走真实请求

老 `main.tsx` 在 `import.meta.env.DEV` 下无脑 `worker.start()`，dev 跑 vite 时
所有 `/api` 请求都被 service worker 接管，看起来「联调成功」，其实数据都是 mock。

**解法**：用 `VITE_ENABLE_MSW` 显式开关，默认 `false`。
`.env.development` 已置 `VITE_ENABLE_MSW=false`；想回到纯前端 mock 模式
就临时改成 `true`。

判断是否走真后端的最快办法：
- DevTools → Network 面板，每行右侧没有齿轮图标（MSW 标记）；
- Console 没有 `[MSW] Mocking enabled.` 提示。

### 2. `VITE_API_BASE` 在 dev / prod 是两套语义

| 模式            | 值                                 | 原因                                                       |
| --------------- | ---------------------------------- | ---------------------------------------------------------- |
| `pnpm dev`      | `http://localhost:8080/api`        | 浏览器 → 8080，跨域；网关 CORS 已放行 `http://localhost:5173` |
| `pnpm build`+nginx | `/api`                          | 同源走 nginx 反代，无 CORS 问题                            |

如果 dev 模式遇到 CORS 报错，先确认 api-gateway `configs/config.yaml` 里
`cors.allowed_origins` 包含当前 vite 的端口（默认 5173）。

### 3. WebSocket URL 不能直接用相对路径

`VITE_WS_BASE=/ws` 这种相对路径在 `new WebSocket()` 里非法。
`useRoomRealtime.ts` 已加判断：以 `/` 开头时自动拼 `ws://${location.host}`。
production 走 nginx 同源升级，dev 直连 `ws://localhost:8081/ws`。

### 4. SRS 的 FLV 路径

SRS 容器内部 HTTP 端口是 `8080`（`/live/<streamKey>.flv`），
宿主机映射到 `8082`（避免与 api-gateway 冲突）。

| 模式 | `VITE_FLV_BASE`         | 最终 URL                                  |
| ---- | ----------------------- | ----------------------------------------- |
| dev  | `http://localhost:8082` | `http://localhost:8082/live/<key>.flv`    |
| prod | （空）                  | `/live/<key>.flv` → nginx 转 `srs:8080`   |

注意 nginx 的 `location /live/ { proxy_pass http://srs_http/live/; }` 末尾的
两个斜杠都不能少，否则会出现 `/live/live/...` 双前缀。

### 5. Token 存储

`useAuthStore` 用 zustand persist 写到 `localStorage`，axios 拦截器在
请求头加 `Authorization: Bearer <token>`，**不依赖 cookie**。
所以网关不需要 `Access-Control-Allow-Credentials`，前端也不用 `withCredentials`。

401 → `/auth/refresh`（单飞 refresh，登录/refresh/logout 本身不触发递归）。

### 6. 字段名分歧（已知未踩或半踩）

联调真正跑起来后最容易出问题的地方，先在这里登记，后续每发现一处改一处：

- 登录返回：前端期望 `{ token, refreshToken }`；user-service 若返回 `access_token`/`refresh_token` 需要在网关或 BFF 层对齐。
- 房间列表：前端期望 `{ list: Room[], total }`；后端通常 `{ items, total }`。
- 弹幕协议：前端 `useRoomRealtime` 解析 `ServerMessage`（`type` 字段），与 im-gateway 的 protobuf/json 包须一致。

> 对齐策略：不要改后端协议迁就 mock — 改 `mocks/handlers/*` 和前端 type 去贴齐真接口。
> mock 是给联调断网时兜底用的，不是真理。

### 7. 断网重连

`useWebSocket` hook 已开启 `reconnect: true`, `maxRetries: 10`,
心跳 20s。手动验证：DevTools → Network → 右上角 throttling → Offline
保持 3-5 秒 → 切回 Online，应观察到一次 `close` + 自动重连，房间消息恢复。

## 用户旅程联调 checklist

> 因为是无人值守跑下来的，每一步给「成功标志」+「失败时排查路径」。

| 步骤 | 操作 | HTTP/WS | 成功标志 | 失败时看哪里 |
| ---- | ---- | ------- | -------- | ------------- |
| 1 注册 | 首页右上「注册」 | `POST /api/auth/register` | 201 + 自动登录 | api-gateway log → user-service log；MySQL `users` 表 |
| 2 登录 | demo / demo | `POST /api/auth/login` | 200 + `{token, refreshToken}` 写入 localStorage | 网关 401 → user-service；网关 5xx → 网关日志（CORS / panic） |
| 3 首页列表 | 路由 `/` | `GET /api/rooms` | 列表渲染，无骨架屏卡死 | room-service log；redis 是否在线 |
| 4 进入直播间 | 点卡片 | `GET /api/rooms/:id` + WS upgrade `/ws?roomId=...` | 视频流加载（FLV 200），WS readyState=1 | SRS 推流是否在 (`http://localhost:8082/api/v1/streams`)；im-gateway log；token 是否带上 |
| 5 看到弹幕 | — | WS frame `type:"chat"` | 列表滚动 | im-gateway → kafka → chat-service 消费链路；Jaeger trace |
| 6 发弹幕 | 输入框回车 | WS send `type:"chat"` 或 `POST /api/chat` | 自己消息回显 | im-gateway log；登陆态是否有效 |
| 7 送礼物 | 礼物面板 | `POST /api/gifts/send` | 200 + 弹特效 | gift-service log；402 `insufficient_coin` 是预期错误，看用户金币 |
| 8 关注主播 | 头像旁「关注」 | `POST /api/rooms/:id/follow` | 200 / 幂等 200 (`Idempotent-Replayed: true`) | room-service follow 表 |
| 9 点赞 | 心形按钮 | `POST /api/rooms/:id/like`（节流） | 累加；失败回滚 | room-service log；rate-limit 命中？看 429 |
| 10 断网重连 | DevTools Offline 5s | WS 自动重连 | console 看到 reconnect 计数 → readyState=1 | im-gateway log，确认旧连接 close；浏览器 token 没过期 |

## 启动顺序速记

1. `cd golive-backend/deploy && docker compose up -d`（基础设施 + nginx）
2. 宿主机 `go run` 起 6 个 service（user / room / chat / gift / api-gw / im-gw）
3. `cd golive-web && pnpm build`（dist 被 nginx 卷挂载）
4. 浏览器 `http://localhost`
5. dev 模式可用 `pnpm dev` + 浏览器 `http://localhost:5173`，CORS 走 8080
