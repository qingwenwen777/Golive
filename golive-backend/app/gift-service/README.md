# gift-service

礼物目录、打赏、SuperChat、本地消息表（事务性 outbox）。

- HTTP: `:8092`（裸路径，由 api-gateway 加 `/api` 前缀）
- pprof: `:6067`

## 端点

| 方法 | 路径             | 鉴权 | 说明                                                                |
| ---- | ---------------- | ---- | ------------------------------------------------------------------- |
| GET  | `/gifts`         | 否   | 礼物目录                                                            |
| POST | `/gifts/send`    | JWT  | 送礼，幂等（X-Request-Id），余额不足 402                            |
| POST | `/super-chats`   | JWT  | SuperChat，amount→tier 映射；tier 0 拒绝；其余规则同 gifts/send     |

## 响应形状（与 src/mocks/handlers/gift.ts 一一对照）

```
HTTP 200 + body=GiftOrder                            # 成功 / 成功 replay
                                                     #   (replay 同时设 Idempotent-Replayed: true)
HTTP 402 + body={...GiftOrder, reason, message}      # 余额不足
                                                     #   reason="insufficient_coin"
                                                     #   message="Insufficient coins"
                                                     #   order.status="failed", failReason="insufficient_coin"
                                                     #   失败也写入幂等缓存，重试同 requestId 仍 402 不再扣款
HTTP 400 + {message: "Bad request" | "Missing requestId" | "Amount below minimum tier"}
HTTP 401 + {message: "Unauthorized"}
HTTP 404 + {message: "Gift not found"}
```

## 幂等：双层兜底

```
Redis (热路径, TTL 600s)             MySQL (持久层)
key=idem:<userId>:<reqId>            UNIQUE INDEX(request_id) on gift_orders / super_chat_orders
value={status, body}                 防 Redis 缓存丢失 / 跨实例并发
```

并发同 requestId 的执行流：

```
T0: req#1 ─ Redis miss ─ BEGIN TX
                          UPDATE balance        ✅ -200
                          INSERT order          ✅ status=success
                          INSERT outbox         ✅
                         COMMIT
                         Redis SET idem (200, body)
T1: req#2..10 ─ 大部分 Redis hit  ─ replay (200, Idempotent-Replayed: true)
                少数 Redis 也 miss ─ BEGIN TX
                                     UPDATE balance         ✅ -200 ❌（这一步用 affected_rows=0 判，不会发生在第一笔成功后；
                                                                       但即便扣了，下一句也会回滚）
                                     INSERT order           ❌ duplicate request_id (1062)
                                    ROLLBACK
                                     SELECT existing order  ✅
                                    Redis SET idem (200, body)
                                    返回带 Idempotent-Replayed
```

测试覆盖：10 个 goroutine 同时同 `requestId` → 余额恰好减一次 → 1 行 INSERT → 1 个 success + 9 个 replay。

## 余额扣减：单 SQL 行锁 + 条件判断

```sql
UPDATE users SET coin_balance = coin_balance - ?
 WHERE id = ? AND coin_balance >= ?
```

`affected_rows = 0` 即"余额不足"，整个 TX 回滚；外层把 `failed` order 单独 INSERT 进 `gift_orders`（不在交易里），保证后续 retry 同 requestId 直接命中 unique 拿到 `failed` 行，**不再扣款**。

## 本地消息表（Transactional Outbox）

```
TX:
  UPDATE balance
  INSERT gift_orders / super_chat_orders
  INSERT local_messages (status='pending', payload=广播 JSON)
COMMIT

后台 worker（1s 轮询）:
  CLAIM (status=pending AND next_at<=now) → 推到 next_at = now+30s 防并发
  Kafka.Publish(payload)
  ┌── ok  → UPDATE status='sent'
  └── err → UPDATE retries+=1, next_at = now + base*2^retries
              if retries >= MaxRetries → status='dead'
```

