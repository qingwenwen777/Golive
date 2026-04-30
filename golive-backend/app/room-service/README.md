# room-service

直播间列表 / 详情 / 关注 / 点赞-踩 / 主播开播 + SRS 推流鉴权。

- HTTP: `:8091`（裸路径，无 `/api` 前缀，由 api-gateway 反代附加）
- pprof: `:6063`

## 端点

| 方法 | 路径                          | 鉴权 | 说明                          |
| ---- | ----------------------------- | ---- | ----------------------------- |
| GET  | `/rooms?category&page&size`   | 否   | 列表，剥 streamKey            |
| GET  | `/rooms/:id`                  | 否   | 详情，剥 streamKey；404       |
| GET  | `/rooms/:id/follow`           | JWT  | `{channelId, following}`      |
| POST | `/rooms/:id/follow`           | JWT  | `following:true`              |
| DEL  | `/rooms/:id/follow`           | JWT  | `following:false`             |
| GET  | `/rooms/:id/like`             | JWT  | `{streamId, liked, disliked, likes}` |
| POST | `/rooms/:id/like`             | JWT  | like，互斥 dislike            |
| DEL  | `/rooms/:id/like`             | JWT  | unlike，likes--               |
| POST | `/rooms/:id/dislike`          | JWT  | dislike，互斥 like            |
| DEL  | `/rooms/:id/dislike`          | JWT  | undislike                     |
| POST | `/rooms/live`                 | JWT  | 主播开播，返回含 streamKey    |
| POST | `/srs/on_publish`             | 无   | SRS callback                  |
| POST | `/srs/on_unpublish`           | 无   | SRS callback                  |

## 数据层

- **MySQL `rooms`**：GORM AutoMigrate；启动 seed 20 行（前 12 与 `streams.ts` 一致，后 8 补分类覆盖）。
- **关注**：Redis ZSet，正向 `user:<uid>:follows`、反向 `channel:<cid>:followers`，score=ms timestamp。
- **点赞**：Redis Hash `like:<sid>:<uid>` 存 `liked/disliked`；计数器 `like:<sid>:count`。互斥切换走 4 段 Lua（保证 INCR/DECR 与 Hash 状态原子）。
- **streamKey**：Redis `streamkey:<key> → roomId`，TTL 4h。

## 启动

```bash
cd deploy && docker compose up -d mysql redis
cd ../app/room-service && go run ./cmd      # :8091
# 另一终端
cd .. && go run ./cmd/api-gateway           # :8080，反代 /api/rooms /api/srs
```

## 测试

```bash
cd app/room-service
go test ./internal/service/...
```

覆盖：

- 分类规范化（empty/all/All/ALL/すべて/Music/Apex Legends/音楽）
- streamKey JSON 序列化：空 → 不出现；非空 → 出现
- like/dislike 状态机：9 步序列，含跨态切换（dislike→like、like→dislike、unlike 不下溢）
- 多用户互不干扰

## 端到端 curl

```bash
# 0) 拿 token
curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"demo"}' | tee /tmp/login.json
TOKEN=$(jq -r .token /tmp/login.json)
H="Authorization: Bearer $TOKEN"

# 1) 列表（无分类）
curl -s 'http://localhost:8080/api/rooms?page=1&size=24' | jq '.total, .items | length'

# 2) 列表（分类筛选，case-insensitive + 日文）
curl -s 'http://localhost:8080/api/rooms?category=music' | jq '.items[].category'
curl -s 'http://localhost:8080/api/rooms?category=音楽'   | jq '.items[].categoryJa'
curl -s 'http://localhost:8080/api/rooms?category=all'    | jq '.total'   # = 全部

# 3) 详情
curl -s http://localhost:8080/api/rooms/luna-music | jq
# streamKey 字段不应出现：
curl -s http://localhost:8080/api/rooms/luna-music | jq 'has("streamKey")'  # → false

# 4) 找不到 → 404 { message:"Not found" }
curl -i http://localhost:8080/api/rooms/no-such

# 5) follow
curl -s -H "$H" http://localhost:8080/api/rooms/luna/follow                   # → following:false
curl -s -X POST -H "$H" http://localhost:8080/api/rooms/luna/follow           # → following:true
curl -s -X DELETE -H "$H" http://localhost:8080/api/rooms/luna/follow         # → following:false

# 6) like / dislike 互斥
curl -s -H "$H" http://localhost:8080/api/rooms/luna-music/like               # 初始 likes
curl -s -X POST -H "$H" http://localhost:8080/api/rooms/luna-music/like       # liked:true, likes++
curl -s -X POST -H "$H" http://localhost:8080/api/rooms/luna-music/dislike    # liked:false, disliked:true, likes--
curl -s -X DELETE -H "$H" http://localhost:8080/api/rooms/luna-music/dislike  # disliked:false
curl -s -X POST -H "$H" http://localhost:8080/api/rooms/luna-music/like       # liked:true, likes++
curl -s -X DELETE -H "$H" http://localhost:8080/api/rooms/luna-music/like     # liked:false, likes--

# 7) 未授权 → 401
curl -i http://localhost:8080/api/rooms/luna-music/like
```

## 对照 MSW 自查

| MSW 行为                                              | 后端                                        |
| ----------------------------------------------------- | ------------------------------------------- |
| `category` 空/all/すべて 不筛选                       | `service.NormalizeCategory` ✅              |
| 大小写不敏感 + 日文匹配                               | `LOWER(category)=? OR category_ja=?` ✅     |
| 分页 `page`/`size`，默认 1/24                         | ✅                                          |
| `Stream` 列表/详情都剥 `streamKey`                    | `omitempty` + 不赋值 ✅                     |
| 详情 404 `{message:"Not found"}`                      | `ErrRoomNotFound` ✅                        |
| follow GET/POST/DELETE 形状 `{channelId, following}`  | ✅                                          |
| like POST：first-time 计数 +1，第二次幂等             | Lua 用 HGET 旧值判断 ✅                     |
| like→dislike：likes-- 且 liked=false, disliked=true   | `luaDislike` ✅                             |
| dislike→like：disliked=false, liked=true, likes++     | `luaLike` ✅                                |
| unlike 不下溢                                         | `if cnt<0 then SET 0` ✅                    |
| 未授权所有 social 接口 401 `{message:"Unauthorized"}` | `AuthRequired` 中间件 ✅                    |

## SRS 集成

`deploy/srs.conf` 里需要把回调指过来（下一轮配 SRS 时一并改）：

```
vhost __defaultVhost__ {
    http_hooks {
        enabled         on;
        on_publish      http://host.docker.internal:8091/srs/on_publish;
        on_unpublish    http://host.docker.internal:8091/srs/on_unpublish;
    }
}
```

主播侧：`POST /api/rooms/live` 拿到 `streamKey="lk_xxxx"` 后，OBS 推到
`rtmp://localhost:1935/live/<streamKey>`，SRS 转发回 `/srs/on_publish` 校验。
