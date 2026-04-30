# chat-service

弹幕审核 / 限流 / 落盘 / 广播。无 HTTP 写接口；唯一对外端点是历史回放。

- HTTP: `:8093`（仅 `/rooms/:id/danmus` + `/healthz`，由 api-gateway 反代）
- pprof: `:6068`

## 数据流

```
client → ws → im-gateway →(produce)→ kafka:danmu
                                       │
                                       ▼ consumer-group "chat-consumer"
                              ┌─────────────────────┐
                              │  chat-service       │
                              │  ┌──────────────┐   │
                              │  │ rate-limit   │   │   Redis bucket
                              │  ├──────────────┤   │
                              │  │ DFA filter   │   │   in-mem trie
                              │  ├──────────────┤   │
                              │  │ persist      │───┼──→ MySQL danmus_{0..7}
                              │  ├──────────────┤   │
                              │  │ publish      │───┼──→ Redis room:<id>
                              │  └──────────────┘   │
                              └─────────────────────┘
                                       │
                                       ▼
                                  im-gateway[*] (subscribe & fanout)
                                       │
                                       ▼
                                   ws clients
```

## 4 阶段流水线

| 阶段       | 实现                                                                                          |
| ---------- | --------------------------------------------------------------------------------------------- |
| 限流       | Redis 固定窗口 + Lua（`INCR` + `PEXPIRE`），每用户 3 msg/sec（可配）。超限直接丢弃。          |
| 敏感词过滤 | 自研 DFA（rune-level trie）。最长匹配优先；ASCII 大小写不敏感；可选跳过字符 `". *-_"`。       |
| 持久化     | MySQL **8 个分表** `danmus_0..7`，按 `fnv32(roomId) % 8`。同一房间永远落同一张表。            |
| 广播       | Redis `PUBLISH room:<roomId>`，im-gateway 订阅后 fan-out 到 WS。                              |

## 内部并发

Kafka 消费者按 N 个 worker goroutine 并行处理，**dispatcher 把每条 record 按 `fnv32(roomId) % N` 路由到 worker**。这样：

- 跨房间天然并行（CPU 利用率 ≈ N）
- 同房间永远同一 worker，**保留时序**（用户 A 发完才轮到 B）
- worker 数与 MySQL 分片数同源 hash，连接池热复用

提交 offset 用 `DisableAutoCommit + CommitUncommittedOffsets`：dispatch 完成后才提交。worker 实际处理出错走重试或落"死信"将来再做；当前 MVP 直接 log + drop（不会卡住消费推进）。**至少一次** delivery 语义，danmu PK 是 UUID，重复消息天然去重。

## DFA 实现要点

- rune trie（`map[rune]*node`），CJK 一等公民
- `Replace(text)` 单次扫描；每个起点走最长匹配后跳到匹配末尾，结果不重叠
- ASCII 字母 fold 成小写；非 ASCII 走 `unicode.ToLower`
- `WithSkipChars(" .*-")` 让 `s.h.i.t` 也命中（仅在已进入匹配状态时才跳，避免空命中）

测试覆盖：基础命中、大小写、最长前缀（`sh` vs `shit`）、不重叠（`abcab → ***ab`）、空词典、跳字符、CJK 部分前缀不误匹、大字典 smoke。

## 分表策略

```
shard(roomId) = fnv32(roomId) % shards
table         = "danmus_" + str(shard)
```

| 选项               | 说明                                            |
| ------------------ | ----------------------------------------------- |
| 分片数             | `mysql.shards`，默认 8                          |
| Hash               | FNV-1a 32bit（廉价、分布均匀）                  |
| 主键               | UUID v4（写入无热点 + 重试天然去重）            |
| 索引               | `room_id`, `ts` 联合查询历史；`user_id` 做 ban 用 |

查询一个房间最近 50 条：

```sql
-- 通过 GORM 自动选表；手写 SQL 也得带上分片号：
SELECT * FROM danmus_3
WHERE room_id = 'luna-music'
ORDER BY ts DESC
LIMIT 50;
```

跨房间统计需要在所有分表上 UNION ALL；线上很少有跨房聚合需求，把它推到离线 OLAP 层（接 ClickHouse / Hive）。

## 启动

```bash
docker compose -f deploy/docker-compose.yml up -d mysql redis kafka zookeeper
go run ./app/chat-service/cmd
```

启动时会：

1. AutoMigrate 8 张分表
2. 加载 `configs/sensitive.txt` 构建 DFA
3. 加入 Kafka consumer group `chat-consumer` 订阅 `danmu` topic

## 测试

```bash
go test ./app/chat-service/...
```

覆盖：

- DFA：9 个用例（命中 / 大小写 / 最长 / 不重叠 / 空 / 跳字符 / CJK / 性能 smoke）
- 限流：4 个用例（允许 / 拒绝 / per-user 隔离 / 跨窗口重置，使用 miniredis FastForward）

## 压测

```bash
go run -tags loadtest ./app/chat-service/loadtest \
  -brokers 127.0.0.1:9092 \
  -redis 127.0.0.1:6379 \
  -qps 5000 -rooms 50 -duration 60s
```

输出端到端延迟 p50/p95/p99（生产到 Redis 收到的时间），观察目标：

- 5000 QPS 持续 60s 不丢
- e2e p99 < 50ms（本机）/ < 200ms（容器化网络）
- MySQL 写入 `INSERT` 不应是瓶颈：8 张表分散热点

## HTTP

```bash
curl 'http://localhost:8093/rooms/luna-music/danmus?limit=20'
# { "items": [ {type:"chat", id, user, text, ts}, ... ] }
```

通过 api-gateway 同样可以访问 `/api/rooms/:id/danmus`（只需在 gateway router 加一行反代到 chat-service）。

## 与 im-gateway 的契约

- Kafka 输入：`{roomId, userId, username?, avatar?, text, ts}` JSON，`Key=roomId`
- Redis 输出：`PUBLISH room:<roomId>` 消息体 = `hub.ChatMsg`：
  ```json
  {"type":"chat","id":"uuid","user":"...","avatar":"...","text":"...","color":"...","ts":1700000000000}
  ```
  字段名与 im-gateway `internal/hub/messages.go ChatMsg` 一致。
