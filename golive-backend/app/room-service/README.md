# room-service

Live-room lists / details / follows / likes and dislikes / creator go-live flow + SRS publishing authentication.

- HTTP: `:8091` (unprefixed routes; api-gateway exposes them under `/api` through its reverse proxy)
- pprof: `:6063`

## Endpoints

| Method | Path | Auth | Description |
| ---- | ----------------------------- | ---- | ----------------------------- |
| GET | `/rooms?category&page&size` | No | List; strip streamKey |
| GET | `/rooms/:id` | No | Details; strip streamKey; 404 if missing |
| GET  | `/rooms/:id/follow`           | JWT  | `{channelId, following}`      |
| POST | `/rooms/:id/follow`           | JWT  | `following:true`              |
| DEL  | `/rooms/:id/follow`           | JWT  | `following:false`             |
| GET  | `/rooms/:id/like`             | JWT  | `{streamId, liked, disliked, likes}` |
| POST | `/rooms/:id/like` | JWT | Like; mutually exclusive with dislike |
| DEL  | `/rooms/:id/like`             | JWT  | unlike, likes--               |
| POST | `/rooms/:id/dislike` | JWT | Dislike; mutually exclusive with like |
| DEL  | `/rooms/:id/dislike`          | JWT  | undislike                     |
| POST | `/rooms/live` | JWT | Creator goes live; response includes streamKey |
| POST | `/srs/on_publish` | No | SRS callback |
| POST | `/srs/on_unpublish` | No | SRS callback |

## Data layer

- **MySQL `rooms`**: GORM AutoMigrate; seed 20 rows at startup (the first 12 match `streams.ts`; the remaining 8 cover additional categories).
- **Follows**: Redis ZSets, forward `user:<uid>:follows` and reverse `channel:<cid>:followers`, with millisecond timestamps as scores.
- **Likes**: Redis Hash `like:<sid>:<uid>` stores `liked/disliked`; `like:<sid>:count` holds the count. Four Lua scripts perform mutually exclusive transitions, keeping INCR/DECR atomic with Hash state changes.
- **streamKey**: Redis `streamkey:<key> → roomId`, TTL 4h.

## Startup

```bash
cd deploy && docker compose up -d mysql redis
cd ../app/room-service && go run ./cmd      # :8091
# In another terminal:
cd .. && go run ./cmd/api-gateway           # :8080, proxies /api/rooms
```

## Tests

```bash
cd app/room-service
go test ./internal/service/...
```

Coverage:

- Category normalization (empty/all/All/ALL, Japanese all/music labels, Music, Apex Legends).
- streamKey JSON serialization: omitted when empty; included when nonempty.
- Like/dislike state machine: a 9-step sequence including transitions (dislike→like, like→dislike, and unlike without underflow).
- Isolation between users.

## End-to-end curl examples

```bash
# 0) Get a token.
curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"demo"}' | tee /tmp/login.json
TOKEN=$(jq -r .token /tmp/login.json)
H="Authorization: Bearer $TOKEN"

# 1) List without a category filter.
curl -s 'http://localhost:8080/api/rooms?page=1&size=24' | jq '.total, .items | length'

# 2) List with category filtering: case-insensitive and Japanese matching.
curl -s 'http://localhost:8080/api/rooms?category=music' | jq '.items[].category'
curl -s 'http://localhost:8080/api/rooms?category=%E9%9F%B3%E6%A5%BD' | jq '.items[].categoryJa'
curl -s 'http://localhost:8080/api/rooms?category=all' | jq '.total'   # All categories

# 3) Details.
curl -s http://localhost:8080/api/rooms/luna-music | jq
# The streamKey field must not appear:
curl -s http://localhost:8080/api/rooms/luna-music | jq 'has("streamKey")'  # → false

# 4) Not found → 404 { message:"Not found" }
curl -i http://localhost:8080/api/rooms/no-such

# 5) follow
curl -s -H "$H" http://localhost:8080/api/rooms/luna/follow                   # → following:false
curl -s -X POST -H "$H" http://localhost:8080/api/rooms/luna/follow           # → following:true
curl -s -X DELETE -H "$H" http://localhost:8080/api/rooms/luna/follow         # → following:false

# 6) Like and dislike are mutually exclusive.
curl -s -H "$H" http://localhost:8080/api/rooms/luna-music/like               # Initial likes
curl -s -X POST -H "$H" http://localhost:8080/api/rooms/luna-music/like       # liked:true, likes++
curl -s -X POST -H "$H" http://localhost:8080/api/rooms/luna-music/dislike    # liked:false, disliked:true, likes--
curl -s -X DELETE -H "$H" http://localhost:8080/api/rooms/luna-music/dislike  # disliked:false
curl -s -X POST -H "$H" http://localhost:8080/api/rooms/luna-music/like       # liked:true, likes++
curl -s -X DELETE -H "$H" http://localhost:8080/api/rooms/luna-music/like     # liked:false, likes--

# 7) Unauthorized → 401.
curl -i http://localhost:8080/api/rooms/luna-music/like
```

## Verify against MSW

| MSW behavior | Backend |
| ----------------------------------------------------- | ------------------------------------------- |
| Empty/all/Japanese all `category` means no filter | `service.NormalizeCategory` ✅ |
| Case-insensitive + Japanese matching | `LOWER(category)=? OR category_ja=?` ✅ |
| Pagination via `page`/`size`, default 1/24 | ✅ |
| Strip `streamKey` from `Stream` lists and details | `omitempty` + leave unset ✅ |
| Details 404 `{message:"Not found"}` | `ErrRoomNotFound` ✅ |
| Follow GET/POST/DELETE shape `{channelId, following}` | ✅ |
| Like POST: first request increments by 1; second is idempotent | Lua checks the old value with HGET ✅ |
| like→dislike: likes-- and liked=false, disliked=true | `luaDislike` ✅ |
| dislike→like: disliked=false, liked=true, likes++     | `luaLike` ✅                                |
| Unlike never underflows | `if cnt<0 then SET 0` ✅ |
| All unauthorized social requests return 401 `{message:"Unauthorized"}` | `AuthRequired` middleware ✅ |

## SRS integration

In `deploy/srs.conf`, point callbacks to the internal room-service address, bypassing the public api-gateway:

```
vhost __defaultVhost__ {
    http_hooks {
        enabled         on;
        on_publish      http://host.docker.internal:8091/srs/on_publish;
        on_unpublish    http://host.docker.internal:8091/srs/on_unpublish;
    }
}
```

Creator flow: `POST /api/rooms/live` returns `streamKey="<roomId>?key=lk_xxxx"`. In OBS, set the
server to `rtmp://localhost:1935/live` and paste that value as the stream key. SRS calls
`/srs/on_publish`, which checks that the `key` parameter belongs to the `<roomId>` stream.
Viewers play `/live/<roomId>.flv`, so the secret never appears in public URLs.
