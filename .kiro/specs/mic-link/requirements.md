# Requirements Document

观众语音连麦 (Audience Voice Mic-Link)

## Introduction

本功能为 GoLive 直播平台新增「观众语音连麦」(Mic-Link) 能力：直播间内已登录的观众可以向主播发起**纯语音**连麦申请，主播在申请队列中逐条审批（批准 / 拒绝），通过后嘉宾的麦克风音频被推上直播间并混入主播的输出画面，房间内所有观众都能听到。直播间同一时间最多允许 **3 名嘉宾**同时在麦。主播可随时开启 / 关闭连麦总开关、限定申请资格、把已在麦的嘉宾下麦（踢出）；嘉宾本人也可随时主动挂断 / 下麦。

本功能**仅做语音连麦，不含视频 / 摄像头**，这是已确认且不会改变的产品决策（目的之一是把宿主机内存开销控制在可接受范围）。

本文档覆盖：用户故事、EARS 格式验收标准、术语表、详细的 UI 设计期望，以及三语 i18n 要求。文档聚焦「要做什么 (WHAT)」与「为什么 (WHY)」；具体「怎么做 (HOW)」留待 design 阶段。用户已明确要求文档「什么都要有，包括 UI 怎么设计」，故本文档专门包含 UI 设计章节。

### 已确认的产品决策 (Confirmed Product Decisions)

1. **仅语音连麦**：无视频 / 摄像头，仅上行 Opus 音频。此决策为最终决策。
2. **并发上限 = 3**：同一直播间最多 3 名嘉宾同时在麦。
3. **主播主控开关**：主播必须先开启「连麦」功能，观众才能申请；主播可随时关闭。
4. **申请资格沿用福袋资格模型**：`all`（所有登录观众）/ `followers`（关注者）/ `fans`（粉丝团成员）/ `fans_level`（粉丝团且等级 ≥ 最低等级 `minFanLevel`）。未登录观众一律不能连麦。
5. **审批 + 双向下麦**：主播逐条审批每个申请，并可随时把在麦嘉宾下麦；嘉宾本人也可主动下麦。

### 已确认的技术方向 (Confirmed Technical Direction)

> 以下为已被用户接受的「可落地」技术路径，作为 design 阶段的方向约束，本需求文档的验收标准均应可在此架构上实现。具体实现细节在 design.md 固化。

- **主播主流不变**：OBS → RTMP → SRS (ossrs/srs:5) → HTTP-FLV / HLS，普通观众的播放链路与现状完全一致，观众端**无需 WebRTC**。
- **嘉宾上行走 WebRTC (WHIP)**：嘉宾麦克风音频（audio-only / Opus）通过 WHIP 推到 SRS 的 `rtc_server`。
- **连麦舞台页 (Mic Stage Page)**：一个网页通过 WHEP 订阅并播放所有已批准嘉宾的音频；主播把该页作为 **OBS 浏览器源 (Browser Source)** 加入，由 OBS 把嘉宾音频混入对外 RTMP。因此**所有观众通过现有播放链路即可听到嘉宾声音**。
- **SRS 需启用 `rtc_server`**：开放 UDP 8000 并配置公网 candidate IP（`154.36.185.85`）。coturn / TURN 为**可选回退**（服务器已有公网 IP，STUN-only 大概率足够），不作为硬性前置条件。
- **内存预算**：启用纯音频 WebRTC 预计新增约 +30MB ~ +90MB，在该受限宿主机（无 swap）上可接受；排除视频正是为控制开销。

### 与现有架构的契合点 (Architecture Fit — 背景，非需求)

- **信令复用现有实时链路**：连麦的申请 / 批准 / 拒绝 / 上麦 / 下麦 / 移除 / 名单变化等事件，复用现有 `room:<RoomID>` Redis 频道 → `im-gateway` WebSocket 扇出 → 前端 `useRoomRealtime.ts` 的实时通道（与弹幕、礼物、福袋、竞猜一致）。
- **新 API 前缀必须注册**：连麦的 HTTP 接口前缀必须在 `golive-backend/app/api-gateway/internal/router/router.go` 注册，否则一律 404。
- **资格模型已存在**：福袋已实现 `all` / `followers` / `fans` / `fans_level(minFanLevel)`；关注关系存于 Redis（`user:<id>:follows`），粉丝团等级通过 `FanBadgeLevelFor` 判定。连麦沿用相同语义。
- **前端约定**：复用 `golive-web/src/features/live-room/` 面板模式与 `gl-*` 样式（`golive-web/src/styles/index.css`）；下拉一律用自定义 `DropdownMenu`（非原生 `<select>`）；复用 `Avatar` / `Dialog` / `DropdownMenu` 等 `components/ui` 组件；不做卡中卡嵌套；除既有小型强调按钮外不用渐变；不用超大按钮。连麦面板像 `LuckyBagPanel` 一样同时挂在 `LiveRoomPage`（观众侧）与 `CreatorStudioPage`（主播侧）。
- **三语 i18n 强制**：所有面向用户的文案必须同时提供 zh-CN / ja-JP / en-US（`golive-web/src/i18n/locales/*/pages.json`），计数键用 `_one` / `_other`，并通过既有 locale parity 测试（`pnpm test`）。
- **鉴权**：观众可未登录浏览；发起连麦等写操作需登录（网关注入 `X-User-Id`）。

