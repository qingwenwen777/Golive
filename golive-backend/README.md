# GoLive Backend

GoLive 后端是一个 Go 微服务工程，给 `../golive-web` 提供 HTTP API、
WebSocket、直播流回调、礼物订单、弹幕处理、上传和管理后台数据。

## 技术栈

- Go 1.22+、Gin、gorilla/websocket、gRPC + Protobuf
- MySQL 8 + GORM、Redis 7、Kafka、etcd、MinIO
- Viper、zap、OpenTelemetry、Jaeger、Prometheus、Grafana
- SRS 5（RTMP → HTTP-FLV/HLS）、Docker Compose、nginx

## 服务划分

| 服务 | 入口 | 说明 |
| ---- | ---- | ---- |
| api-gateway | `app/api-gateway/cmd` | 对外 HTTP 网关，鉴权、CSRF、限流、反向代理 |
| user-service | `app/user-service/cmd` | 登录、Google 登录、用户资料、头像/封面、金币、创作者申请 |
| room-service | `app/room-service/cmd` | 房间、开播、预约、频道动态、关注点赞、回放、审核 |
| chat-service | `app/chat-service/cmd` | 弹幕限流、敏感词、落盘、历史弹幕 |
| gift-service | `app/gift-service/cmd` | 礼物、SuperChat、竞猜、幂等订单、本地消息表 |
| im-gateway | `app/im-gateway/cmd` | WebSocket 长连接、房间 fanout、Redis Pub/Sub |

## 端口约定

| 组件 | 端口 | 说明 |
| ---- | ---- | ---- |
| nginx | 80 / 443 | 静态文件、`/api`、`/ws`、`/live` 入口 |
| api-gateway | 8080 | HTTP JSON，外部路径前缀 `/api` |
| im-gateway | 8081 | WebSocket，路径 `/ws` |
| SRS RTMP | 1935 | OBS 推流 |
| SRS HTTP | 容器内 8080 | nginx 反代为 `/live` |
| MySQL / Redis / Kafka / etcd / MinIO | compose 内网 | 不应对公网开放 |
| Grafana / Prometheus / Jaeger | 127.0.0.1 绑定 | 通过 SSH tunnel 访问 |

## 目录结构

```text
golive-backend/
  api/
    proto/              Protobuf 定义
    gen/go/             生成后的 Go 代码
  app/
    <service>/
      cmd/              服务入口
      configs/          本地示例配置
      internal/         handler/server/service/repo/model/config
      README.md         单服务说明
  deploy/
    docker-compose.yml  本地和服务器运行栈
    configs/            compose 使用的服务配置
    nginx*.conf         HTTP/HTTPS 入口
    srs.conf            SRS 配置
    observability/      Prometheus、OTel、Grafana
  pkg/                  公共包：JWT、幂等、错误码、日志、上传、内容策略等
```

## 本地启动

推荐从仓库根目录使用一键脚本：

```bash
bash scripts/dev.sh
# 或 Windows:
powershell -File scripts/dev.ps1
```

只启动后端栈：

```bash
cd golive-backend/deploy
docker compose up -d
```

Compose 中的 Go 服务使用 `go run ./app/<service>/cmd -config ./deploy/configs/<service>.yaml`，
源码以只读卷挂载，`docker compose restart <service>` 会重新编译并加载最新代码。

## 测试

```bash
go test ./...
```

也可以按服务缩小范围：

```bash
go test ./app/api-gateway/...
go test ./app/room-service/...
go test ./app/user-service/...
go test ./app/chat-service/...
go test ./app/gift-service/...
go test ./app/im-gateway/...
```

## 前端契约

- 前端默认同源访问：`VITE_API_BASE=/api`、`VITE_WS_BASE=/ws`、
  `VITE_FLV_BASE=/live`。
- JWT 使用 `Authorization: Bearer <token>`，不依赖 cookie。
- 写操作需要 CSRF token；前端会从 `/api/csrf-token` 获取并自动重试一次。
- `/api/gifts/send`、`/api/super-chats` 按 `X-Request-Id` 幂等，replay 响应头为
  `Idempotent-Replayed: true`。
- 余额不足返回 HTTP 402，`reason` 为 `insufficient_coin`。
- 对观众返回的直播流信息不能暴露 `streamKey`。
- WebSocket 握手：`/ws?roomId=<id>&token=<jwt>`。

更多联调细节见 `../docs/integration.md`，部署流程见 `../docs/deploy-git-bare.md`。