`payload` 字段名严格对齐 im-gateway `internal/hub/messages.go`：
```json
{ "type": "gift",       "user": "...", "giftName": "...", "ts": 1700000000000 }
{ "type": "super_chat", "id": "sc-...", "user": "...", "amount": "1000", "tier": 2, "text": "...", "ts": 1700000000000 }
```

下游 chat/gift consumer 拿到后：

1. 更新主播收益统计
2. `ZINCRBY gift:rank:<roomId> totalCoin user`
3. `PUBLISH room:<roomId> <payload>` → im-gateway fanout

## 启动

```bash
docker compose -f deploy/docker-compose.yml up -d mysql redis
go run ./app/gift-service/cmd     # :8092
# 通过 api-gateway 访问：http://localhost:8080/api/gifts
```

## 测试

```bash
go test ./app/gift-service/...
```

覆盖：

- **gift_service_test.go**（7 用例 + 真实 SQLite）
  - **并发同 requestId 10 次** → 1 笔扣款 + 1 行 order + 9 次 replay
  - 余额边界：100 → 50×2 成功 → 再发任意一个 → 402 失败持久化 → 重发同 reqId replay 同失败
  - GiftNotFound
  - 成功路径写 outbox，payload 含 `type:"gift"`
  - 失败路径**不**写 outbox（不广播失败）
  - `AmountToTier` 12 个边界值
  - SC tier=0 拒绝且不扣款

- **outbox_service_test.go**（4 用例）
  - 失败 2 次 retry → 第 3 次成功 → status=sent
  - MaxRetries=3 全失败 → status=dead
  - Claim 跳过 next_at 在未来的行
  - Claim 不会捞已 sent 的行

## 与前端 mock 自查

| MSW 行为                                     | 后端                                                |
| -------------------------------------------- | --------------------------------------------------- |
| `Bad request` 缺 roomId/giftId/count         | 400 `{message:"Bad request"}` ✅                    |
| `Missing requestId`                          | 400 `{message:"Missing requestId"}` ✅              |
| `Gift not found`                             | 404 `{message:"Gift not found"}` ✅                 |
| 余额不足 402 `{...order, reason, message}`   | 字段一字不差展平 ✅                                 |
| 失败也缓存：重试相同 requestId 不再扣款      | DB unique + Redis 双层 ✅                           |
| 命中幂等缓存设 `Idempotent-Replayed: true`   | hot replay + DB-replay 都设此 header ✅             |
| `amountToTier` 阈值                          | `service.AmountToTier` 完全相同 ✅                  |
| 30% 随机失败                                 | **后端不模拟随机失败**——这是 mock 用来制造抖动的，
                                                  生产侧只在真实余额不足时返回 402 ✅                |

## 端到端 curl

```bash
# 拿 token
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"demo"}' | jq -r .token)

# 礼物列表
curl -s http://localhost:8080/api/gifts | jq

# 送礼
curl -i -X POST http://localhost:8080/api/gifts/send \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'X-Request-Id: req-test-1' \
  -d '{"roomId":"luna-music","giftId":"flower","count":3,"requestId":"req-test-1"}'
# → 200 {orderId, requestId, giftId, count, totalCoin:30, status:"success", createdAt}

# 立即重试同 requestId → 200 + Idempotent-Replayed: true
curl -i -X POST http://localhost:8080/api/gifts/send \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'X-Request-Id: req-test-1' \
  -d '{"roomId":"luna-music","giftId":"flower","count":3,"requestId":"req-test-1"}'

# SuperChat
curl -i -X POST http://localhost:8080/api/super-chats \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'X-Request-Id: sc-1' \
  -d '{"roomId":"luna-music","amount":1000,"text":"good stream","requestId":"sc-1"}'
# → tier:2

# 故意打高额触发 402
curl -i -X POST http://localhost:8080/api/super-chats \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'X-Request-Id: sc-2' \
  -d '{"roomId":"luna-music","amount":99999999,"text":"big","requestId":"sc-2"}'
# → 402 {orderId,...,status:"failed",failReason:"insufficient_coin",reason,message}
```