## Glossary

- **Mic_Link_Service**：后端连麦领域服务，负责连麦会话、申请的状态机、资格 / 权限校验、3 人并发额度控制，并通过 Redis 发布连麦事件。可作为新微服务或并入现有 `room-service`（design 阶段确定）。
- **IM_Gateway**：现有 `im-gateway` 服务，通过 WebSocket 向房间内客户端扇出 `room:<RoomID>` 频道事件。
- **SRS_RTC**：SRS 的 `rtc_server` 组件，承载嘉宾的 WHIP 上行与连麦舞台页的 WHEP 下行音频（audio-only / Opus）。
- **Mic_Link_Session**：一个直播间内进行中的连麦会话，包含房主与 0~3 名在麦嘉宾。
- **Mic_Request**：观众发起的一次连麦申请，状态为 `pending` / `approved` / `rejected` / `cancelled` / `on_air` / `removed` 之一。
- **Mic_Guest**：申请被批准并已上麦、其音频正在被广播的观众。
- **Request_Queue**：主播侧展示的待处理（`pending`）连麦申请列表，按发起时间升序。
- **On_Air_Roster**：当前在麦嘉宾名单（最多 3 人），含每名嘉宾的昵称、头像、静音 / 说话状态。
- **Room_Owner**：直播间所属主播（房主），由 `room:owner:<RoomID>` 标识，是唯一可管理连麦的人。
- **Viewer**：观看直播的用户，可能未登录。
- **Mic_Link_Console**：主播侧管理连麦的 UI 面板（总开关 + 资格设置 + 申请队列 + 在麦名单 + OBS 舞台地址），挂在 `CreatorStudioPage`。
- **Mic_Link_Panel**：观众侧申请 / 在麦的 UI 面板，挂在 `LiveRoomPage`。
- **On_Mic_Indicator**：房间内向所有观众展示「当前谁在麦上」的指示组件。
- **Mic_Stage_Page**：通过 WHEP 订阅并播放所有在麦嘉宾音频的网页，由主播作为 OBS 浏览器源加入，从而把嘉宾音频混入对外直播。
- **Eligibility**：申请资格档位，取值 `all` / `followers` / `fans` / `fans_level`，与福袋一致。
- **Concurrency_Limit**：单个直播间同时在麦嘉宾上限，固定为 **3**。
- **RoomID**：直播间标识。

## Requirements

### Requirement 1: 主播开启 / 关闭连麦功能

**User Story:** 作为主播，我想为本场直播开启或关闭连麦功能，以便决定本场是否接受观众语音连麦。

#### Acceptance Criteria

1. WHEN Room_Owner 在 Mic_Link_Console 开启连麦总开关, THE Mic_Link_Service SHALL 将该 RoomID 的连麦状态置为「已开启」并允许接收新的 Mic_Request。
2. WHEN Room_Owner 关闭连麦总开关, THE Mic_Link_Service SHALL 将该 RoomID 的连麦状态置为「已关闭」并拒绝所有新的 Mic_Request，返回错误原因 `mic_link_disabled`。
3. WHILE 连麦状态为「已关闭」, THE Mic_Link_Panel SHALL 向 Viewer 隐藏或禁用「申请连麦」入口并展示「主播未开启连麦」的本地化提示。
4. WHEN Room_Owner 关闭连麦总开关且当前存在在麦的 Mic_Guest, THE Mic_Link_Service SHALL 结束所有在麦嘉宾的连麦、释放其媒体资源，并对每名嘉宾发布 `event=removed`（原因 `feature_disabled`）。
5. IF 非 Room_Owner 的用户尝试切换连麦总开关, THEN THE Mic_Link_Service SHALL 拒绝该操作并返回错误原因 `forbidden`。
6. WHEN 连麦总开关状态发生变化, THE Mic_Link_Service SHALL 通过 `room:<RoomID>` 频道发布连麦事件（`event=feature_enabled` 或 `event=feature_disabled`）以同步各端 UI。

### Requirement 2: 主播配置连麦申请资格

