# Requirements Document

Audience Voice Mic-Link

## Introduction

This feature adds audience voice mic-link to the GoLive streaming platform. Logged-in viewers in a live room can submit **audio-only** requests to join the streamer. The streamer reviews each request in a queue (approve / reject). Once approved, the guest microphone audio is published to the room and mixed into the streamer's output so every viewer can hear it. At most **3 guests** may be on mic simultaneously in a room. The streamer can enable or disable the feature, restrict eligibility, and remove on-air guests at any time. Guests can also hang up or leave voluntarily at any time.

This feature is **audio only, with no video or camera support**. This is a confirmed, final product decision, partly to keep host memory usage within an acceptable range.

This document covers user stories, EARS acceptance criteria, a glossary, detailed UI design expectations, and i18n requirements for three languages. It focuses on WHAT to build and WHY; implementation details (HOW) are deferred to the design phase. The user explicitly requested comprehensive documentation, including UI design, so a dedicated UI section is included.

### Confirmed Product Decisions

1. **Audio only**: no video or camera; only an Opus audio uplink. This decision is final.
2. **Concurrency limit = 3**: at most 3 guests may be on mic simultaneously in one room.
3. **Streamer master toggle**: the streamer must enable mic-link before viewers can request access and can disable it at any time.
4. **Reuse lucky bag eligibility**: `all` (all logged-in viewers) / `followers` / `fans` (fan club members) / `fans_level` (fan club members with level ≥ `minFanLevel`). Logged-out viewers cannot join mic-link.
5. **Approval and leave controls for both sides**: the streamer reviews each request and can remove any on-air guest; guests may also leave voluntarily.

### Confirmed Technical Direction

> The user has accepted the following deployable approach as a constraint for the design phase. All acceptance criteria must be achievable with this architecture. Detailed implementation decisions belong in design.md.

- **Keep the main stream unchanged**: OBS → RTMP → SRS (ossrs/srs:5) → HTTP-FLV / HLS. Normal viewer playback remains unchanged and **does not require WebRTC**.
- **Guest uplink uses WebRTC (WHIP)**: guest microphone audio (audio-only / Opus) is published to SRS `rtc_server` through WHIP.
- **Mic Stage Page**: a web page subscribes to and plays all approved guests through WHEP. The streamer adds it as an **OBS Browser Source**, allowing OBS to mix guest audio into the outgoing RTMP stream. **All viewers hear guests through the existing playback path**.
- **Enable SRS `rtc_server`**: open UDP 8000 and configure the public candidate IP (`154.36.185.85`). coturn / TURN is an **optional fallback**, not a prerequisite; the server already has a public IP and STUN-only is expected to suffice in most cases.
- **Memory budget**: audio-only WebRTC is estimated to add +30MB to +90MB, acceptable on this resource-constrained host without swap. Video is excluded specifically to control resource usage.

### Architecture Fit (Background, Not Requirements)

- **Reuse the real-time signaling path**: request / approval / rejection / join / leave / removal / roster events reuse `room:<RoomID>` Redis channels → `im-gateway` WebSocket fanout → frontend `useRoomRealtime.ts`, as chat, gifts, lucky bags, and betting already do.
- **Register the new API prefix**: mic-link HTTP prefixes must be registered in `golive-backend/app/api-gateway/internal/router/router.go`; otherwise requests return 404.
- **Eligibility already exists**: lucky bags implement `all` / `followers` / `fans` / `fans_level(minFanLevel)`. Follow relationships live in Redis (`user:<id>:follows`); fan club levels are checked through `FanBadgeLevelFor`. Mic-link reuses these semantics.
- **Frontend conventions**: reuse panels in `golive-web/src/features/live-room/` and `gl-*` styles from `golive-web/src/styles/index.css`. Use custom `DropdownMenu` controls instead of native `<select>` elements, and reuse `components/ui` primitives such as `Avatar`, `Dialog`, and `DropdownMenu`. Avoid nested cards, gradients except for existing small accent buttons, and oversized buttons. Mount the mic-link panel in both `LiveRoomPage` (viewers) and `CreatorStudioPage` (streamer), following `LuckyBagPanel`.
- **Three-language i18n is mandatory**: all user-facing text must be available in zh-CN / ja-JP / en-US (`golive-web/src/i18n/locales/*/pages.json`). Count keys use `_one` / `_other` and must pass the existing locale parity tests (`pnpm test`).
- **Authentication**: viewers may browse without signing in; writes such as mic-link requests require login (the gateway injects `X-User-Id`).

