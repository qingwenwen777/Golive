# 前后端联调备忘

当前联调模型：`golive-web` 默认直接访问真实 Go 后端，Vite 开发环境和
生产 nginx 都使用 `/api`、`/ws`、`/live` 三个同源入口。

## 拓扑

```text
浏览器
  ├─ /              -> golive-web/dist
  ├─ /api/*         -> api-gateway:8080
  ├─ /ws            -> im-gateway:8081
  └─ /live/*        -> srs:8080/live/*

docker compose:
  nginx、api-gateway、user-service、room-service、chat-service、
  gift-service、im-gateway、SRS、MySQL、Redis、Kafka、etcd、MinIO、
  Prometheus、Grafana、Jaeger、OTel Collector
```

## 一键启动

从仓库根目录执行：

```bash
# Windows PowerShell
powershell -File scripts/dev.ps1

# Linux/macOS/WSL/Git Bash
bash scripts/dev.sh
```

脚本会构建前端、启动 Compose、轮询健康检查并打印本地访问地址。

## 前端环境变量

| 变量 | 开发和生产默认值 | 说明 |
| ---- | ---------------- | ---- |
| `VITE_API_BASE` | `/api` | axios baseURL |
| `VITE_WS_BASE` | `/ws` | 直播间 WebSocket |
| `VITE_FLV_BASE` | `/live` | HTTP-FLV 拉流 |
| `VITE_RTMP_BASE` | `rtmp://localhost/live` 或生产域名 | OBS 推流提示 |

Vite `server.proxy` 会把这些相对路径代理到本地后端端口。生产环境由 nginx
反代同样的路径，所以代码里不需要维护两套 base URL。

## 容易踩到的点

### WebSocket 相对路径

浏览器不能直接 `new WebSocket('/ws')`。前端 `useRoomRealtime.ts` 会在
`VITE_WS_BASE` 以 `/` 开头时自动补成当前页面协议对应的 `ws://host/ws` 或
`wss://host/ws`。

### HTTP-FLV 路径

前端最终访问 `/live/<streamKey>.flv`。nginx 的 `location /live/` 会把请求
转发到 SRS 容器的 `http://srs:8080/live/`。两个路径末尾的斜杠都要保留，
否则容易变成 `/live/live/...`。

### Token 和 CSRF

`useAuthStore` 把 token 和 refreshToken 持久化到 `localStorage`。axios 拦截器
会追加 `Authorization: Bearer <token>`，401 时自动调用 `/api/auth/refresh`。

写操作会先取 `/api/csrf-token`，并带上 CSRF header；如果收到
`reason=csrf_invalid`，前端会清缓存后自动重试一次。

### 幂等请求

送礼和 SuperChat 需要稳定的 `X-Request-Id`。命中 replay 时后端返回
`Idempotent-Replayed: true`，前端可用它区分真实执行和幂等重放。

## 用户旅程 checklist

| 步骤 | 操作 | 成功标志 | 排查入口 |
| ---- | ---- | -------- | -------- |
| 1 登录 | `/login` 或登录弹窗 | 200，token 写入 localStorage | `api-gateway`、`user-service` 日志 |
| 2 首页列表 | `/` | 直播卡片渲染 | `room-service` 日志、MySQL/Redis 状态 |
| 3 进入直播间 | `/live/:id` | 房间详情加载，WS readyState=1 | `im-gateway` 日志、token、roomId |
| 4 拉流 | 播放器加载 | FLV 请求 200 | SRS 是否有流，nginx `/live/` 配置 |
| 5 发弹幕 | 输入框发送 | 自己消息回显，房间 fanout | im-gateway -> Kafka -> chat-service -> Redis |
| 6 送礼 | 礼物面板 | 订单成功，动画出现 | `gift-service` 日志、金币余额、requestId |
| 7 关注/点赞 | 房间操作 | UI 乐观更新后与接口一致 | `room-service` 社交接口 |
| 8 创作者开播 | `/studio/prepare` | 返回 streamKey 和 RTMP 地址 | room-service、SRS on_publish 回调 |
| 9 管理后台 | `/admin/dashboard` | 指标和审核列表加载 | api-gateway、room/user/gift service |
| 10 断网重连 | DevTools Offline 后恢复 | WS 自动重连，消息恢复 | 浏览器 console、im-gateway close/reconnect |

## 常用排查命令

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

健康检查：

```bash
curl http://localhost:8080/healthz
curl http://localhost:8081/healthz
curl http://localhost:8082/api/v1/versions
```