**User Story:** 作为主播，我想限定哪些观众可以申请连麦，以便控制连麦人群与房间秩序。

#### Acceptance Criteria

1. THE Mic_Link_Service SHALL 支持申请资格档位 `all`（所有登录观众）/ `followers`（关注者）/ `fans`（粉丝团成员）/ `fans_level`（粉丝团且等级 ≥ `minFanLevel`），语义与福袋 `LuckyBagEligibility` 一致。
2. WHEN Room_Owner 设置申请资格, THE Mic_Link_Service SHALL 记录该 RoomID 的当前 Eligibility 与（当为 `fans_level` 时的）`minFanLevel`。
3. WHERE 申请资格为 `followers`, THE Mic_Link_Service SHALL 仅允许已关注该主播频道的 Viewer 创建 Mic_Request。
4. WHERE 申请资格为 `fans`, THE Mic_Link_Service SHALL 仅允许该主播粉丝团成员的 Viewer 创建 Mic_Request。
5. WHERE 申请资格为 `fans_level`, THE Mic_Link_Service SHALL 仅允许粉丝团等级大于或等于 `minFanLevel` 的 Viewer 创建 Mic_Request。
6. IF 不满足申请资格的 Viewer 发起连麦申请, THEN THE Mic_Link_Service SHALL 拒绝该申请并返回错误原因 `mic_link_not_eligible`。
7. WHEN Room_Owner 选择资格档位为 `fans_level`, THE Mic_Link_Console SHALL 展示一个最低等级数字输入，取值范围与福袋一致（最小 1）。
8. IF 非 Room_Owner 的用户尝试修改申请资格, THEN THE Mic_Link_Service SHALL 拒绝该操作并返回错误原因 `forbidden`。

### Requirement 3: 观众发起连麦申请

**User Story:** 作为已登录且符合资格的观众，我想向主播发起语音连麦申请，以便有机会与主播实时语音互动。

#### Acceptance Criteria

1. WHEN 已登录且符合资格的 Viewer 在 Mic_Link_Panel 点击「申请连麦」, THE Mic_Link_Service SHALL 创建一条状态为 `pending` 的 Mic_Request，记录其 RoomID、用户标识与发起时间。
2. IF 未登录的 Viewer 点击「申请连麦」, THEN THE Mic_Link_Panel SHALL 触发登录弹窗且不创建 Mic_Request。
3. WHEN 一条 Mic_Request 创建成功, THE Mic_Link_Service SHALL 通过 `room:<RoomID>` 频道发布连麦事件，`event` 字段为 `requested`，并携带申请者昵称与头像供主播队列展示。
4. IF 同一 Viewer 在同一直播间已存在状态为 `pending` 或 `approved` 或 `on_air` 的 Mic_Request, THEN THE Mic_Link_Service SHALL 拒绝新的申请并返回错误原因 `mic_link_request_exists`。
5. WHILE 一条 Mic_Request 处于 `pending` 状态, THE Mic_Link_Panel SHALL 向该 Viewer 显示「申请待处理」状态并提供「取消申请」操作。
6. WHEN Viewer 在申请被处理前点击「取消申请」, THE Mic_Link_Service SHALL 将该 Mic_Request 状态置为 `cancelled` 并发布 `event=cancelled` 事件。
7. IF 连麦总开关为「已关闭」时 Viewer 发起申请, THEN THE Mic_Link_Service SHALL 拒绝该申请并返回错误原因 `mic_link_disabled`。

### Requirement 4: 主播审批连麦申请

**User Story:** 作为主播，我想在申请队列里看到待处理的连麦申请并逐个批准或拒绝，以便决定让谁上麦。

#### Acceptance Criteria

1. WHILE 直播间存在至少一条 `pending` 的 Mic_Request, THE Mic_Link_Console SHALL 向 Room_Owner 展示 Request_Queue，每项包含申请者昵称与头像。
2. THE Request_Queue SHALL 按 Mic_Request 的发起时间升序排列。
3. WHEN Room_Owner 对某条申请点击「批准」, THE Mic_Link_Service SHALL 将该 Mic_Request 状态置为 `approved` 并发布 `event=approved` 事件，事件中包含被批准的用户标识。
4. WHEN Room_Owner 对某条申请点击「拒绝」, THE Mic_Link_Service SHALL 将该 Mic_Request 状态置为 `rejected` 并发布 `event=rejected` 事件。
5. IF Room_Owner 批准一条申请时在麦嘉宾数量已达 Concurrency_Limit（3）, THEN THE Mic_Link_Service SHALL 拒绝该批准操作并返回错误原因 `mic_link_slot_full`。
6. IF 非 Room_Owner 的用户尝试审批 Mic_Request, THEN THE Mic_Link_Service SHALL 拒绝该操作并返回错误原因 `forbidden`。
7. IF Room_Owner 审批一条已不存在或已非 `pending` 状态的 Mic_Request, THEN THE Mic_Link_Service SHALL 拒绝该操作并返回错误原因 `mic_link_request_not_found`。