## Glossary

- **Mic_Link_Service**: backend domain service managing mic-link sessions, request state machines, eligibility and permissions, the 3-guest concurrency limit, and Redis event publication. It may be a new microservice or part of the existing `room-service`, to be decided during design.
- **IM_Gateway**: existing `im-gateway` service that fans out `room:<RoomID>` channel events to room clients over WebSocket.
- **SRS_RTC**: SRS `rtc_server`, carrying guest WHIP uplink and Mic Stage Page WHEP downlink audio (audio-only / Opus).
- **Mic_Link_Session**: an active mic-link session in a live room, consisting of the owner and 0–3 on-air guests.
- **Mic_Request**: a viewer request with one of these states: `pending` / `approved` / `rejected` / `cancelled` / `on_air` / `removed`.
- **Mic_Guest**: an approved viewer who is on mic and whose audio is being broadcast.
- **Request_Queue**: streamer-visible list of `pending` mic-link requests, ordered by creation time ascending.
- **On_Air_Roster**: current on-air guests (at most 3), including each guest's nickname, avatar, and mute/speaking status.
- **Room_Owner**: the streamer owning the room, identified by `room:owner:<RoomID>`, and the only user allowed to manage mic-link.
- **Viewer**: a person watching the stream, possibly without signing in.
- **Mic_Link_Console**: streamer management panel in `CreatorStudioPage`, containing the master toggle, eligibility settings, request queue, on-air roster, and OBS stage URL.
- **Mic_Link_Panel**: viewer request/on-air panel in `LiveRoomPage`.
- **On_Mic_Indicator**: a component showing all viewers who is currently on mic.
- **Mic_Stage_Page**: web page that subscribes to and plays all on-air guests through WHEP; the streamer adds it as an OBS browser source to mix guests into the outgoing live stream.
- **Eligibility**: request eligibility tier, one of `all` / `followers` / `fans` / `fans_level`, matching lucky bags.
- **Concurrency_Limit**: maximum simultaneous on-air guests per room, fixed at **3**.
- **RoomID**: live-room identifier.

## Requirements

### Requirement 1: Streamer Enables or Disables Mic-Link

**User Story:** As a streamer, I want to enable or disable mic-link for a broadcast so I can decide whether to accept audience voice participation.

#### Acceptance Criteria

1. WHEN Room_Owner enables the master toggle in Mic_Link_Console, THE Mic_Link_Service SHALL mark mic-link as enabled for that RoomID and accept new Mic_Request entries.
2. WHEN Room_Owner disables the master toggle, THE Mic_Link_Service SHALL mark mic-link as disabled for that RoomID and reject all new Mic_Request entries with reason `mic_link_disabled`.
3. WHILE mic-link is disabled, THE Mic_Link_Panel SHALL hide or disable the request action and show Viewer a localized "The streamer has not enabled mic-link" message.
4. WHEN Room_Owner disables mic-link while Mic_Guest users are on air, THE Mic_Link_Service SHALL end all guest connections, release their media resources, and publish `event=removed` with reason `feature_disabled` for each guest.
5. IF a user other than Room_Owner attempts to change the master toggle, THEN THE Mic_Link_Service SHALL reject the operation with reason `forbidden`.
6. WHEN the master toggle changes, THE Mic_Link_Service SHALL publish a mic-link event (`event=feature_enabled` or `event=feature_disabled`) on `room:<RoomID>` to synchronize client UIs.

### Requirement 2: Streamer Configures Request Eligibility

**User Story:** As a streamer, I want to restrict who can request mic-link so I can manage participants and maintain order in the room.

