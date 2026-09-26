# gift-service

Gift catalog, tipping, SuperChat, and a transactional outbox.

- HTTP: `:8092` (unprefixed routes; api-gateway exposes them under `/api`)
- pprof: `:6067`

## Endpoints

| Method | Path | Auth | Description |
| ---- | ---------------- | ---- | ------------------------------------------------------------------- |
| GET | `/gifts` | No | Gift catalog |
| POST | `/gifts/send` | JWT | Send a gift; idempotent via X-Request-Id; 402 for insufficient balance |
| POST | `/gifts/fan-clubs/join` | JWT | Join a creator's fan club (Fan Light price); 409 `already_fan_club_member` for an existing member, without charging |
| POST | `/super-chats` | JWT | SuperChat; map amount→tier; reject tier 0 and text over the tier's limit; otherwise follow gifts/send rules |

## Response shapes (matching src/mocks/handlers/gift.ts)

```
HTTP 200 + body=GiftOrder                            # Success / successful replay
                                                     #   (replay also sets Idempotent-Replayed: true)
HTTP 402 + body={...GiftOrder, reason, message}        # Insufficient balance
                                                     #   reason="insufficient_coin"
                                                     #   message="Insufficient coins"
                                                     #   order.status="failed", failReason="insufficient_coin"
                                                     #   Cache failures too; the same requestId still returns 402 without another debit.
HTTP 400 + {message: "Bad request" | "Missing requestId" | "Amount below minimum tier"}
HTTP 400 + {message, reason: "super_chat_text_too_long"}  # SC text over its tier's limit: 50/100/150/200/200
                                                     #   characters (runes) for tiers 1-5, after trimming
HTTP 401 + {message: "Unauthorized"}
HTTP 404 + {message: "Gift not found"}
```

## Idempotency: two layers of protection

```
Redis (hot path, TTL 600s)           MySQL (persistence layer)
key=idem:<userId>:<reqId>            UNIQUE INDEX(request_id) on gift_orders / super_chat_orders
value={status, body}                Protects against Redis cache loss and cross-instance concurrency
```

Concurrent execution with the same requestId:

```
T0: req#1 ─ Redis miss ─ BEGIN TX
                          UPDATE balance        ✅ -200
                          INSERT order          ✅ status=success
                          INSERT outbox         ✅
                         COMMIT
                         Redis SET idem (200, body)
T1: req#2..10 ─ mostly Redis hits ─ replay (200, Idempotent-Replayed: true)
                some Redis misses ─ BEGIN TX
                                     UPDATE balance         ✅ -200 ❌ (checked via affected_rows=0; this case does not occur after the first successful request,
                                                                       but even if debited, the next statement causes a rollback)
                                     INSERT order           ❌ duplicate request_id (1062)
                                    ROLLBACK
                                     SELECT existing order  ✅
                                    Redis SET idem (200, body)
                                    Return with Idempotent-Replayed
```

Test coverage: 10 goroutines using the same `requestId` concurrently → balance debited exactly once → 1 inserted row → 1 success + 9 replays.

## Balance debit: one SQL statement with a row lock and condition

```sql
UPDATE users SET coin_balance = coin_balance - ?
 WHERE id = ? AND COALESCE(banned, false) = false
   AND coin_balance - COALESCE(frozen_coins, 0) >= ?
```

The statement lives in `pkg/wallet` (`wallet.Debit`), which also writes the `coin_transactions` row; every balance change goes through that package inside the order's transaction.

`affected_rows = 0` means insufficient balance and rolls back the entire transaction. The outer layer inserts a `failed` order into `gift_orders` separately, outside the transaction. A retry with the same requestId hits the unique constraint and retrieves the failed row, **without another debit**.

## Transactional outbox

```
TX:
  UPDATE balance
  INSERT gift_orders / super_chat_orders
  INSERT local_messages (status='pending', payload=broadcast JSON)
COMMIT

Background worker (polls every 1s):
  CLAIM (status=pending AND next_at<=now, FOR UPDATE SKIP LOCKED) → set next_at = now+30s
        so each row goes to one worker; a batch stops publishing once half the hold is used
  Kafka.Publish(payload + "eventId": local_messages.id)
  ┌── ok  → UPDATE status='sent'
  └── err → UPDATE retries+=1, next_at = now + base*2^retries
              if retries >= MaxRetries → status='dead'
```