### Requirement 5: 嘉宾上麦与音频上行

**User Story:** 作为被批准的观众，我想在批准后开启麦克风上麦，以便我的声音能被主播和房间内所有观众听到。

#### Acceptance Criteria

1. WHEN 一条 Mic_Request 被置为 `approved`, THE Mic_Link_Service SHALL 将该被批准用户纳入 On_Air_Roster 并将其状态推进为 `on_air`。
2. WHEN 一名被批准的 Viewer 准备上麦, THE Mic_Link_Panel SHALL 请求浏览器麦克风授权。
3. IF 麦克风授权被拒绝或不可用, THEN THE Mic_Link_Panel SHALL 展示授权失败的本地化提示并取消该用户的上麦流程，且 Mic_Link_Service SHALL 将其移出 On_Air_Roster。
4. WHEN 嘉宾获得麦克风授权后上麦, THE SRS_RTC SHALL 通过 WHIP 接收该嘉宾的 audio-only（Opus）上行音频流。
5. WHEN 一名 Mic_Guest 成功上麦, THE Mic_Link_Service SHALL 发布 `event=on_air` 事件，事件携带最新的 On_Air_Roster。
6. IF 嘉宾在被批准后的连接超时（默认 15 秒）内未能建立 WHIP 音频上行, THEN THE Mic_Link_Service SHALL 将其移出 On_Air_Roster 并发布 `event=removed`（原因 `connect_timeout`）。
7. THE Mic_Link_Service SHALL 保证 On_Air_Roster 中在麦嘉宾数量不超过 Concurrency_Limit（3）。

### Requirement 6: 嘉宾自助控制（静音 / 下麦）

**User Story:** 作为连麦嘉宾，我想自己控制麦克风静音并能随时主动下麦，以便掌控自己的发言与退出时机。

#### Acceptance Criteria

1. WHILE 自己处于 `on_air` 状态, THE Mic_Link_Panel SHALL 向 Mic_Guest 提供「静音 / 取消静音」与「下麦」操作。
2. WHEN Mic_Guest 点击「静音」, THE Mic_Link_Panel SHALL 停止本地麦克风音频上行，且 Mic_Link_Service SHALL 发布 `event=guest_muted` 事件以更新各端的说话 / 静音指示。
3. WHEN Mic_Guest 点击「取消静音」, THE Mic_Link_Panel SHALL 恢复本地麦克风音频上行，且 Mic_Link_Service SHALL 发布 `event=guest_unmuted` 事件。
4. WHEN Mic_Guest 点击「下麦」, THE Mic_Link_Service SHALL 将其移出 On_Air_Roster、释放其媒体资源并发布 `event=left` 事件。
5. WHEN 一名 Mic_Guest 下麦或被移除, THE Mic_Link_Service SHALL 将当前在麦嘉宾数量减一，使后续审批可再次通过。

### Requirement 7: 主播管理在麦嘉宾

**User Story:** 作为主播，我想看到当前在麦的嘉宾并能随时把任意嘉宾下麦，以便掌控直播节奏与内容安全。

#### Acceptance Criteria

1. WHILE 存在至少一名 Mic_Guest 在麦, THE Mic_Link_Console SHALL 向 Room_Owner 展示 On_Air_Roster，每名嘉宾一行，含头像、昵称与静音 / 说话状态。
2. THE On_Air_Roster SHALL 最多展示 3 名在麦嘉宾，并展示当前占用数 / 上限（如「2 / 3」）。
3. WHEN Room_Owner 对某 Mic_Guest 点击「下麦」（移除）, THE Mic_Link_Service SHALL 结束该嘉宾的连麦、释放其媒体资源并发布 `event=removed` 事件（原因 `removed_by_owner`）。
4. IF 非 Room_Owner 的用户尝试移除某 Mic_Guest, THEN THE Mic_Link_Service SHALL 拒绝该操作并返回错误原因 `forbidden`。
5. WHEN 一名 Mic_Guest 被 Room_Owner 移除, THE Mic_Link_Panel SHALL 向被移除的该用户展示「已被主播下麦」的本地化提示并恢复到可再次申请的初始态（若仍符合资格且连麦开启）。

### Requirement 8: 向房间内所有观众呈现连麦音频

**User Story:** 作为房间内的普通观众，我想听到正在连麦的嘉宾并看到谁在麦上，以便完整地观看互动。

#### Acceptance Criteria