#### Acceptance Criteria

1. THE Mic_Link_Service SHALL support `all` (all logged-in viewers) / `followers` / `fans` (fan club members) / `fans_level` (fan club members with level ≥ `minFanLevel`), using the same semantics as `LuckyBagEligibility`.
2. WHEN Room_Owner configures eligibility, THE Mic_Link_Service SHALL store the current Eligibility for that RoomID and `minFanLevel` when the tier is `fans_level`.
3. WHERE eligibility is `followers`, THE Mic_Link_Service SHALL allow only Viewer users who follow the streamer's channel to create a Mic_Request.
4. WHERE eligibility is `fans`, THE Mic_Link_Service SHALL allow only Viewer users who belong to the streamer's fan club to create a Mic_Request.
5. WHERE eligibility is `fans_level`, THE Mic_Link_Service SHALL allow only Viewer users whose fan club level is at least `minFanLevel` to create a Mic_Request.
6. IF an ineligible Viewer submits a mic-link request, THEN THE Mic_Link_Service SHALL reject it with reason `mic_link_not_eligible`.
7. WHEN Room_Owner selects `fans_level`, THE Mic_Link_Console SHALL display a numeric minimum-level input using the same range as lucky bags (minimum 1).
8. IF a user other than Room_Owner attempts to change eligibility, THEN THE Mic_Link_Service SHALL reject the operation with reason `forbidden`.

### Requirement 3: Viewer Requests Mic-Link

**User Story:** As a logged-in, eligible viewer, I want to request voice participation so I can interact with the streamer in real time.

#### Acceptance Criteria

1. WHEN a logged-in, eligible Viewer clicks "Request mic-link" in Mic_Link_Panel, THE Mic_Link_Service SHALL create a `pending` Mic_Request recording RoomID, user identifier, and creation time.
2. IF a logged-out Viewer clicks "Request mic-link", THEN THE Mic_Link_Panel SHALL open the login dialog without creating a Mic_Request.
3. WHEN a Mic_Request is created successfully, THE Mic_Link_Service SHALL publish a mic-link event on `room:<RoomID>` with `event=requested` and the requester's nickname and avatar for the streamer queue.
4. IF the same Viewer already has a `pending`, `approved`, or `on_air` Mic_Request in the same room, THEN THE Mic_Link_Service SHALL reject the new request with reason `mic_link_request_exists`.
5. WHILE a Mic_Request is `pending`, THE Mic_Link_Panel SHALL show that Viewer a pending status and a "Cancel request" action.
6. WHEN Viewer clicks "Cancel request" before it is processed, THE Mic_Link_Service SHALL set the Mic_Request to `cancelled` and publish `event=cancelled`.
7. IF Viewer submits a request while mic-link is disabled, THEN THE Mic_Link_Service SHALL reject it with reason `mic_link_disabled`.

### Requirement 4: Streamer Reviews Requests

**User Story:** As a streamer, I want to see pending mic-link requests and approve or reject them individually so I can decide who joins the microphone.

#### Acceptance Criteria

1. WHILE the room has at least one `pending` Mic_Request, THE Mic_Link_Console SHALL display Request_Queue to Room_Owner, including each requester's nickname and avatar.
2. THE Request_Queue SHALL sort Mic_Request entries by creation time ascending.
3. WHEN Room_Owner clicks "Approve" on a request, THE Mic_Link_Service SHALL set the Mic_Request to `approved` and publish `event=approved` with the approved user's identifier.
4. WHEN Room_Owner clicks "Reject" on a request, THE Mic_Link_Service SHALL set the Mic_Request to `rejected` and publish `event=rejected`.
5. IF the on-air guest count has reached Concurrency_Limit (3) when Room_Owner approves a request, THEN THE Mic_Link_Service SHALL reject the approval with reason `mic_link_slot_full`.
6. IF a user other than Room_Owner attempts to review a Mic_Request, THEN THE Mic_Link_Service SHALL reject the operation with reason `forbidden`.
7. IF Room_Owner reviews a Mic_Request that no longer exists or is no longer `pending`, THEN THE Mic_Link_Service SHALL reject the operation with reason `mic_link_request_not_found`.

