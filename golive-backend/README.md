# GoLive Backend

GoLive 直播平台的 Go 后端。与 `../golive-web` 前端严格按契约对接，
目标是前端关闭 MSW 后直连本后端，功能与 mock 行为完全一致。

## 技术栈

- Go 1.22+、Gin（HTTP 网关）、gRPC + Protobuf（服务间）、gorilla/websocket
- MySQL 8 + GORM、Redis 7、Kafka、etcd（服务发现）
- Viper 配置、zap 日志、OpenTelemetry + Jaeger
- SRS 5.0（RTMP → HLS/FLV）、Docker Compose 编排

## 端口约定（硬性，来自前端 .env）

| 组件          | 端口   | 说明                                |
| ------------- | ------ | ----------------------------------- |
| api-gateway   | 8080   | HTTP JSON，前缀 `/api`              |
| im-gateway    | 8081   | WebSocket，路径 `/ws`               |
| SRS HTTP-FLV  | 8082   | 拉流（`VITE_FLV_BASE`）             |
| SRS RTMP      | 1935   | 推流                                |
| SRS HTTP API  | 1985   | SRS 管理接口                        |
| MySQL         | 3306   |                                     |
| Redis         | 6379   |                                     |
| Kafka         | 9092   |                                     |
| etcd          | 2379   |                                     |
| MinIO         | 9000/9001 | 对象存储（后续启用）             |

## 服务划分

```
cmd/
  api-gateway/      # Gin，对外 HTTP，鉴权/幂等/聚合 gRPC
  im-gateway/       # gorilla/websocket，房间消息推送
  user-service/     # 登录、鉴权、用户资料、金币
  room-service/     # 房间、分类、关注、点赞
  chat-service/     # 弹幕、敏感词、限流
  gift-service/     # 礼物、SuperChat、订单、幂等
```

## 目录结构

```
golive-backend/
├── cmd/<service>/main.go
├── internal/<service>/{config,server,service,repo,model}/
├── pkg/
│   ├── logger/         # zap 封装 + FromCtx
│   ├── discovery/      # Registry 接口 + etcd 实现
│   ├── errcode/        # 统一 {message, reason?} 错误
│   └── idempotency/    # X-Request-Id + Redis 幂等中间件
├── api/proto/          # .proto（后续补）
├── deploy/
│   ├── docker-compose.yml
│   └── srs.conf
└── go.mod
```

## 本地开发

```bash
# 启动依赖（MySQL/Redis/Kafka/etcd/SRS/MinIO）
cd deploy && docker compose up -d

# 启动 api-gateway（示例）
go run ./cmd/api-gateway
```

## 前端联调（关掉 MSW）

前端 `golive-web` 默认开启 MSW。联调本后端时二选一：

1. **环境变量（推荐）**：在 `golive-web/.env.development.local` 里写
   ```
   VITE_ENABLE_MSW=false
   VITE_API_BASE=http://localhost:8080/api
   VITE_WS_BASE=ws://localhost:8081/ws
   VITE_FLV_BASE=http://localhost:8082
   ```
   并在 `main.tsx` 中根据 `import.meta.env.VITE_ENABLE_MSW` 判断是否调用
   `worker.start()`。

2. **直接删 main.tsx 里 `worker.start()` 调用**（临时做法）。

然后 `npm run dev`，前端请求会直接打到 `http://localhost:8080/api`。

## 契约要点（必读）

- JSON 字段 camelCase；时间戳 HTTP 用 ISO 8601，WS 用毫秒 Unix。
- JWT Bearer + refreshToken 轮换；401 触发前端 `/api/auth/refresh`。
- `/api/gifts/send` 和 `/api/super-chats` 按 `X-Request-Id` 幂等（10 分钟），
  replay 命中响应头带 `Idempotent-Replayed: true`。
- 余额不足：HTTP 402，body `{ message, reason: "insufficient_coin" }` +
  一个 `status=failed, failReason="insufficient_coin"` 的 order 对象。
- 返回给观众的 `Stream` 必须剥除 `streamKey`。
- WS 握手 `ws://localhost:8081/ws?roomId=<id>&token=<jwt>`，无 token 会被拒绝。

详细端点清单见 `docs/` 与前端 `src/mocks/handlers/*.ts`。