1. WHILE 存在至少一名 Mic_Guest 在麦, THE Mic_Stage_Page SHALL 通过 WHEP 订阅并播放所有 `on_air` 嘉宾的音频。
2. WHILE Mic_Stage_Page 作为 OBS 浏览器源被加入主播的 OBS, THE 直播输出 SHALL 通过现有 RTMP → SRS → HTTP-FLV / HLS 链路把嘉宾音频混入直播，使所有 Viewer 经现有播放链路即可听到嘉宾声音。
3. THE 普通 Viewer 的播放端 SHALL 无需任何 WebRTC 能力即可听到连麦音频。
4. WHILE 存在至少一名 Mic_Guest 在麦, THE On_Mic_Indicator SHALL 向房间内所有 Viewer 展示当前在麦嘉宾的头像、昵称与说话 / 静音状态。
5. WHEN On_Air_Roster 发生变化（新增 / 移除 / 静音态变化）, THE On_Mic_Indicator SHALL 在收到对应连麦实时事件后更新展示内容。

### Requirement 9: 连麦实时信令事件

**User Story:** 作为系统，我需要把连麦各阶段的状态变化实时广播给相关客户端，以便各端 UI 与房间状态保持一致。

#### Acceptance Criteria

1. THE Mic_Link_Service SHALL 通过 Redis `room:<RoomID>` 频道发布类型为 `mic_link` 的事件，其 `event` 取值集合为 `feature_enabled` / `feature_disabled` / `requested` / `approved` / `rejected` / `cancelled` / `on_air` / `left` / `removed` / `guest_muted` / `guest_unmuted`。
2. WHEN IM_Gateway 从 `room:<RoomID>` 收到一条 `mic_link` 事件, THE IM_Gateway SHALL 将该事件扇出给该房间所有已连接的 WebSocket 客户端。
3. WHEN 前端 `useRoomRealtime` 收到一条 `mic_link` 事件, THE 前端 SHALL 更新连麦相关本地状态或失效相关查询缓存，以刷新 Mic_Link_Panel、Mic_Link_Console 与 On_Mic_Indicator。
4. THE `mic_link` 事件 SHALL 携带毫秒级时间戳 `ts` 字段，与现有 chat / gift / bet / lucky_bag 事件格式保持一致。
5. WHERE 一条 `mic_link` 事件仅与特定用户相关（如对申请者的 `approved` / `rejected`）, THE 前端 SHALL 仅对匹配该用户标识的客户端更新其个人申请状态展示。

### Requirement 10: 向主播暴露 OBS 连麦舞台地址

**User Story:** 作为主播，我想拿到连麦舞台页地址并一键复制，以便把它作为 OBS 浏览器源加入，从而把嘉宾声音混进直播。

#### Acceptance Criteria

1. WHILE Room_Owner 处于 Mic_Link_Console, THE Mic_Link_Console SHALL 展示本直播间的 Mic_Stage_Page 地址，展示形态与既有 OBS server / Stream key / Playback URL 行（`PublisherLine`）一致。
2. THE Mic_Stage_Page 地址行 SHALL 提供「复制」操作，复用既有 `copyText` / clipboard 机制，并在复制成功 / 手动复制时给出本地化 toast 反馈。
3. THE Mic_Stage_Page 地址 SHALL 绑定到具体 RoomID，使该页仅订阅本直播间的在麦嘉宾音频。
4. THE Mic_Link_Console SHALL 在 Mic_Stage_Page 地址附近提供简短的本地化使用说明（提示主播将其加为 OBS 浏览器源）。
5. WHERE 仅 Room_Owner 可见, THE Mic_Stage_Page 地址 SHALL 不向普通 Viewer 展示。

### Requirement 11: 权限、鉴权与接口接入

**User Story:** 作为平台，我需要连麦写操作要求登录、且仅房主能管理本房间连麦，并使接口被网关正确路由，以便保证安全与可用。

#### Acceptance Criteria

1. THE 连麦相关写接口 SHALL 通过 `api-gateway` 路由注册，使用统一的 `/api/mic-link` 路径前缀。
2. IF 一个连麦写请求缺少有效的鉴权凭证, THEN THE api-gateway SHALL 拒绝该请求并返回未授权错误（HTTP 401）。
3. THE Mic_Link_Service SHALL 仅允许该 RoomID 的 Room_Owner 执行开关切换、资格设置、审批、移除等管理操作。
4. WHERE 某个连麦只读接口需要支持游客查看（如查询当前在麦名单与连麦开关态）, THE api-gateway SHALL 将该只读接口登记为公开路由（参照既有 `publicRoutes()` 约定）。
5. THE Mic_Link_Service SHALL 校验所有连麦写请求中由网关注入的 `X-User-Id` 与所声明角色一致。