The claim needs MySQL 8.0+ or MariaDB 10.6+ (`SKIP LOCKED`). Publishing is
still at-least-once (a publish can succeed and the `sent` update fail), so the
publisher stamps the outbox row id into the payload as `eventId` and
im-gateway ignores an `eventId` it has already counted into the contribution
leaderboard.

`payload` field names strictly match im-gateway `internal/hub/messages.go`:
```json
{ "type": "gift",       "user": "...", "giftName": "...", "ts": 1700000000000 }
{ "type": "super_chat", "id": "sc-...", "user": "...", "amount": "1000", "tier": 2, "text": "...", "ts": 1700000000000 }
```

After receiving a message, the downstream chat/gift consumer:

1. Updates creator revenue statistics.
2. `ZINCRBY gift:rank:<roomId> totalCoin user`
3. `PUBLISH room:<roomId> <payload>` → im-gateway fanout

## Startup

```bash
docker compose -f deploy/docker-compose.yml up -d mysql redis
go run ./app/gift-service/cmd     # :8092
# Access through api-gateway: http://localhost:8080/api/gifts
```

## Tests

```bash
go test ./app/gift-service/...
```

Coverage:

- **gift_service_test.go** (7 cases + real SQLite)
  - **10 concurrent requests with the same requestId** → 1 debit + 1 order row + 9 replays.
  - Balance boundaries: 100 → 50×2 succeeds → any further gift → persist a 402 failure → same reqId replays the same failure.
  - GiftNotFound
  - Success writes an outbox entry with `type:"gift"` in the payload.
  - Failure does **not** write to the outbox (failed orders are not broadcast).
  - `AmountToTier`: 12 boundary values.
  - Reject SC tier=0 without debiting.
  - Reject SC text over its tier's limit without debiting; every limit fits the columns that store the text.

- **outbox_service_test.go** (5 cases)
  - Fail twice, retry, then succeed on the third attempt → status=sent.
  - MaxRetries=3 with all attempts failing → status=dead.
  - Claim skips rows whose next_at is in the future.
  - Claim excludes rows already marked sent.
  - A batch outliving half its claim hold stops publishing; the rest are published once, later.

- **\*_mysql_test.go** (run only when `GOLIVE_TEST_MYSQL_DSN` is set, e.g.
  `root:root@tcp(127.0.0.1:3306)/?parseTime=true&loc=UTC`; each test creates
  and drops its own database): bet settle/cancel races, concurrent fan-badge
  contributions and fan club joins, concurrent outbox claims/drains, and SC
  text at the column limits.

## Verify against the frontend mock

| MSW behavior | Backend |
| -------------------------------------------- | --------------------------------------------------- |
| `Bad request` when roomId/giftId/count is missing | 400 `{message:"Bad request"}` ✅ |
| `Missing requestId`                          | 400 `{message:"Missing requestId"}` ✅              |
| `Gift not found`                             | 404 `{message:"Gift not found"}` ✅                 |
| Insufficient balance: 402 `{...order, reason, message}` | Identical flattened fields ✅ |
| Cache failures too: retrying the same requestId does not debit again | DB unique constraint + Redis protection ✅ |
| Set `Idempotent-Replayed: true` on an idempotent cache hit | Both hot replay and DB replay set this header ✅ |
| `amountToTier` thresholds | `service.AmountToTier` is identical ✅ |
| 30% random failures | **The backend does not simulate random failures**; the mock uses them to introduce instability. Production returns 402 only for actual insufficient balance ✅ |

## End-to-end curl examples

```bash
# Get a token.
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"demo"}' | jq -r .token)

# List gifts.
curl -s http://localhost:8080/api/gifts | jq

# Send a gift.
curl -i -X POST http://localhost:8080/api/gifts/send \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'X-Request-Id: req-test-1' \
  -d '{"roomId":"luna-music","giftId":"flower","count":3,"requestId":"req-test-1"}'
# → 200 {orderId, requestId, giftId, count, totalCoin:30, status:"success", createdAt}

# Retry immediately with the same requestId → 200 + Idempotent-Replayed: true.
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

# Deliberately request a large amount to trigger 402.
curl -i -X POST http://localhost:8080/api/super-chats \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'X-Request-Id: sc-2' \
  -d '{"roomId":"luna-music","amount":99999999,"text":"big","requestId":"sc-2"}'
# → 402 {orderId,...,status:"failed",failReason:"insufficient_coin",reason,message}
```
