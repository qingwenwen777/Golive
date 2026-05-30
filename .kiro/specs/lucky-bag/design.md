# 福袋 (Lucky Bag) — Design

## 1. Overview

直播间互动玩法，主播用自己的 coins 包成若干红包发给观众，倒计时结束自动开奖。
照搬竞猜 (betting) 的全栈骨架：gift-service 领域逻辑 + coins 扣减/发放 + 本地消息表
outbox → Redis `room:<id>` fan-out → im-gateway 广播 → 前端直播间面板 + 三语 i18n。

与竞猜的关键差异：
- 观众**免费参与**（不花 coins）。
- 开奖判定：**必须已参与且开奖时仍在直播间**才有中奖资格（用 Redis 房间在线集合判定）。
- 倒计时结束**自动开奖**（gift-service 后台调度器），竞猜是主播手动结算。
- 主播可设定**参与资格**：全部 / 关注 / 粉丝团 / 粉丝团且等级≥N。

## 2. Confirmed requirements

1. 主播用余额 coins 包福袋；余额不足返回 402 `insufficient_coin`。
2. 设定红包个数 N（=中奖名额）。
3. 金额模式：固定（每份相同）/ 拼手气随机（总额随机拆 N 份，每份≥1）。
4. 倒计时默认 60s，上限 600s（10 分钟），前端限制。
5. 观众免费点击参与；未登录不可参与。
6. 开奖须"已参与 + 在房间内"；不在房间者视为弃权。
7. 参与人数 < N：实际中奖人数 = 参与且在场人数，**剩余红包退还主播**。
8. 一个观众一次最多中一个红包。
9. 参与资格四档：`all` / `followers` / `fans` / `fans_level`(min_fan_level)。
10. 一个直播间同一时间只允许一个进行中的福袋。
11. 主播可在开奖前手动取消，全额退还主播。

## 3. Data model (gift-service, MySQL `golive`)

### lucky_bags
| col | type | note |
| --- | --- | --- |
| id | varchar(64) PK | `lb-`+uuid |
| room_id | varchar(64) index | |
| owner_id | varchar(36) index | |
| message | varchar(255) | optional blessing text |
| total_coin | bigint | escrowed at open |
| count | int | packet/winner count |
| amount_mode | varchar(16) | `fixed` / `random` |
| eligibility | varchar(16) | `all`/`followers`/`fans`/`fans_level` |
| min_fan_level | int default 0 | for `fans_level` |
| status | varchar(16) index | `open`/`drawn`/`cancelled` |
| close_at | datetime index | |
| drawn_at | *datetime | |
| created_at / updated_at | | |

### lucky_bag_entries
| col | type | note |
| --- | --- | --- |
| id | varchar(64) PK | `lbe-`+uuid |
| bag_id | varchar(64) index; uniq(bag_id,user_id) | |
| room_id | varchar(64) index | |
| user_id | varchar(36); uniq(bag_id,user_id) | |
| status | varchar(16) | `joined`/`won`/`missed` |
| payout | bigint default 0 | |
| created_at / updated_at | | |

### coin tx types (append to model/coin.go)
`lucky_bag_send` (owner −total), `lucky_bag_payout` (winner +amount),
`lucky_bag_refund` (owner +remainder/cancel).

### outbox topic
`OutboxTopicLuckyBag = "lucky_bag"`. Broadcast payload `type:"lucky_bag"`,
`event: opened|joined|drawn|cancelled`. Published to `room:<roomId>` like bet.

## 4. API (gift-service, /api stripped by gateway)

| method | path | auth | body / resp |
| --- | --- | --- | --- |
| GET | /lucky-bags/latest?roomId= | optional | `LuckyBagView` (bag + myEntry + counts) |
| POST | /lucky-bags | auth, host | `{roomId,totalCoin,count,amountMode,eligibility,minFanLevel,durationSeconds,message}` |
| POST | /lucky-bags/:id/join | auth | participate; eligibility + presence checked |
| POST | /lucky-bags/:id/cancel | auth, host | refund owner |

`LuckyBagView`: `{ bag, myEntry?, participantCount, winners? }`.
Errors mirror bet reasons: `insufficient_coin`, `active_lucky_bag_exists`,
`lucky_bag_closed`, `lucky_bag_already_joined`, `lucky_bag_not_eligible`,
`forbidden`, `lucky_bag_not_found`, `bad_lucky_bag`.

## 5. Presence (im-gateway → Redis)

Add to `pubsub.Broker`:
- `AddPresence(ctx, roomID, userID) error`
- `RemovePresence(ctx, roomID, userID) error`

RedisBroker: SET key `room:<id>:presence`, member = userId, `EXPIRE` 6h refresh.
`room.go` add/remove/updateViewer → call broker presence for authenticated,
non-owner viewers. Best-effort (errors logged at debug). fakeBroker in tests gets
no-op impls.

gift-service draw reads `SMEMBERS room:<id>:presence` via its own redis client to
get the set of present userIds.

## 6. Draw algorithm (at close_at, gift-service scheduler)

Background goroutine polls every 1s for `status=open AND close_at<=now`, draws in a
single DB tx:
1. Load entries `status=joined`.
2. `present = SMEMBERS room:<id>:presence`; eligible = entries ∩ present.
3. `winners = min(count, len(eligible))`.
4. Compute `count` packet amounts:
   - fixed: each = `total/count`; remainder added to first packet.
   - random: split `total` into `count` parts, each ≥1 (double-average algorithm).
5. Shuffle eligible, take first `winners`; assign packets[0..winners-1]; credit each
   winner (`+payout`, coin tx `lucky_bag_payout`), entry → `won`.
6. Non-winning joined entries → `missed`.
7. `refund = total - sum(paid packets)`; if >0 credit owner (`lucky_bag_refund`).
8. bag → `drawn`, `drawn_at=now`; outbox `drawn` event with winners summary.

Manual cancel: bag → `cancelled`, full refund to owner, outbox `cancelled`.

## 7. Frontend

- `src/types/luckyBag.ts` — types.
- `src/api/luckyBag.ts` — `useLatestLuckyBag` (poll 2s), `useOpenLuckyBag`,
  `useJoinLuckyBag`, `useCancelLuckyBag`.
- `src/features/live-room/LuckyBagPanel.tsx` — owner create form (count, amount mode,
  fixed/total amount, eligibility select + min level, duration) + viewer participate
  card (countdown, 参与 button, my-entry/result state). Mirrors `BettingPanel` styling
  (`gl-bag-*` classes echoing `gl-bet-*`).
- Mount: CreatorStudioPage live console (owner, next to Betting module) and
  LiveRoomPage (viewer-facing, in the main column under InfoBlock for live rooms).
- `useRoomRealtime.ts` — handle `type:"lucky_bag"` events: invalidate
  `luckyBagQueryKey`, push a system message (opened / drawn / cancelled), invalidate
  `['me']` on drawn/cancelled (balances changed).
- i18n: `luckyBag.*` block in zh-CN / ja-JP / en-US `pages.json` (use `_one/_other`
  for counts per project convention).
- CSS: `gl-bag-*` in `index.css`, consistent with `gl-bet-*` (rounded cards,
  `--gl-*` vars, no gradients).

## 8. Verification

- Backend: `go build ./...`, `go vet`, gift-service tests.
- Frontend: typecheck, lint (0 errors), locale parity test, production build.
- Deploy: commit + push prod (rebuilds gift-service + im-gateway + frontend).