### Requirement 12: 会话生命周期与边界处理

**User Story:** 作为用户，我想在直播结束、断线、重复申请等异常情况下连麦行为可预期，以便不出现残留会话或错误状态。

#### Acceptance Criteria

1. WHEN 一个直播间的直播结束（收到 `live_ended` / 直播状态变为 `ended`）, THE Mic_Link_Service SHALL 结束该房间所有在麦的 Mic_Guest、释放其媒体资源，并对每名嘉宾发布 `event=removed`（原因 `live_ended`）。
2. WHEN 一个直播间的直播结束, THE Mic_Link_Service SHALL 将该房间所有 `pending` 的 Mic_Request 置为 `cancelled`（原因 `live_ended`）。
3. WHEN 一名 Mic_Guest 的 WHIP 音频上行或 WebSocket 连接断开并超过宽限时间（默认 15 秒）未恢复, THE Mic_Link_Service SHALL 将其移出 On_Air_Roster 并发布 `event=removed`（原因 `disconnected`）。
4. IF 同一 Viewer 重复提交连麦申请（已有 `pending` / `approved` / `on_air`）, THEN THE Mic_Link_Service SHALL 拒绝并返回错误原因 `mic_link_request_exists`（见 Requirement 3.4）。
5. IF 一名 Viewer 已在另一个直播间处于 `on_air` 状态, THEN THE Mic_Link_Service SHALL 拒绝其在新直播间上麦并返回错误原因 `mic_link_already_on_air_elsewhere`。
6. WHEN On_Air_Roster 因任何原因变为空, THE Mic_Link_Service SHALL 保持连麦总开关状态不变（关闭功能须由 Room_Owner 显式操作）。

### Requirement 13: 连麦状态一致性与重连恢复

**User Story:** 作为用户，我想在刷新页面或网络抖动后看到正确的连麦状态，以便各端展示与真实会话一致。

#### Acceptance Criteria

1. THE Mic_Link_Service SHALL 提供一个只读接口，返回某 RoomID 的连麦开关态、当前 Eligibility / `minFanLevel`、On_Air_Roster（含各嘉宾静音态），以及调用者自身的 Mic_Request 状态。
2. WHEN 一个客户端进入直播间或 WebSocket 重连后, THE 前端 SHALL 通过该只读接口重建 Mic_Link_Panel / Mic_Link_Console 与 On_Mic_Indicator，而不依赖错过的实时事件。
3. WHILE 某 Viewer 自身存在 `pending` / `approved` / `on_air` 的 Mic_Request, THE Mic_Link_Panel SHALL 在该 Viewer 刷新或重连后恢复对应的申请 / 在麦状态展示。
4. THE 只读接口返回的 On_Air_Roster SHALL 与最近一次广播的 `mic_link` 事件结果最终一致。

### Requirement 14: 错误处理与反馈

**User Story:** 作为用户，我想在连麦操作失败时收到清晰的本地化原因，以便知道下一步怎么做。

#### Acceptance Criteria

1. WHEN 任一连麦写操作失败, THE Mic_Link_Service SHALL 返回稳定的机器可读错误原因码，集合至少包含：`mic_link_disabled` / `mic_link_not_eligible` / `mic_link_request_exists` / `mic_link_slot_full` / `mic_link_request_not_found` / `mic_link_already_on_air_elsewhere` / `connect_timeout` / `forbidden`。
2. WHEN 前端收到一个已知连麦错误原因码, THE Mic_Link_Panel / Mic_Link_Console SHALL 通过既有 toast（`sonner`）机制展示对应的本地化提示文案。
3. IF 前端收到一个未识别的错误原因码, THEN THE Mic_Link_Panel / Mic_Link_Console SHALL 展示一条通用的「操作失败」本地化提示。

### Requirement 15: 三语国际化

**User Story:** 作为多语言用户，我想用自己的语言看到连麦相关的全部文案，以便正常使用功能。

#### Acceptance Criteria

1. THE Mic_Link_Panel、Mic_Link_Console、On_Mic_Indicator 及所有连麦提示文案 SHALL 同时在 zh-CN、ja-JP、en-US 三个 locale 的 `pages.json` 中提供对应键值。
2. WHERE 某条文案包含数量（如在麦人数、占用 / 上限）, THE 对应 i18n 键 SHALL 使用 `_one` / `_other` 复数形式。
3. THE 连麦新增的 i18n 键 SHALL 通过既有的 locale parity 测试（三语键集合一致）。

## 非功能性需求 (Non-Functional Requirements)

> 以下为可度量的质量目标，作为 design.md 与验收的依据。