### Requirement 5: Guest Joins and Publishes Audio

**User Story:** As an approved viewer, I want to enable my microphone and join so the streamer and all room viewers can hear me.

#### Acceptance Criteria

1. WHEN a Mic_Request becomes `approved`, THE Mic_Link_Service SHALL add the approved user to On_Air_Roster and advance the state to `on_air`.
2. WHEN an approved Viewer prepares to join, THE Mic_Link_Panel SHALL request browser microphone permission.
3. IF microphone permission is denied or unavailable, THEN THE Mic_Link_Panel SHALL display a localized permission-failure message and cancel the joining flow, and THE Mic_Link_Service SHALL remove the user from On_Air_Roster.
4. WHEN a guest joins after granting microphone permission, THE SRS_RTC SHALL receive the guest's audio-only (Opus) uplink through WHIP.
5. WHEN a Mic_Guest successfully joins, THE Mic_Link_Service SHALL publish `event=on_air` with the latest On_Air_Roster.
6. IF a guest fails to establish a WHIP audio uplink within the connection timeout after approval (default 15 seconds), THEN THE Mic_Link_Service SHALL remove the guest from On_Air_Roster and publish `event=removed` with reason `connect_timeout`.
7. THE Mic_Link_Service SHALL ensure the number of on-air guests in On_Air_Roster never exceeds Concurrency_Limit (3).

### Requirement 6: Guest Controls (Mute / Leave)

**User Story:** As a mic-link guest, I want to mute my microphone and leave at any time so I control when I speak and exit.

#### Acceptance Criteria

1. WHILE the guest is `on_air`, THE Mic_Link_Panel SHALL provide Mic_Guest with mute/unmute and leave actions.
2. WHEN Mic_Guest clicks "Mute", THE Mic_Link_Panel SHALL stop the local microphone audio uplink, and THE Mic_Link_Service SHALL publish `event=guest_muted` to update speaking/mute indicators across clients.
3. WHEN Mic_Guest clicks "Unmute", THE Mic_Link_Panel SHALL resume the local microphone audio uplink, and THE Mic_Link_Service SHALL publish `event=guest_unmuted`.
4. WHEN Mic_Guest clicks "Leave", THE Mic_Link_Service SHALL remove the guest from On_Air_Roster, release media resources, and publish `event=left`.
5. WHEN a Mic_Guest leaves or is removed, THE Mic_Link_Service SHALL decrement the on-air guest count so subsequent approvals can succeed.

### Requirement 7: Streamer Manages On-Air Guests

**User Story:** As a streamer, I want to see current guests and remove any guest at any time so I can manage the broadcast flow and content safety.

#### Acceptance Criteria

1. WHILE at least one Mic_Guest is on air, THE Mic_Link_Console SHALL display On_Air_Roster to Room_Owner, with one row per guest containing avatar, nickname, and mute/speaking status.
2. THE On_Air_Roster SHALL show at most 3 on-air guests and the current occupancy / limit, such as "2 / 3".
3. WHEN Room_Owner clicks "Remove" for a Mic_Guest, THE Mic_Link_Service SHALL end that guest's connection, release media resources, and publish `event=removed` with reason `removed_by_owner`.
4. IF a user other than Room_Owner attempts to remove a Mic_Guest, THEN THE Mic_Link_Service SHALL reject the operation with reason `forbidden`.
5. WHEN Room_Owner removes a Mic_Guest, THE Mic_Link_Panel SHALL show that user a localized "The streamer removed you from mic-link" message and return to the initial request state if the feature remains enabled and the user is still eligible.

### Requirement 8: Deliver Guest Audio to All Room Viewers

**User Story:** As a normal viewer, I want to hear participating guests and see who is on mic so I can follow the entire interaction.

#### Acceptance Criteria

