# Design Document: Audience Voice Mic-Link

## Overview

This design implements audience voice mic-link: logged-in viewers request to join the streamer using audio only. After approval, guest microphone audio is published to SRS over WebRTC. The streamer adds a Mic Stage browser source in OBS to mix guest audio into the live stream, so all viewers hear it through existing FLV/HLS playback. At most 3 guests may be on mic simultaneously.

The implementation reuses the existing architecture, with **no new microservices or database tables**:
- Mic-link domain logic lives in **gift-service**, which already hosts betting and lucky bags and provides `RoomOwner` / `RoomChannelID` / `FanBadgeLevelFor` eligibility checks and `room:<id>` Redis fanout.
- Mic-link session state is ephemeral and stored in **Redis** (one JSON document per room + a short-lived lock), with a 6-hour TTL.
- Signaling reuses the existing `room:<id>` → im-gateway → `useRoomRealtime` real-time channel, adding the `mic_link` event type.
- The media plane enables **SRS `rtc_server`** (WHIP uplink / WHEP downlink, audio-only Opus).

## Architecture

### Media flow

```
Guest browser ──WHIP(audio)──> SRS rtc_server ──WHEP(audio)──> Mic Stage page (OBS browser source)
                                                             │
Host OBS microphone ─────────────────────────────────────────┴── Mix ──RTMP──> SRS ──FLV/HLS──> All viewers
```

- Guest stream name: `miclink-<roomId>-<userId>`. On approval gift-service stores a random publish token in Redis `miclink:token:<stream>` (10-minute TTL, re-issued to the on-air guest through `/mic-link/latest` as `myPublishToken`, deleted on leave/remove/disable). The guest adds it to the WHIP URL as `key=<token>`; the room-service SRS `on_publish` hook rejects `miclink-` streams whose `key` does not match, without room accounting.
- Signaling (SDP offer/answer) uses HTTPS: nginx adds `location ^~ /rtc/` to proxy to `srs:1985`.
- Media uses UDP 8000 (the SRS container adds `8000:8000/udp`, with `CANDIDATE` set to the public IP `154.36.185.85`).

### Signaling flow

gift-service publishes state changes such as enable/disable, requests, approvals, joining/leaving, and muting to Redis `room:<id>`. im-gateway fans them out over WebSocket. When frontend `useRoomRealtime` receives a `mic_link` event, it invalidates `['mic-link', roomId]` to trigger a refetch. The frontend also polls every 3 seconds as a fallback for eventual consistency after refreshes or disconnections.

## Components and interfaces

### Backend (gift-service)

**Redis state document** `miclink:<roomId>`:
```
{ roomId, ownerId, enabled, eligibility, minFanLevel, requests[], roster[], updatedAt }
```
Before each write, a short-lived `miclink:lock:<roomId>` lock (SetNX + Lua release) makes read-modify-write atomic, enforcing the 3-guest limit strictly.

**Service** (`mic_link_service.go`): `Latest` / `Config` (owner) / `Request` / `Cancel` / `Leave` / `Mute` / `Approve` (owner) / `Reject` (owner) / `Remove` (owner). Eligibility checks for `all/followers/fans/fans_level` reuse the lucky bag model.

**HTTP** (`mic_link.go`, hosted in gift-service; the gateway strips `/api`):
- `GET  /mic-link/latest` (public read-only endpoint with a caller-specific view)
- `POST /mic-link/config|request|cancel|leave|mute|approve|reject|remove` (login required)

**Gateway** (`router.go`): `api.Any("/mic-link/*action", giftProxy)`; register the read-only GET as a public route.

### Frontend

- `types/micLink.ts`, `api/micLink.ts` (React Query hooks, 3s polling).
- `lib/micRtc.ts`: WHIP `publishMic` / WHEP `playMic`, with `micStreamName` for stream names.
- `features/live-room/MicLinkPanel.tsx`: `ownsStream` selects the host console (toggle + eligibility dropdown reusing `gl-bag-select-*` + request queue + on-air roster + OBS stage URL row) or viewer panel (request button + state machine + self mute/leave + read-only roster). This component manages the guest WHIP uplink lifecycle while `on_air`.
- `pages/MicStagePage.tsx`: standalone `/mic-stage/:roomId` route without the app shell; polls the on-air roster, creates a WHEP subscription per guest, and plays hidden `<audio>` elements for capture by the OBS browser source.
- Mounting: `LiveRoomPage` (viewers) and the `CreatorStudioPage` console (host), alongside `LuckyBagPanel`.
- Styling: `gl-mic-*`, blue accents distinct from lucky bag gold, reused `--gl-*` variables, no gradients, custom dropdowns, and no nested cards.
- Three languages: `micLink.*` keys cover zh-CN / ja-JP / en-US, use `_one`/`_other` for counts, and pass the locale parity test.

## Data model

No new MySQL tables. Only ephemeral Redis state (see above). Media is not stored in the database.

## Error handling

Machine-readable reason codes: `mic_link_disabled` / `mic_link_not_eligible` / `mic_link_request_exists` / `mic_link_slot_full` / `mic_link_request_not_found` / `mic_link_busy` / `forbidden`; the frontend maps them to localized toasts.

## Test strategy

- Backend: `mic_link_service_test.go` (miniredis + sqlite) covers feature gating, owner-only configuration, approval and the 3-guest cap, duplicate request rejection, mute/leave, owner removal, clearing state on disable, and follower eligibility.
- Frontend: reuse the existing typecheck / lint / locale parity / build pipeline.
- The WebRTC media plane requires end-to-end testing with real SRS and browsers and cannot be covered by CI unit tests. Validate manually after deployment (host enables → viewer requests → approval → guest joins and audio is heard → guest leaves).

## Deployment impact

- `deploy/srs.conf`: enable `rtc_server` (listen 8000, candidate `$CANDIDATE`) and vhost `rtc`.
- `deploy/docker-compose.yml`: add `8000:8000/udp` and the `CANDIDATE` environment variable to SRS.
- `deploy/nginx.conf` + `nginx.https.conf`: add the `/rtc/` → `srs:1985` proxy.
- Estimated additional memory: +30–90MB (audio only). **Note: the server firewall must allow UDP 8000.**