1. **延迟**：嘉宾语音从上麦到房间内观众听到，端到端目标 < 2 秒（嘉宾 WHIP 上行 + OBS 混流 + 现有 FLV 播放叠加延迟）；嘉宾本地静音 / 取消静音应在 1 秒内反映到上行。
2. **并发**：单直播间在麦嘉宾硬上限 3；服务端必须在并发审批 / 上麦竞态下严格不超过 3（见 Requirement 5.7、4.5）。
3. **状态一致性**：客户端进房或重连后，应在 2 秒内通过只读接口重建出正确的连麦状态（开关态、资格、在麦名单、静音态、自身申请态）。
4. **资源预算**：启用 audio-only SRS WebRTC 预计新增内存约 +30MB ~ +90MB，须在该无 swap 宿主机上稳定运行；不得引入视频上行。
5. **可观测性**：连麦关键事件（开关切换、申请、批准、拒绝、上麦、下麦、移除、断线）应可被记录以便审计与排障（参照现有 outbox 模式）。
6. **优雅降级**：嘉宾端浏览器若不支持 WebRTC / `getUserMedia`，连麦申请入口禁用，但该用户及所有其他观众的直播观看（HTTP-FLV）不受影响。
7. **安全**：所有管理操作仅限 Room_Owner；所有写操作需登录鉴权；Mic_Stage_Page 地址仅向房主展示。

## UI/UX 设计期望 (UI Design Expectations)

> 用户明确要求文档覆盖「UI 怎么设计」。以下为 UI/UX 设计约束与布局期望，作为 design.md 的输入。所有控件复用 `golive-web/src/features/live-room/` 既有面板模式与 `gl-*` 样式（`golive-web/src/styles/index.css`），下拉一律用自定义 `DropdownMenu`（非原生 `<select>`），复用 `Avatar` / `Dialog` / `DropdownMenu` 等组件，不做卡中卡嵌套，除既有小型强调按钮外不用渐变，不用超大按钮，仅用 `--gl-*` CSS 变量。新增样式统一以 `gl-mic-*` 前缀命名，图标使用 `lucide-react`（如 Mic / MicOff / Users / Check / X / Copy / Radio）。

### 通用与放置

- 连麦面板落位于直播间互动区，与 `LuckyBagPanel`、`BettingPanel`、`GiftPanel` 并列；观众视图（Mic_Link_Panel）挂在 `LiveRoomPage`，主播视图（Mic_Link_Console）挂在 `CreatorStudioPage` 的直播控制台，挂载方式参考 `LuckyBagPanel`。
- 通过 `ownsStream` 区分主播视图（管理）与观众视图（申请 / 在麦），与 `LuckyBagPanel` 的 `ownsStream` 用法一致。
- 普通观众视图**不得**出现任何主播管理控件或 Mic_Stage_Page 地址。

### 观众侧 UI（Mic_Link_Panel）

1. **「申请连麦」入口**：互动区内一个主操作按钮，采用与 `gl-bag-join` 同级的小型强调按钮风格（非超大按钮）。未登录点击触发登录弹窗（Requirement 3.2）。
2. **开关关闭态**：当主播未开启连麦时，入口禁用并显示「主播未开启连麦」本地化提示（Requirement 1.3）。
3. **资格不满足态**：当连麦已开启但当前 Viewer 不符合资格时，入口禁用并显示资格要求提示（如「仅限关注者」「仅限粉丝团 Lv.N+」），文案对齐福袋资格展示风格。
4. **申请状态机展示**（参考 `gl-bag-result is-joined` 风格的状态条）：
   - `pending`：显示「申请待处理」状态条 + 「取消申请」次级按钮。
   - `approved`（准备上麦）：显示麦克风授权请求与授权状态；授权失败显示重试 / 取消。
   - `on_air`（连麦中）：显示「连麦中」状态 + 本地「静音 / 取消静音」切换 + 「下麦」按钮。
   - `rejected` / `cancelled` / `removed`：以非阻断 toast 告知，并恢复到可再次申请的初始态。
5. **本地麦克风状态**：在 `on_air` 态展示当前麦克风开 / 关（Mic / MicOff 图标）与说话指示（轻量音浪或高亮）。

### 主播侧 UI（Mic_Link_Console）

1. **连麦总开关**：面板顶部一个开 / 关切换（参考既有开关控件样式），控制 Requirement 1。
2. **申请资格设置**：与福袋一致的自定义下拉（`gl-bag-select-*` / `gl-mic-select-*` 模式，基于 `DropdownMenu`），选项 `all` / `followers` / `fans` / `fans_level`；选 `fans_level` 时显示最低等级数字输入（取值与福袋一致，最小 1）。**禁止使用原生 `<select>`**。
3. **Request_Queue（申请队列）**：
   - 列表项含 `Avatar` + 昵称 + 「批准 / 拒绝」两个操作按钮。
   - 按发起时间升序；队列为空时显示空状态文案。
   - 当在麦数已达 3 时，「批准」按钮置灰并提示「已满 (3/3)」。