1. WHILE at least one Mic_Guest is on air, THE Mic_Stage_Page SHALL subscribe to and play all `on_air` guest audio through WHEP.
2. WHILE Mic_Stage_Page is added to the streamer's OBS as a browser source, THE broadcast output SHALL mix guest audio into the live stream through the existing RTMP → SRS → HTTP-FLV / HLS path, allowing every Viewer to hear guests through existing playback.
3. THE normal Viewer playback client SHALL play mic-link audio without requiring WebRTC capabilities.
4. WHILE at least one Mic_Guest is on air, THE On_Mic_Indicator SHALL show all Viewer users the current guests' avatars, nicknames, and speaking/mute status.
5. WHEN On_Air_Roster changes through additions, removals, or mute-state changes, THE On_Mic_Indicator SHALL update after receiving the corresponding real-time mic-link event.

### Requirement 9: Real-Time Mic-Link Signaling

**User Story:** As the system, I need to broadcast mic-link state changes to relevant clients in real time so their UIs remain consistent with room state.

#### Acceptance Criteria

1. THE Mic_Link_Service SHALL publish `mic_link` events through Redis `room:<RoomID>`, with `event` values from `feature_enabled` / `feature_disabled` / `requested` / `approved` / `rejected` / `cancelled` / `on_air` / `left` / `removed` / `guest_muted` / `guest_unmuted`.
2. WHEN IM_Gateway receives a `mic_link` event from `room:<RoomID>`, THE IM_Gateway SHALL fan it out to all connected WebSocket clients in that room.
3. WHEN frontend `useRoomRealtime` receives a `mic_link` event, THE frontend SHALL update local mic-link state or invalidate relevant query caches to refresh Mic_Link_Panel, Mic_Link_Console, and On_Mic_Indicator.
4. THE `mic_link` event SHALL include a millisecond timestamp `ts`, consistent with existing chat / gift / bet / lucky_bag events.
5. WHERE a `mic_link` event concerns a specific user, such as `approved` / `rejected` for a requester, THE frontend SHALL update the personal request-status display only on clients matching that user identifier.

### Requirement 10: Provide the Streamer with an OBS Mic Stage URL

**User Story:** As a streamer, I want a Mic Stage Page URL with one-click copying so I can add it as an OBS browser source and mix guest audio into my broadcast.

#### Acceptance Criteria

1. WHILE Room_Owner is in Mic_Link_Console, THE Mic_Link_Console SHALL display the room's Mic_Stage_Page URL using the same format as existing OBS server / Stream key / Playback URL rows (`PublisherLine`).
2. THE Mic_Stage_Page URL row SHALL provide a copy action using the existing `copyText` / clipboard mechanism, with localized toast feedback for successful copying or manual-copy instructions.
3. THE Mic_Stage_Page URL SHALL be bound to a specific RoomID so it subscribes only to that room's on-air guest audio.
4. THE Mic_Link_Console SHALL provide brief localized instructions near the Mic_Stage_Page URL telling the streamer to add it as an OBS browser source.
5. WHERE the URL is restricted to Room_Owner, THE Mic_Stage_Page URL SHALL not be displayed to ordinary Viewer users.

### Requirement 11: Permissions, Authentication, and Routing

**User Story:** As the platform, I need mic-link writes to require login, room management to be owner-only, and gateway routing to work correctly so the feature is secure and available.

#### Acceptance Criteria

1. THE mic-link write endpoints SHALL be registered with `api-gateway` under the shared `/api/mic-link` prefix.
2. IF a mic-link write request lacks valid credentials, THEN THE api-gateway SHALL reject it with an unauthorized error (HTTP 401).
3. THE Mic_Link_Service SHALL allow only the Room_Owner for that RoomID to perform management operations such as toggling, configuring eligibility, reviewing requests, and removing guests.
4. WHERE a read-only mic-link endpoint must support guests, such as current roster and enabled-state queries, THE api-gateway SHALL register it as a public route following the existing `publicRoutes()` convention.
5. THE Mic_Link_Service SHALL verify that the gateway-injected `X-User-Id` in every mic-link write request matches the claimed role.

