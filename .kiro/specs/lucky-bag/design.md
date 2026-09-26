# Lucky Bag — Design

## 1. Overview

A live-room interaction in which the streamer uses their own coins to create prize packets for viewers, with an automatic draw when the countdown ends.
Reuse the full-stack betting structure: gift-service domain logic + coin debits/credits + transactional
outbox → Redis `room:<id>` fanout → im-gateway broadcast → frontend live-room panel + i18n in three languages.

Key differences from betting:
- Viewers **participate for free** (no coins required).
- Draw eligibility: viewers **must have joined and still be in the room at draw time** (checked against the Redis room presence set).
- The draw runs **automatically when the countdown ends** (gift-service background scheduler); betting is settled manually by the streamer.
- The streamer can set **eligibility**: everyone / followers / fan club / fan club with level ≥ N.

## 2. Confirmed requirements

1. The streamer funds a lucky bag from their coin balance; insufficient balance returns 402 `insufficient_coin`.
2. Set the number of packets N (= number of winning slots).
3. Amount modes: fixed (equal amount per packet) / random (split the total randomly into N packets, each ≥ 1).
4. Countdown defaults to 60s and is capped at 600s (10 minutes) by the frontend.
5. Viewers click to join for free; login is required.
6. The draw requires both prior participation and presence in the room; absent viewers forfeit eligibility.
7. If participants < N, the actual winner count is the number of participants present; **remaining packets are refunded to the streamer**.
8. Each viewer can win at most one packet per draw.
9. Four eligibility tiers: `all` / `followers` / `fans` / `fans_level` (min_fan_level).
10. Only one active lucky bag is allowed per room at a time.
11. The streamer can cancel manually before the draw and receive a full refund.

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
  card (countdown, Join button, my-entry/result state). Mirrors `BettingPanel` styling
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