4. **On_Air_Roster（在麦名单）**：
   - 顶部显示占用 / 上限（如「2 / 3」）。
   - 每名在麦嘉宾一行：`Avatar` + 昵称 + 麦克风 / 说话状态图标 + 「下麦」（移除）按钮。
   - 名单为空时显示空状态文案。
5. **OBS 连麦舞台地址行**：复用 `PublisherLine` 形态（label + 只读 `code` + 复制按钮），展示 Mic_Stage_Page 地址，配本地化使用说明「将此地址添加为 OBS 浏览器源」。仅房主可见（Requirement 10）。

### 房间内在麦指示（On_Mic_Indicator）

1. **叠层指示**：在直播画面上叠加一个轻量指示（参考 `Player.tsx` 内 `gl-player-top` 叠层风格），展示「正在连麦」+ 在麦嘉宾头像（最多 3 个）+ 昵称，对所有观众可见。
2. **说话 / 静音态**：每个在麦头像旁展示说话音浪或静音图标（MicOff），随 `guest_muted` / `guest_unmuted` 实时事件更新。
3. 仅音频连麦不占用主画面视频位（无视频画面）。

### 对话框与提示 (Dialogs / Toasts)

1. **确认类操作**：主播「下麦（移除）」某嘉宾、关闭连麦总开关且有在麦嘉宾时，使用 `Dialog` 二次确认，文案本地化。
2. **轻量反馈**：申请被批准 / 拒绝、被主播下麦、操作失败等，统一用 `sonner` toast 本地化提示（参考 `LuckyBagPanel` 的 toast 模式）。

### 无障碍 (Accessibility)

- 所有交互按钮提供 `aria-label`；状态变化区使用 `role="status"` / `aria-live`（参考 `Player.tsx` 的 buffering / ending 提示）。
- 切换 / 下拉控件可键盘操作并有可见焦点态（复用 `gl-bag-select-trigger:focus-visible` 同类样式）。

### i18n 键期望（三语：zh-CN / ja-JP / en-US）

> 在每个 locale 的 `pages.json` 新增 `micLink.*` 文案块，键集合三语一致并通过 locale parity 测试。计数键用 `_one` / `_other`。以下为期望覆盖的键（命名最终以 design / 实现为准）：

- `micLink.title`、`micLink.requestButton`、`micLink.cancelRequest`、`micLink.leave`
- `micLink.muteSelf`、`micLink.unmuteSelf`
- `micLink.statusPending`、`micLink.statusApproved`、`micLink.statusOnAir`、`micLink.statusRejected`、`micLink.statusRemoved`
- `micLink.featureDisabledHint`（主播未开启）、`micLink.micPermissionDenied`（授权失败）
- 主播侧：`micLink.toggleLabel`、`micLink.eligibilityLabel`、`micLink.eligibility.all`、`micLink.eligibility.followers`、`micLink.eligibility.fans`、`micLink.eligibility.fansLevel`、`micLink.minFanLevelLabel`
- 队列 / 名单：`micLink.queueTitle`、`micLink.queueEmpty`、`micLink.approve`、`micLink.reject`、`micLink.rosterTitle`、`micLink.rosterEmpty`、`micLink.remove`
- 计数：`micLink.onAirCount_one`、`micLink.onAirCount_other`、`micLink.slotUsage`（如 `{{used}}/{{max}}`）
- OBS：`micLink.stageUrlLabel`、`micLink.stageUrlHint`、`micLink.stageUrlCopied`
- 指示：`micLink.indicatorOnAir`、`micLink.indicatorMuted`
- 错误：`micLink.error.disabled`、`micLink.error.notEligible`、`micLink.error.requestExists`、`micLink.error.slotFull`、`micLink.error.requestNotFound`、`micLink.error.alreadyOnAirElsewhere`、`micLink.error.connectTimeout`、`micLink.error.forbidden`、`micLink.error.generic`

## 范围之外 (Out of Scope)

- **视频 / 摄像头连麦**：本功能仅语音，明确排除视频，以控制宿主机内存开销。
- **观众端 WebRTC 播放**：普通观众继续走现有 HTTP-FLV / HLS，不引入观众端 WebRTC。
- **主播主流改造**：OBS → RTMP → SRS → FLV / HLS 主链路不变。
- **连麦历史 / 回放 / 数据分析**：本期不做面向用户的连麦历史展示（关键事件落库仅用于审计 / 排障）。