### Requirement 12: Session Lifecycle and Edge Cases

**User Story:** As a user, I want predictable behavior when streams end, connections drop, or requests are duplicated so sessions and states do not become stale or incorrect.

#### Acceptance Criteria

1. WHEN a broadcast ends (`live_ended` is received or stream status becomes `ended`), THE Mic_Link_Service SHALL end all on-air Mic_Guest connections in the room, release their media resources, and publish `event=removed` with reason `live_ended` for each guest.
2. WHEN a broadcast ends, THE Mic_Link_Service SHALL set all `pending` Mic_Request entries in the room to `cancelled` with reason `live_ended`.
3. WHEN a Mic_Guest WHIP uplink or WebSocket disconnects and does not recover within the grace period (default 15 seconds), THE Mic_Link_Service SHALL remove the guest from On_Air_Roster and publish `event=removed` with reason `disconnected`.
4. IF the same Viewer submits a duplicate request while one is already `pending` / `approved` / `on_air`, THEN THE Mic_Link_Service SHALL reject it with reason `mic_link_request_exists` (see Requirement 3.4).
5. IF a Viewer is already `on_air` in another room, THEN THE Mic_Link_Service SHALL reject joining in the new room with reason `mic_link_already_on_air_elsewhere`.
6. WHEN On_Air_Roster becomes empty for any reason, THE Mic_Link_Service SHALL keep the master toggle unchanged; disabling the feature requires an explicit Room_Owner action.

### Requirement 13: State Consistency and Reconnection Recovery

**User Story:** As a user, I want to see the correct mic-link state after refreshing or a network interruption so every client reflects the actual session.

#### Acceptance Criteria

1. THE Mic_Link_Service SHALL provide a read-only endpoint returning a RoomID's enabled state, current Eligibility / `minFanLevel`, On_Air_Roster with guest mute states, and the caller's own Mic_Request state.
2. WHEN a client enters a room or reconnects its WebSocket, THE frontend SHALL reconstruct Mic_Link_Panel / Mic_Link_Console and On_Mic_Indicator from that endpoint, without relying on missed real-time events.
3. WHILE a Viewer has a `pending` / `approved` / `on_air` Mic_Request, THE Mic_Link_Panel SHALL restore the corresponding request/on-air display after refresh or reconnection.
4. THE On_Air_Roster returned by the read-only endpoint SHALL be eventually consistent with the most recently broadcast `mic_link` event.

### Requirement 14: Error Handling and Feedback

**User Story:** As a user, I want clear localized reasons when mic-link operations fail so I know what to do next.

#### Acceptance Criteria

1. WHEN any mic-link write fails, THE Mic_Link_Service SHALL return a stable machine-readable reason code, including at least `mic_link_disabled` / `mic_link_not_eligible` / `mic_link_request_exists` / `mic_link_slot_full` / `mic_link_request_not_found` / `mic_link_already_on_air_elsewhere` / `connect_timeout` / `forbidden`.
2. WHEN the frontend receives a known mic-link reason code, THE Mic_Link_Panel / Mic_Link_Console SHALL display the corresponding localized message through the existing `sonner` toast mechanism.
3. IF the frontend receives an unknown reason code, THEN THE Mic_Link_Panel / Mic_Link_Console SHALL display a generic localized "Operation failed" message.

### Requirement 15: Internationalization in Three Languages

**User Story:** As a multilingual user, I want all mic-link text in my language so I can use the feature normally.

#### Acceptance Criteria

1. THE Mic_Link_Panel, Mic_Link_Console, On_Mic_Indicator, and all mic-link messages SHALL have corresponding keys and values in `pages.json` for zh-CN, ja-JP, and en-US.
2. WHERE text includes counts, such as on-air guests or occupancy/limit, THE corresponding i18n keys SHALL use `_one` / `_other` plural forms.
3. THE new mic-link i18n keys SHALL pass the existing locale parity tests, with identical key sets across the three languages.

## Non-Functional Requirements

> These measurable quality targets guide design.md and acceptance.

