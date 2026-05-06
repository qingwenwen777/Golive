# im-gateway

实时消息长连接网关。承载所有直播间 WebSocket，单实例目标 ~50k 并发。

- HTTP/WS: `:8081`
- /metrics + /healthz + /debug/rooms 同端口
- pprof: `:6066`

## 是什么 / 为什么

| 关注点         | 设计                                                                                                                                   |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| 长连接管理     | `Hub`（rooms map）→ `Room`（conns map）→ `Conn`（per-conn send chan + 双 goroutine 读写泵）。两层 RWMutex；fanout 路径"快照后释放锁"，永不持锁过网络。 |
| 跨实例分发     | Redis Pub/Sub。`im-gateway` 自身不直接处理弹幕业务；chat-service / gift-service 把消息 publish 到 `room:<id>`，所有 im-gateway 实例订阅并 fanout。 |
| **懒订阅**     | 房间第一个连接进入 → SUBSCRIBE；最后一个离开 → UNSUBSCRIBE。这是关键 ——避免空闲进程持有上亿条 idle 订阅。最后离开后再加锁双检防 race。              |
| 慢消费者       | 每个 `Conn.send` 是 cap=256 的 chan，**满即踢**（非阻塞）。一个慢客户端不能拖慢同房间的其他 5 万人。                                          |
| 客户端→服务端  | 不直接广播。`chat` 写 Kafka `danmu` topic（partition=roomId 保序），由 chat-service 审核+落盘+回 publish。本服务只做 token / 限流校验。       |
| 鉴权           | 握手 query 必须带有效 `token`：缺失/非法 → 401，合法 → 写入 `Identity`。                                                               |
| 心跳/超时      | `ReadIdleTimeout=60s`，任意帧（含 heartbeat、pong）都会续期；`WriteDeadline=10s`；`ReadLimit=4KB` 防滥用。                                  |
| 指标           | Prometheus：连接数、房间数、按 type 收发计数、丢弃原因、broadcast latency 直方图。Top-N 单独 `/debug/rooms`，避免 roomId label 高基数。     |

## 协议（与前端 ws-server.ts 一致）

握手：`ws://host:8081/ws?roomId=<id>&token=<jwt>`

| 方向       | type            | 字段                                                                  |
| ---------- | --------------- | --------------------------------------------------------------------- |
| S→C        | `system`        | `text, ts(ms)`                                                        |
| S→C        | `viewer_count`  | `count`                                                               |
| S→C        | `chat`          | `id, user, avatar?, text, color?, ts`                                 |
| S→C        | `super_chat`    | `id, user, avatar?, amount, tier(0–5), text, ts`                      |
| S→C        | `gift`          | `user, giftName, ts`                                                  |
| C→S        | `heartbeat`     | —                                                                     |
| C→S        | `resume`        | `lastMessageId?` （MVP 仅回 `system: "resumed"`，无重放缓冲）         |
| C→S        | `chat`          | `text` （≤200 字节；rate limit 5 msg/s/conn）                         |

握手即送：一条 `system: "Welcome to the live room!"` + 一条 `viewer_count: 1`，对齐前端 mock 行为。

## 启动

```bash
docker compose -f deploy/docker-compose.yml up -d redis
go run ./app/im-gateway/cmd      # :8081

# 查看指标
curl http://localhost:8081/metrics | head
curl http://localhost:8081/debug/rooms
```

## 测试

```bash
go test ./app/im-gateway/...
```

覆盖：

- **hub**：lazy subscribe 仅首次 / 最后离开拆订阅 / 同房 fanout / 房间隔离 /
  慢消费者被 evict / viewer_count 周期推送 / Snapshot Top-N 排序
- **dispatch**：heartbeat 静默 / resume ack / 未认证 chat 拒绝 / 已认证 chat 进 producer /
  rate-limit 上限 / 超长 chat 丢弃 / 未知 type 丢弃
- **auth**：空 token 拒绝 / 合法 token 鉴权 / 错签名拒绝 / 错 secret 拒绝

## 压测

### k6（推荐）

```bash
k6 run \
  -e WS_URL=ws://localhost:8081/ws \
  -e ROOMS=20 \
  -e PER_VU=1000 \
  -u 50 -d 5m \
  app/im-gateway/loadtest/connect.js
```

50 VUs × 1000 conns = 50k 并发，5 分钟。阈值 handshake p95 < 200ms。

### Go 原生（k6 不可用时）

```bash
go run -tags loadtest ./app/im-gateway/loadtest \
  -url ws://localhost:8081/ws \
  -conns 50000 -rooms 20 -hold 60s -dial 512
```

输出 handshake p50/p95/p99、成功/失败计数、消息接收总量。

### 系统调优（5 万连接前必做）

Linux：
```bash
sysctl -w fs.file-max=2000000
sysctl -w net.ipv4.ip_local_port_range="10000 65535"
sysctl -w net.core.somaxconn=65535
ulimit -n 1048576
```

服务侧：`GOMAXPROCS=$(nproc)`，开 pprof 抓 goroutine + 堆图验证。

## 与前端对接

把 `golive-web/.env.development.local` 设置：

```env
VITE_ENABLE_MSW=false
VITE_WS_BASE=ws://localhost:8081/ws
```

并在 `src/main.tsx` 跳过 `startMockWsServer()`。LiveRoomPage 直接使用真实 WS。

## 与 gift-service / chat-service 的协作

下一步两条线：

1. **chat-service**：消费 Kafka `danmu`，过敏感词，写 MySQL `danmaku` 表，
   再 `PUBLISH room:<id>` 一条 `chat` 类型消息。im-gateway 订阅到后 fanout。
2. **gift-service**：扣款成功后 `PUBLISH room:<id>` 一条 `gift` 或
   `super_chat` 消息。im-gateway 同样 fanout。

整个调用链：
```
client → ws → im-gateway →(produce) kafka → chat-service →(publish) redis
                                                              ↓
                                  im-gateway[N实例] (subscribe & fanout)
                                                              ↓
                                                          所有 ws 客户端
```
