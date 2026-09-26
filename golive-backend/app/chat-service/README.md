# chat-service

Chat moderation / rate limiting / persistence / broadcasting. No HTTP write endpoint; the only public application endpoint serves chat history for replay. History renders each chat's name, avatar, level and fan badge from current server-side data looked up by user id (never by display name, never the values stored with the row).

Internal (service-to-service, not proxied by api-gateway): `GET /internal/rooms/:id/fan-badges/:userId` → `{"fanBadge": {"creatorId","level"} | null}`, used by im-gateway to decorate live chat.

- HTTP: `:8093` (only `/rooms/:id/danmus` + `/healthz`, proxied by api-gateway)
- pprof: `127.0.0.1:6068` (loopback only; an empty `service.pprof_addr` disables it)

## Data flow

Deployed today (`kafka.enabled: false` in chat-service and im-gateway, the default): im-gateway
moderates live chat and publishes it to Redis `room:<id>`; chat-service subscribes to the same
channels (`internal/redissub`) and persists each message. No Kafka client is created.

The Kafka pipeline below is optional. It needs a broker (Compose profile `kafka`) and
`kafka.enabled: true` in both im-gateway and chat-service:

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

## Four-stage pipeline

| Stage | Implementation |
| ---------- | --------------------------------------------------------------------------------------------- |
| Rate limiting | `pkg/chatlimit`: Redis fixed window + Lua (`INCR` + `PEXPIRE`), 3 msg/sec per user by default (`ratelimit.bucket_seconds` is an integer number of seconds). Drop immediately when exceeded. |
| Sensitive-word filtering | `pkg/chatfilter` (shared with im-gateway): custom DFA (rune-level trie) over normalised text (zero-width chars ignored, NFKD/fullwidth and case folded). Longest match first; short Latin words match whole words only; optional skipped characters `". *-_"`. |
| Persistence | MySQL **8 sharded tables** `danmus_0..7`, selected by `fnv32(roomId) % 8`. A room always uses the same table. |
| Broadcasting | Redis `PUBLISH room:<roomId>`; im-gateway subscribes and fans out to WebSocket clients. |

## Internal concurrency

The Kafka consumer processes messages using N worker goroutines in parallel. **The dispatcher routes each record to a worker using `fnv32(roomId) % N`**. This provides:

- Natural parallelism across rooms (CPU utilization ≈ N).
- A fixed worker per room, **preserving order** (user A's message is processed before user B's subsequent message).
- The worker and MySQL shard routing share the same hash, allowing warm connection-pool reuse.

Offsets use `DisableAutoCommit + CommitUncommittedOffsets` and are committed only after dispatch. Retries or dead-letter handling for worker processing errors are deferred; the current MVP logs and drops failures so consumption can advance. Delivery semantics are **at least once**; UUID danmu primary keys deduplicate repeated messages.

## DFA implementation notes

- Rune trie (`map[rune]*node`) with first-class CJK support.
- `Replace(text)` scans once; at each starting position it takes the longest match and skips to its end, preventing overlapping replacements.
- Fold ASCII letters to lowercase; use `unicode.ToLower` for non-ASCII characters.
- `WithSkipChars(" .*-")` also matches `s.h.i.t`; characters are skipped only after matching has begun, avoiding empty matches.

Test coverage: basic matches, case folding, longest prefix (`sh` vs `shit`), nonoverlap (`abcab → ***ab`), empty dictionary, skipped characters, no false match for a partial CJK prefix, and a large-dictionary smoke test.

## Sharding strategy

```
shard(roomId) = fnv32(roomId) % shards
table         = "danmus_" + str(shard)
```

| Option | Description |
| ------------------ | ----------------------------------------------- |
| Shard count | `mysql.shards`, default 8 |
| Hash | FNV-1a 32-bit (cheap and evenly distributed) |
| Primary key | UUID v4 (no write hot spots; retries deduplicate naturally) |
| Indexes | `room_id`, `ts` for history queries; `user_id` for bans |

Query the most recent 50 messages in a room:

```sql
-- GORM selects the table automatically; handwritten SQL must also specify the shard:
SELECT * FROM danmus_3
WHERE room_id = 'luna-music'
ORDER BY ts DESC
LIMIT 50;
```

Cross-room statistics require UNION ALL across all shards. Online cross-room aggregation is rare; move it to an offline OLAP layer such as ClickHouse / Hive.

## Startup

MySQL and Redis must be reachable at the addresses in `configs/config.yaml`; Kafka only when
`kafka.enabled` is true.

```bash
go run ./app/chat-service/cmd
```

At startup, the service:

1. Runs AutoMigrate on the 8 sharded tables.
2. Loads `configs/sensitive.txt` to build the DFA.
3. Subscribes to Redis `room:*` and persists live chat.
4. Only with `kafka.enabled: true` (env `CHATSVC_KAFKA_ENABLED`): joins Kafka consumer group
   `chat-consumer` and subscribes to the `danmu` topic.

## Tests

```bash
go test ./app/chat-service/...
```

Coverage:

- DFA: 9 cases (matches / case folding / longest match / nonoverlap / empty dictionary / skipped characters / CJK / performance smoke).
- Rate limiting: 4 cases (allow / reject / per-user isolation / reset across windows using miniredis FastForward).

## Load testing

```bash
go run -tags loadtest ./app/chat-service/loadtest \
  -brokers 127.0.0.1:9092 \
  -redis 127.0.0.1:6379 \
  -qps 5000 -rooms 50 -duration 60s
```

Reports end-to-end p50/p95/p99 latency (from production to Redis receipt). Targets:

- Sustain 5000 QPS for 60s without drops.
- e2e p99 < 50ms locally / < 200ms on a container network.
- MySQL `INSERT` writes should not be the bottleneck: 8 tables distribute hot spots.

## HTTP

```bash
curl 'http://localhost:8093/rooms/luna-music/danmus?limit=20'
# { "items": [ {type:"chat", id, user, text, ts}, ... ] }
```

The endpoint is also accessible through api-gateway at `/api/rooms/:id/danmus` (add a proxy route to chat-service in the gateway router).

## Contract with im-gateway

- Kafka input: `{roomId, userId, username?, avatar?, text, ts}` JSON, `Key=roomId`.
- Redis output: `PUBLISH room:<roomId>` with a `hub.ChatMsg` body:
  ```json
  {"type":"chat","id":"uuid","user":"...","avatar":"...","text":"...","color":"...","ts":1700000000000}
  ```
  Field names match `ChatMsg` in im-gateway `internal/hub/messages.go`.