1. **Latency**: target end-to-end guest audio latency below 2 seconds, including guest WHIP uplink, OBS mixing, and existing FLV playback. Local mute/unmute should affect the uplink within 1 second.
2. **Concurrency**: a hard limit of 3 on-air guests per room; the server must enforce it under concurrent approval/join races (Requirements 5.7 and 4.5).
3. **State consistency**: within 2 seconds of entering or reconnecting, clients should reconstruct the correct enabled state, eligibility, roster, mute states, and personal request state from the read-only endpoint.
4. **Resource budget**: audio-only SRS WebRTC is expected to add +30MB to +90MB and must run stably on the host without swap. Video uplinks must not be introduced.
5. **Observability**: key events (toggle changes, requests, approvals, rejections, joins, leaves, removals, disconnections) should be recordable for auditing and troubleshooting, following the existing outbox pattern.
6. **Graceful degradation**: disable requests in guest browsers without WebRTC / `getUserMedia`, while preserving HTTP-FLV playback for that user and all other viewers.
7. **Security**: management is restricted to Room_Owner, all writes require authentication, and the Mic_Stage_Page URL is shown only to the owner.

## UI/UX Design Expectations

> The user explicitly requested UI design coverage. The following constraints and layout expectations feed into design.md. Reuse existing panels in `golive-web/src/features/live-room/` and `gl-*` styles in `golive-web/src/styles/index.css`. Use custom `DropdownMenu` controls, never native `<select>`, and reuse `Avatar`, `Dialog`, and `DropdownMenu`. Avoid nested cards, gradients except for existing small accent buttons, and oversized buttons. Use only `--gl-*` CSS variables. Prefix new styles with `gl-mic-*` and use `lucide-react` icons such as Mic / MicOff / Users / Check / X / Copy / Radio.

### General Layout and Placement

- Place mic-link in the room interaction area alongside `LuckyBagPanel`, `BettingPanel`, and `GiftPanel`. Mount Mic_Link_Panel in `LiveRoomPage` and Mic_Link_Console in the `CreatorStudioPage` live console, following the mounting pattern of `LuckyBagPanel`.
- Use `ownsStream` to distinguish streamer management from viewer request/on-air views, matching `LuckyBagPanel`.
- The normal viewer view **must not** contain streamer management controls or the Mic_Stage_Page URL.

### Viewer UI (Mic_Link_Panel)

1. **Request action**: a primary action in the interaction area, styled as a small accent button comparable to `gl-bag-join`, not an oversized button. Logged-out users open the login dialog (Requirement 3.2).
2. **Disabled feature**: disable the action and show a localized "The streamer has not enabled mic-link" hint (Requirement 1.3).
3. **Ineligible viewer**: when enabled but the current Viewer is ineligible, disable the action and show requirements such as "Followers only" or "Fan club Lv.N+ only", matching lucky bag eligibility styling.
4. **Request state display**, using a status bar similar to `gl-bag-result is-joined`:
   - `pending`: pending status bar + secondary "Cancel request" button.
   - `approved` (preparing to join): microphone permission request and status; provide retry/cancel after permission failure.
   - `on_air`: connected status + local mute/unmute toggle + leave button.
   - `rejected` / `cancelled` / `removed`: nonblocking toast, then return to the initial request state.
5. **Local microphone status**: while `on_air`, show microphone on/off (Mic / MicOff icons) and a speaking indicator such as a subtle waveform or highlight.

### Streamer UI (Mic_Link_Console)

1. **Master toggle**: an on/off control at the top of the panel, following existing toggle styles, for Requirement 1.
2. **Eligibility settings**: a custom dropdown matching lucky bags (`gl-bag-select-*` / `gl-mic-select-*`, based on `DropdownMenu`), with `all` / `followers` / `fans` / `fans_level`. Selecting `fans_level` reveals a minimum-level numeric input with the same range as lucky bags, minimum 1. **Native `<select>` is prohibited**.
3. **Request_Queue**:
   - Each row contains `Avatar`, nickname, and approve/reject buttons.
   - Sort by creation time ascending; show empty-state text when the queue is empty.
   - At 3 on-air guests, disable "Approve" and show "Full (3/3)".
