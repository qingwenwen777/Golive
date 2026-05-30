# 设计文档：观众语音连麦 (Mic-Link)

## 概述

本设计实现「观众语音连麦」：直播间已登录观众向主播申请纯语音上麦，主播审批后嘉宾麦克风经 WebRTC 上行到 SRS，主播在 OBS 里通过「连麦舞台」浏览器源把嘉宾音频混入直播流，所有观众经现有 FLV/HLS 播放即可听到。最多 3 人同时在麦。

实现复用现有架构，**未新增微服务、未新增数据库表**：
- 连麦领域逻辑放在 **gift-service**（已托管竞猜、福袋等直播间互动，且已有 `RoomOwner` / `RoomChannelID` / `FanBadgeLevelFor` 资格判定与 `room:<id>` Redis 扇出）。
- 连麦会话状态是临时性的，存于 **Redis**（每房一份 JSON 文档 + 短锁），TTL 6 小时。
- 信令复用现有 `room:<id>` → im-gateway → `useRoomRealtime` 实时通道，新增 `mic_link` 事件类型。
- 媒体面启用 **SRS `rtc_server`**（WHIP 上行 / WHEP 下行，audio-only Opus）。

## 架构

### 媒体流向

```
嘉宾浏览器 ──WHIP(音频)──> SRS rtc_server ──WHEP(音频)──> 连麦舞台页(OBS浏览器源)
                                                              │
主播OBS麦克风 ──────────────────────────────────────────────┴── 混音 ──RTMP──> SRS ──FLV/HLS──> 所有观众
```

- 嘉宾流名：`miclink-<roomId>-<userId>`。room-service 的 SRS `on_publish` / `on_unpublish` 钩子对 `miclink-` 前缀的流直接放行（不做房间账务）。
- 信令（SDP offer/answer）走 HTTPS：nginx 新增 `location ^~ /rtc/` 反代到 `srs:1985`。
- 媒体走 UDP 8000（SRS 容器新增 `8000:8000/udp` 端口，`CANDIDATE` 环境变量设为公网 IP `154.36.185.85`）。

### 信令流向

连麦的开关 / 申请 / 审批 / 上下麦 / 静音等状态变化，由 gift-service 发布到 Redis `room:<id>`，im-gateway 扇出到 WebSocket，前端 `useRoomRealtime` 收到 `mic_link` 事件后失效 `['mic-link', roomId]` 查询触发重新拉取。前端同时以 3 秒轮询兜底，保证刷新 / 断线后状态最终一致。

## 组件与接口

### 后端 (gift-service)

**Redis 状态文档** `miclink:<roomId>`：
```
{ roomId, ownerId, enabled, eligibility, minFanLevel, requests[], roster[], updatedAt }
```
每次写操作前用 `miclink:lock:<roomId>` 短锁（SetNX + Lua 释放）保证读改写原子，从而严格执行 3 人上限。

**Service**（`mic_link_service.go`）：`Latest` / `Config`(owner) / `Request` / `Cancel` / `Leave` / `Mute` / `Approve`(owner) / `Reject`(owner) / `Remove`(owner)。资格判定 `all/followers/fans/fans_level` 与福袋同源。

**HTTP**（`mic_link.go`，挂在 gift-service，`/api` 前缀由网关剥离）：
- `GET  /mic-link/latest`（公开只读，按调用者投影）
- `POST /mic-link/config|request|cancel|leave|mute|approve|reject|remove`（需登录）

**网关**（`router.go`）：`api.Any("/mic-link/*action", giftProxy)`；只读 GET 登记为 public route。

### 前端

- `types/micLink.ts`、`api/micLink.ts`（React Query hooks，轮询 3s）。
- `lib/micRtc.ts`：WHIP `publishMic` / WHEP `playMic`，流名 `micStreamName`。
- `features/live-room/MicLinkPanel.tsx`：`ownsStream` 分流——主播控制台（开关 + 资格下拉复用 `gl-bag-select-*` + 申请队列 + 在麦名单 + OBS 舞台地址行）/ 观众面板（申请按钮 + 状态机 + 自助静音/下麦 + 只读在麦名单）。嘉宾在 `on_air` 时由该组件管理 WHIP 上行生命周期。
- `pages/MicStagePage.tsx`：无壳独立路由 `/mic-stage/:roomId`，轮询在麦名单并对每位嘉宾建立 WHEP 订阅、用隐藏 `<audio>` 播放，供 OBS 浏览器源采集。
- 挂载：`LiveRoomPage`（观众）、`CreatorStudioPage` 控制台（主播），与 `LuckyBagPanel` 并列。
- 样式：`gl-mic-*`，蓝色强调（区别于福袋金色），复用 `--gl-*` 变量、无渐变、自定义下拉、无卡中卡。
- 三语：`micLink.*` 键覆盖 zh-CN / ja-JP / en-US，计数键 `_one`/`_other`，通过 locale parity 测试。

## 数据模型

无新增 MySQL 表。仅 Redis 临时状态（见上）。媒体不落库。

## 错误处理

机器可读原因码：`mic_link_disabled` / `mic_link_not_eligible` / `mic_link_request_exists` / `mic_link_slot_full` / `mic_link_request_not_found` / `mic_link_busy` / `forbidden`，前端映射为本地化 toast。

## 测试策略

- 后端：`mic_link_service_test.go`（miniredis + sqlite）覆盖开关门禁、仅房主配置、审批入名单与 3 人上限、重复申请拒绝、静音/下麦、房主移除、关闭清空、关注资格门禁。
- 前端：复用既有 typecheck / lint / locale parity / build 流水线。
- WebRTC 媒体面属端到端范畴，依赖真实 SRS + 浏览器，无法在 CI 单测；通过部署后人工验证（主播开关 → 观众申请 → 审批 → 上麦出声 → 下麦）。

## 部署影响

- `deploy/srs.conf`：启用 `rtc_server`（listen 8000，candidate `$CANDIDATE`）与 vhost `rtc`。
- `deploy/docker-compose.yml`：SRS 新增 `8000:8000/udp` 与 `CANDIDATE` 环境变量。
- `deploy/nginx.conf` + `nginx.https.conf`：新增 `/rtc/` → `srs:1985` 反代。
- 内存增量约 +30~90MB（纯音频）。**注意：服务器防火墙需放行 UDP 8000。**