4. **On_Air_Roster**:
   - Show occupancy / limit at the top, such as "2 / 3".
   - One row per guest: `Avatar`, nickname, microphone/speaking icon, and remove button.
   - Show empty-state text when no guests are on air.
5. **OBS Mic Stage URL row**: reuse `PublisherLine` (label + read-only `code` + copy button), display the Mic_Stage_Page URL, and include localized "Add this URL as an OBS browser source" instructions. Visible only to the owner (Requirement 10).

### Room On-Mic Indicator (On_Mic_Indicator)

1. **Overlay**: a lightweight overlay on the live video, following `gl-player-top` in `Player.tsx`, with an active mic-link label, up to 3 guest avatars, and nicknames; visible to all viewers.
2. **Speaking/mute state**: show a waveform or MicOff icon beside each avatar, updated through `guest_muted` / `guest_unmuted` events.
3. Audio-only mic-link does not occupy a video tile in the main picture.

### Dialogs and Toasts

1. **Confirmations**: use a localized `Dialog` to confirm streamer guest removal and disabling the feature while guests are on air.
2. **Lightweight feedback**: use localized `sonner` toasts for approval/rejection, removal by the streamer, and failures, following `LuckyBagPanel`.

### Accessibility

- Provide `aria-label` for interactive buttons and use `role="status"` / `aria-live` for status changes, following buffering/ending indicators in `Player.tsx`.
- Toggles and dropdowns must support keyboard operation and visible focus states, reusing styles like `gl-bag-select-trigger:focus-visible`.

### Expected i18n Keys (zh-CN / ja-JP / en-US)

> Add a `micLink.*` block to each locale's `pages.json`, with matching key sets and passing locale parity tests. Use `_one` / `_other` for counts. Expected coverage follows; final names are determined in design/implementation:

- `micLink.title`, `micLink.requestButton`, `micLink.cancelRequest`, `micLink.leave`
- `micLink.muteSelf`, `micLink.unmuteSelf`
- `micLink.statusPending`, `micLink.statusApproved`, `micLink.statusOnAir`, `micLink.statusRejected`, `micLink.statusRemoved`
- `micLink.featureDisabledHint` (streamer has not enabled mic-link), `micLink.micPermissionDenied` (permission denied)
- Streamer controls: `micLink.toggleLabel`, `micLink.eligibilityLabel`, `micLink.eligibility.all`, `micLink.eligibility.followers`, `micLink.eligibility.fans`, `micLink.eligibility.fansLevel`, `micLink.minFanLevelLabel`
- Queue/roster: `micLink.queueTitle`, `micLink.queueEmpty`, `micLink.approve`, `micLink.reject`, `micLink.rosterTitle`, `micLink.rosterEmpty`, `micLink.remove`
- Counts: `micLink.onAirCount_one`, `micLink.onAirCount_other`, `micLink.slotUsage` (for example `{{used}}/{{max}}`)
- OBS: `micLink.stageUrlLabel`, `micLink.stageUrlHint`, `micLink.stageUrlCopied`
- Indicators: `micLink.indicatorOnAir`, `micLink.indicatorMuted`
- Errors: `micLink.error.disabled`, `micLink.error.notEligible`, `micLink.error.requestExists`, `micLink.error.slotFull`, `micLink.error.requestNotFound`, `micLink.error.alreadyOnAirElsewhere`, `micLink.error.connectTimeout`, `micLink.error.forbidden`, `micLink.error.generic`

## Out of Scope

- **Video/camera participation**: explicitly excluded from this audio-only feature to control host memory usage.
- **Viewer WebRTC playback**: normal viewers continue using existing HTTP-FLV / HLS; no viewer-side WebRTC is introduced.
- **Changes to the main broadcast path**: OBS → RTMP → SRS → FLV / HLS remains unchanged.
- **Mic-link history/replay/analytics**: no user-facing mic-link history in this iteration; persisted key events are only for auditing/troubleshooting.
