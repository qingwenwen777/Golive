// Package service (live) handles publisher-side flows: issuing stream keys
// for RTMP publish, and verifying SRS on_publish / on_unpublish webhooks.
package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
	"github.com/qingwenwen777/golive/pkg/logger"
	"github.com/qingwenwen777/golive/pkg/miclink"
	"go.uber.org/zap"
)

type LiveService struct {
	rooms          *repo.RoomRepo
	live           *repo.LiveRepo
	appointments   *repo.AppointmentRepo
	moderation     *repo.ModerationRepo
	textPolicy     TextPolicy
	replay         *ReplayService
	srs            *srsAPI
	keySecret      []byte
	keyTTL         time.Duration
	flvBase        string
	unpublishGrace time.Duration
	now            func() time.Time
}

type liveStatusMsg struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Text   string `json:"text,omitempty"`
	Ts     int64  `json:"ts"`
}

type liveMetadataMsg struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Cover       string `json:"cover"`
	Ts          int64  `json:"ts"`
}

func NewLiveService(rooms *repo.RoomRepo, live *repo.LiveRepo, secret string, ttl time.Duration, flvBase string) *LiveService {
	if flvBase == "" {
		flvBase = "http://localhost:8082/live"
	}
	return &LiveService{
		rooms:          rooms,
		live:           live,
		keySecret:      []byte(secret),
		keyTTL:         ttl,
		flvBase:        strings.TrimRight(flvBase, "/"),
		unpublishGrace: 20 * time.Second,
		now:            time.Now,
	}
}

func (s *LiveService) SetAppointmentRepo(appointments *repo.AppointmentRepo) {
	s.appointments = appointments
}

func (s *LiveService) SetModerationRepo(moderation *repo.ModerationRepo) {
	s.moderation = moderation
}

func (s *LiveService) SetTextPolicy(policy TextPolicy) {
	s.textPolicy = policy
}

func (s *LiveService) SetReplayService(replay *ReplayService) {
	s.replay = replay
}

// SetSRSAPIBase sets the SRS HTTP API base URL (e.g. "http://srs:1985") used
// to disconnect the publisher when a room is stopped. Empty disables it.
func (s *LiveService) SetSRSAPIBase(base string) {
	s.srs = newSRSAPI(base)
}

// srsPublishers returns SRS's stream list, stream name -> publisher client id,
// or nil when there is no SRS API or it cannot be asked; callers then go on
// as they did before they asked SRS.
func (s *LiveService) srsPublishers(ctx context.Context, roomID string) map[string]string {
	if s.srs == nil {
		return nil
	}
	publishers, err := s.srs.activePublishers(ctx)
	if err != nil {
		logger.L().Warn("list srs streams", zap.Error(err), zap.String("room_id", roomID))
		return nil
	}
	return publishers
}

// disconnectPublisher kicks the SRS clients publishing room, so a stopped
// room stops streaming and recording instead of only changing state: the
// publishers SRS lists for the room's play name and, for a room that went
// live before play names, for its raw publish key; with includeSession also
// the one on_publish last accepted, which SRS may not list yet (or at all
// while its API cannot be asked). Best effort: failures are logged and never
// fail the stop.
func (s *LiveService) disconnectPublisher(ctx context.Context, room *model.Room, includeSession bool) {
	if s.srs == nil || room.StreamKey == "" {
		return
	}
	var clientIDs []string
	publishers := s.srsPublishers(ctx, room.ID)
	for _, stream := range []string{playStreamName(room.ID, room.StreamKey), room.StreamKey} {
		if clientID := publishers[stream]; clientID != "" && !slices.Contains(clientIDs, clientID) {
			clientIDs = append(clientIDs, clientID)
		}
	}
	if includeSession {
		clientID, err := s.live.PublishSession(ctx, room.StreamKey)
		if err == nil && !slices.Contains(clientIDs, clientID) {
			clientIDs = append(clientIDs, clientID)
		} else if err != nil && !errors.Is(err, repo.ErrStreamKeyNotFound) {
			logger.L().Warn("load srs publish session", zap.Error(err), zap.String("room_id", room.ID))
		}
	}
	for _, clientID := range clientIDs {
		if err := s.srs.kickClient(ctx, clientID); err != nil {
			logger.L().Warn("kick srs publisher", zap.Error(err), zap.String("room_id", room.ID), zap.String("client_id", clientID))
		}
	}
}

// startLockTTL bounds how long a crashed GoLive or appointment Start can
// block the owner from starting another live.
const startLockTTL = 15 * time.Second

// lockOwnerStart serialises starting a live for ownerID (GoLive, appointment
// Start): each ends the owner's active room and creates a new one, so two
// concurrent starts would otherwise leave two active rooms.
func (s *LiveService) lockOwnerStart(ctx context.Context, ownerID string) (func(), error) {
	if s == nil || s.live == nil {
		return func() {}, nil
	}
	name := "start:" + ownerID
	token, ok, err := s.live.AcquireLock(ctx, name, startLockTTL)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errcode.New(409, "A live is already being started. Please try again.")
	}
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.live.ReleaseLock(ctx, name, token); err != nil {
			logger.L().Warn("release live start lock", zap.Error(err), zap.String("owner_id", ownerID))
		}
	}, nil
}

// GoLiveReq is the body of POST /rooms/live.
type GoLiveReq struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	Category    string `json:"category" binding:"required"`
	Cover       string `json:"cover"`
	ChannelName string `json:"channelName"`
	Avatar      string `json:"avatar"`
	FanClubOnly bool   `json:"fanClubOnly"`
}

// UpdateLiveReq is the body of PATCH /rooms/live.
type UpdateLiveReq struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	Cover       string `json:"cover"`
}

// GoLive provisions or refreshes a streaming session for the user. Returns a
// Stream WITH streamKey populated — only the publisher ever sees this.
func (s *LiveService) GoLive(ctx context.Context, ownerID string, req GoLiveReq) (*model.Stream, error) {
	unlock, err := s.lockOwnerStart(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	now := s.now()
	if s.appointments != nil {
		if appt, err := s.appointments.DueStartWindowForOwner(ctx, ownerID, now); err == nil && appt != nil {
			return nil, errcode.New(409, "An appointment is ready to start. Please start live from Live Appointments.")
		} else if err != nil && !errors.Is(err, repo.ErrAppointmentNotFound) {
			return nil, err
		}
	}
	if active, err := s.rooms.ActiveByOwner(ctx, ownerID); err == nil {
		if _, err := s.stopRoom(ctx, active, now, true); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, repo.ErrRoomNotFound) {
		return nil, err
	}

	roomID := "live-" + ownerID + "-" + strconv.FormatInt(now.UnixNano(), 36)
	channelID := "ch-" + ownerID
	ownerName := cleanDisplayName(req.ChannelName, ownerID)
	verified := false
	if profile, err := s.rooms.OwnerProfile(ctx, ownerID); err == nil {
		verified = profile.Verified
	}
	title := trimRunes(strings.TrimSpace(req.Title), 120)
	if title == "" {
		return nil, errcode.New(400, "title is required")
	}
	description := cleanDescription(req.Description)
	if s.textPolicy != nil {
		if err := s.textPolicy.EnsureTextAllowed(ctx, title, description); err != nil {
			return nil, err
		}
	}

	streamKey := s.generateKey(roomID, ownerID, now)

	room := &model.Room{
		ID:                  roomID,
		Title:               title,
		Description:         description,
		Category:            strings.TrimSpace(req.Category),
		Cover:               req.Cover,
		Channel:             ownerName,
		ChannelID:           channelID,
		Verified:            verified,
		Avatar:              cleanAvatar(req.Avatar),
		FanClubOnly:         req.FanClubOnly,
		Viewers:             0,
		PeakViewers:         0,
		StartedAt:           now,
		Status:              model.StatusPublishing,
		OwnerID:             ownerID,
		StreamKey:           streamKey,
		ReplayUploadEnabled: false,
		ReplayStatus:        model.ReplayStatusNone,
		ReplayVisibility:    model.PostVisibilityPublic,
	}
	if err := s.rooms.Upsert(ctx, room); err != nil {
		return nil, err
	}
	if err := s.live.Save(ctx, streamKey, roomID, s.keyTTL); err != nil {
		return nil, err
	}
	if s.moderation != nil {
		if err := s.moderation.SyncRoomModerators(ctx, room.ID, ownerID); err != nil {
			return nil, err
		}
	}

	st := room.ToStream(now)
	st.StreamKey = obsStreamKey(roomID, streamKey)
	return &st, nil
}

func (s *LiveService) playbackURL(r *model.Room) string {
	if r == nil || r.Status != model.StatusLive || r.StreamKey == "" {
		return ""
	}
	return s.flvBase + "/" + playStreamName(r.ID, r.StreamKey) + ".flv"
}

// publishKeyParam is the RTMP query parameter carrying the publish secret.
// Creators publish to live/<play name>?key=<secret> and viewers play
// live/<play name>.flv, so the public playback URL never contains the secret.
const publishKeyParam = "key"

// playTagLen is how many hex digits of its tag a play name carries (96 bits).
const playTagLen = 24

// playStreamName is the SRS stream name a room is published and played under:
// the room id plus a tag derived from the room's publish key. The room id is
// public (room lists, page URLs) but the tag is not, so only viewers given the
// playback URL can play the stream; non-members of a fan-club-only room get
// none. The tag is stable for the room's life and does not reveal the key.
// Empty when the room has no key.
func playStreamName(roomID, streamKey string) string {
	if roomID == "" || streamKey == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(streamKey))
	mac.Write([]byte("play:" + roomID))
	return roomID + "_" + hex.EncodeToString(mac.Sum(nil))[:playTagLen]
}

// playStreamRoomID returns the room id that stream, a play name, starts with,
// or "" when stream is not shaped like one.
func playStreamRoomID(stream string) string {
	i := strings.LastIndexByte(stream, '_')
	if i <= 0 || len(stream)-i-1 != playTagLen {
		return ""
	}
	return stream[:i]
}

// obsStreamKey is the value the owner pastes into OBS's "Stream key" field.
func obsStreamKey(roomID, secret string) string {
	name := playStreamName(roomID, secret)
	if name == "" {
		return ""
	}
	return name + "?" + publishKeyParam + "=" + secret
}

// publishKeyFromParam extracts the publish secret from the query string SRS
// reports in its hooks (e.g. "?key=lk_...").
func publishKeyFromParam(param string) string {
	values, err := url.ParseQuery(strings.TrimPrefix(strings.TrimSpace(param), "?"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(values.Get(publishKeyParam))
}

// UpdateLiveMetadata changes the active live room's public metadata without
// rotating the stream key or interrupting the publisher.
func (s *LiveService) UpdateLiveMetadata(ctx context.Context, ownerID string, req UpdateLiveReq) (*model.Stream, error) {
	title := trimRunes(strings.TrimSpace(req.Title), 120)
	if title == "" {
		return nil, errcode.New(400, "title is required")
	}
	description := cleanDescription(req.Description)
	cover := trimRunes(strings.TrimSpace(req.Cover), 500)
	if s.textPolicy != nil {
		if err := s.textPolicy.EnsureTextAllowed(ctx, title, description); err != nil {
			return nil, err
		}
	}

	room, err := s.rooms.ActiveByOwner(ctx, ownerID)
	if errors.Is(err, repo.ErrRoomNotFound) {
		return nil, errcode.New(404, "active live not found")
	}
	if err != nil {
		return nil, err
	}
	room.Title = title
	room.Description = description
	room.Cover = cover
	if err := s.rooms.UpdateMetadata(ctx, room.ID, title, description, cover); err != nil {
		return nil, err
	}

	now := s.now()
	st := room.ToStream(now)
	st.PlaybackURL = s.playbackURL(room)
	st.StreamKey = obsStreamKey(room.ID, room.StreamKey)
	_ = s.broadcastRoomUpdated(ctx, room, now)
	return &st, nil
}

// cleanAvatar keeps an empty avatar empty: clients draw the owner's initial
// instead of loading a generated image from a third-party service.
func cleanAvatar(raw string) string {
	return trimRunes(strings.TrimSpace(raw), 500)
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsUUIDLike reports whether s looks like a bare UUID. Used to reject channel
// names that are obviously the raw user id rather than a human-readable name.
func IsUUIDLike(s string) bool {
	return uuidRe.MatchString(strings.TrimSpace(s))
}

func cleanDisplayName(raw, ownerID string) string {
	name := trimRunes(strings.TrimSpace(raw), 64)
	if name == "" || IsUUIDLike(name) {
		suffix := ownerID
		if len(suffix) > 8 {
			suffix = suffix[:8]
		}
		return "Creator " + suffix
	}
	return name
}

func cleanDescription(raw string) string {
	return trimRunes(strings.TrimSpace(raw), 2000)
}

func trimRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// StopLive marks the owner's room as ended. Used by the publisher to end a
// session without waiting for SRS on_unpublish.
func (s *LiveService) StopLive(ctx context.Context, ownerID string) error {
	endedAt := s.now()
	rooms, err := s.rooms.ActiveRoomsByOwner(ctx, ownerID)
	if err != nil {
		return err
	}
	if len(rooms) == 0 {
		return nil
	}
	for _, room := range rooms {
		if _, err := s.stopRoom(ctx, &room, endedAt, true); err != nil {
			return err
		}
	}
	return nil
}

func (s *LiveService) ForceStopRoom(ctx context.Context, roomID string) error {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return nil
	}
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, repo.ErrRoomNotFound) {
			return nil
		}
		return err
	}
	if room.Status != model.StatusPublishing && room.Status != model.StatusLive && room.Status != model.StatusEnding {
		return nil
	}
	_, err = s.stopRoom(ctx, room, s.now(), true)
	return err
}

// ForceStopOwnerRooms ends every active room of ownerID like ForceStopRoom,
// kicking the publisher, and returns how many rooms this call ended. Rooms
// that already ended are left alone, so repeating it is harmless. A ban uses
// it to take the owner off air.
func (s *LiveService) ForceStopOwnerRooms(ctx context.Context, ownerID string) (int, error) {
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return 0, nil
	}
	rooms, err := s.rooms.ActiveRoomsByOwner(ctx, ownerID)
	if err != nil {
		return 0, err
	}
	ended := 0
	var errs []error
	for i := range rooms {
		stopped, err := s.stopRoom(ctx, &rooms[i], s.now(), true)
		if stopped {
			ended++
		}
		errs = append(errs, err)
	}
	return ended, errors.Join(errs...)
}

func (s *LiveService) PublishSystemNotice(ctx context.Context, roomID, text string) error {
	roomID = strings.TrimSpace(roomID)
	text = strings.TrimSpace(text)
	if roomID == "" || text == "" {
		return nil
	}
	payload, err := json.Marshal(struct {
		Type string `json:"type"`
		Text string `json:"text"`
		Ts   int64  `json:"ts"`
	}{
		Type: "system",
		Text: text,
		Ts:   s.now().UnixMilli(),
	})
	if err != nil {
		return err
	}
	return s.live.PublishRoomEvent(ctx, roomID, payload)
}

// SRSPublishReq is the JSON SRS posts to on_publish / on_unpublish.
type SRSPublishReq struct {
	Action   string `json:"action"`
	ClientID string `json:"client_id"`
	IP       string `json:"ip"`
	Vhost    string `json:"vhost"`
	App      string `json:"app"`
	TCURL    string `json:"tcUrl"`
	Stream   string `json:"stream"`
	Param    string `json:"param"`
}

var streamVariantSuffixes = map[string]struct{}{
	"_q720": {},
	"_q480": {},
}

func canonicalStreamName(stream string) (base string, isVariant bool) {
	for suffix := range streamVariantSuffixes {
		if strings.HasSuffix(stream, suffix) {
			return strings.TrimSuffix(stream, suffix), true
		}
	}
	return stream, false
}

// micLinkStreamPrefix marks WebRTC audio streams published by mic-link guests.
// These are not room RTMP streams, so the publish/unpublish hooks handle them
// without touching room state.
const micLinkStreamPrefix = miclink.StreamPrefix

// authorizeMicLinkPublish accepts a mic-link stream only with the token
// gift-service issued for that exact stream when the owner approved the guest.
//
// Guests publish via WHIP to /rtc/v1/whip/?app=live&stream=<name>&key=<token>.
// SRS 5 (SrsGoApiRtcWhip::do_serve_http) takes app and stream from that query
// and sets the request param to the raw query string, which on_publish reports
// as "param" without a leading "?" ("app=live&stream=...&key=..."). An RTMP
// publish to live/<name>?key=<token> reports "?key=<token>". publishKeyFromParam
// parses both.
func (s *LiveService) authorizeMicLinkPublish(ctx context.Context, req SRSPublishReq) error {
	token := publishKeyFromParam(req.Param)
	if token == "" {
		return errors.New("missing mic-link token")
	}
	expected, err := s.live.MicLinkPublishToken(ctx, req.Stream)
	if err != nil {
		return fmt.Errorf("resolve mic-link token: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
		return errors.New("mic-link token does not match stream")
	}
	return nil
}

// OnPublish authorizes the incoming RTMP publish. Returns nil on accept.
// The stream name is the room's play name; the secret stream key arrives in
// the "key" query parameter and must be the key that name belongs to.
func (s *LiveService) OnPublish(ctx context.Context, req SRSPublishReq) error {
	if req.Stream == "" {
		return errors.New("missing stream name")
	}
	if strings.HasPrefix(req.Stream, micLinkStreamPrefix) {
		// Mic-link guest WebRTC audio: token check, no room bookkeeping.
		return s.authorizeMicLinkPublish(ctx, req)
	}
	streamName, isVariant := canonicalStreamName(req.Stream)
	streamKey := publishKeyFromParam(req.Param)
	if streamKey == "" {
		return errors.New("missing stream key")
	}
	roomID, err := s.resolveStreamKey(ctx, streamName, streamKey)
	if err != nil {
		return fmt.Errorf("resolve stream key: %w", err)
	}
	if streamName != playStreamName(roomID, streamKey) {
		return errors.New("stream key does not belong to this stream")
	}
	room, err := s.rooms.GetByID(ctx, roomID)
	if err != nil {
		return fmt.Errorf("load room: %w", err)
	}
	if room.StreamKey != streamKey {
		_ = s.live.Delete(ctx, streamKey)
		_ = s.live.DeletePublishSession(ctx, streamKey)
		return errors.New("stream key is no longer active")
	}
	if isVariant {
		switch room.Status {
		case model.StatusPublishing, model.StatusLive:
			return nil
		case model.StatusEnding, model.StatusEnded:
			return errors.New("stream has ended")
		default:
			return errors.New("stream is not publishable")
		}
	}
	// SRS asks on_publish before it refuses a second publisher of a stream
	// that is on air ("stream busy"; 5.0.213 then reports the refused
	// attempt's unpublish), so such an attempt must change nothing: the
	// session and a pending disconnect stay those of the publisher on air.
	if publishers := s.srsPublishers(ctx, roomID); publishers != nil {
		if clientID, busy := publishers[streamName]; busy && clientID != req.ClientID {
			return errors.New("stream is already being published")
		}
	}
	if err := s.live.SavePublishSession(ctx, streamKey, req.ClientID, s.keyTTL); err != nil {
		return err
	}
	// (Re)connected: restart the key's TTL and cancel a pending unpublish.
	if err := s.live.Save(ctx, streamKey, roomID, s.keyTTL); err != nil {
		return err
	}
	if err := s.live.DeleteDisconnect(ctx, roomID); err != nil {
		return err
	}
	switch room.Status {
	case model.StatusPublishing, model.StatusEnding:
		wentLive, err := s.rooms.SetLive(ctx, roomID, s.now())
		if err != nil || wentLive {
			return err
		}
		// The room changed since it was read: fine if a concurrent publish
		// already set it live, but a stop in between must not be undone.
		if room, err = s.rooms.GetByID(ctx, roomID); err != nil {
			return fmt.Errorf("load room: %w", err)
		}
		if room.Status == model.StatusLive {
			return nil
		}
		_ = s.live.Delete(ctx, streamKey)
		_ = s.live.DeletePublishSession(ctx, streamKey)
		return errors.New("stream has ended")
	case model.StatusLive:
		return nil
	case model.StatusEnded:
		_ = s.live.Delete(ctx, streamKey)
		_ = s.live.DeletePublishSession(ctx, streamKey)
		return errors.New("stream has ended")
	default:
		return errors.New("stream is not publishable")
	}
}

func (s *LiveService) OnUnpublish(ctx context.Context, req SRSPublishReq) error {
	if req.Stream == "" {
		return nil
	}
	if strings.HasPrefix(req.Stream, micLinkStreamPrefix) {
		// Mic-link guest teardown: no room state. Guests publish over
		// WebRTC, which SRS does not record (rtc_to_rtmp off), so only an
		// RTMP publish under this name left a DVR file to drop; its param is
		// "?key=...", while WHIP's is the bare query (see
		// authorizeMicLinkPublish).
		if s.replay != nil && strings.HasPrefix(req.Param, "?") {
			go s.replay.cleanupStreamRecording(req.Stream)
		}
		return nil
	}
	streamName, isVariant := canonicalStreamName(req.Stream)
	streamKey := publishKeyFromParam(req.Param)
	if streamKey == "" {
		return nil
	}
	roomID, err := s.resolveStreamKey(ctx, streamName, streamKey)
	if err != nil || streamName != playStreamName(roomID, streamKey) {
		// Unknown key, or a key for a different stream: nothing to update.
		return nil
	}
	room, err := s.rooms.GetByID(ctx, roomID)
	if errors.Is(err, repo.ErrRoomNotFound) {
		_ = s.live.DeletePublishSession(ctx, streamKey)
		return s.live.Delete(ctx, streamKey)
	}
	if err != nil {
		return err
	}
	if room.StreamKey != streamKey {
		_ = s.live.Delete(ctx, streamKey)
		_ = s.live.DeletePublishSession(ctx, streamKey)
		return nil
	}
	if isVariant {
		return nil
	}
	if room.Status == model.StatusEnded {
		_ = s.live.DeletePublishSession(ctx, streamKey)
		return s.live.Delete(ctx, streamKey)
	}
	// Record the disconnect before waiting out the grace period, so a restart
	// meanwhile does not lose it: the reconciler finishes it instead.
	disconnect := repo.PublishDisconnect{ClientID: req.ClientID, At: s.now(), Hook: true}
	if err := s.live.SaveDisconnect(ctx, roomID, disconnect, disconnectRecordTTL); err != nil {
		return err
	}
	if s.unpublishGrace > 0 {
		go s.finalizeUnpublishAfterGrace(streamKey, roomID, disconnect.At)
		return nil
	}
	return s.finalizeUnpublish(ctx, streamKey, roomID, disconnect.At)
}

// disconnectRecordTTL bounds how long a recorded disconnect lingers if
// nothing finishes or clears it.
const disconnectRecordTTL = 24 * time.Hour

// resolveStreamKey returns the room bound to a publish key. When Redis lost
// the binding (TTL expiry after downtime, flush) the key of a room that is
// already live is still accepted from the room row when streamName is that
// room's play name, and the binding restored, so its publisher can reconnect
// and its unpublish still ends the room.
func (s *LiveService) resolveStreamKey(ctx context.Context, streamName, streamKey string) (string, error) {
	roomID, err := s.live.Resolve(ctx, streamKey)
	if !errors.Is(err, repo.ErrStreamKeyNotFound) {
		return roomID, err
	}
	candidate := playStreamRoomID(streamName)
	if candidate == "" {
		return "", err
	}
	room, rerr := s.rooms.GetByID(ctx, candidate)
	if rerr != nil || room.StreamKey == "" || subtle.ConstantTimeCompare([]byte(room.StreamKey), []byte(streamKey)) != 1 ||
		playStreamName(room.ID, room.StreamKey) != streamName {
		return "", err
	}
	if room.Status != model.StatusLive && room.Status != model.StatusEnding {
		return "", err
	}
	if err := s.live.Save(ctx, streamKey, room.ID, s.keyTTL); err != nil {
		return "", err
	}
	return room.ID, nil
}

func (s *LiveService) finalizeUnpublishAfterGrace(streamKey, roomID string, at time.Time) {
	time.Sleep(s.unpublishGrace)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = s.finalizeUnpublish(ctx, streamKey, roomID, at)
}

// finalizeUnpublish ends roomID when the disconnect recorded for it at at is
// still pending: the publisher did not reconnect (which clears the record), no
// newer publisher took over the stream key, and SRS, when it can be asked,
// lists no publisher for the stream. The room ends at the disconnect.
func (s *LiveService) finalizeUnpublish(ctx context.Context, streamKey, roomID string, at time.Time) error {
	disconnect, err := s.live.Disconnect(ctx, roomID)
	if errors.Is(err, repo.ErrStreamKeyNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !disconnect.At.Equal(at) && s.now().Sub(disconnect.At) < s.unpublishGrace {
		// A newer disconnect replaced this one; its grace period runs on.
		return nil
	}
	if disconnect.ClientID != "" {
		activeClient, err := s.live.PublishSession(ctx, streamKey)
		if err == nil && activeClient != disconnect.ClientID {
			// A late unpublish of a publisher that was already replaced.
			return s.live.DeleteDisconnect(ctx, roomID)
		}
		if err != nil && !errors.Is(err, repo.ErrStreamKeyNotFound) {
			return err
		}
	}
	room, err := s.rooms.GetByID(ctx, roomID)
	if errors.Is(err, repo.ErrRoomNotFound) {
		_ = s.live.DeletePublishSession(ctx, streamKey)
		return s.live.Delete(ctx, streamKey)
	}
	if err != nil {
		return err
	}
	if room.StreamKey != streamKey {
		_ = s.live.DeletePublishSession(ctx, streamKey)
		_ = s.live.Delete(ctx, streamKey)
		return nil
	}
	if room.Status == model.StatusEnded {
		_ = s.live.DeletePublishSession(ctx, streamKey)
		_ = s.live.DeleteDisconnect(ctx, roomID)
		return s.live.Delete(ctx, streamKey)
	}
	if publishers := s.srsPublishers(ctx, roomID); publishers != nil {
		if _, publishing := publishers[playStreamName(roomID, streamKey)]; publishing {
			// On air after all, e.g. the unpublish was of an attempt SRS
			// refused as busy while another publisher kept streaming.
			return s.live.DeleteDisconnect(ctx, roomID)
		}
	}
	_, err = s.stopRoom(ctx, room, disconnect.At, false)
	return err
}

// stopRoom ends room and does the follow-up work of every stop: kick the SRS
// publishers (the one on_publish stored only when kick is set), queue the
// replay upload or recording cleanup, drop the stream key and publish
// session, and tell viewers. It reports whether this call ended the room;
// when another path ended it first, that path owns the follow-up work and
// nothing more is done here.
func (s *LiveService) stopRoom(ctx context.Context, room *model.Room, endedAt time.Time, kick bool) (bool, error) {
	ended, err := s.endRoom(ctx, room, endedAt)
	if err != nil || !ended {
		return false, err
	}
	// Even a room whose publisher is gone can have one SRS lists: one that
	// raced the stop, or one still publishing under the raw key from before
	// play names. Kicked, it stops streaming into nothing and SRS closes the
	// recording the replay upload waits for.
	s.disconnectPublisher(ctx, room, kick)
	if s.replay != nil {
		s.replay.EnqueueUpload(ctx, *room)
	}
	errs := []error{s.live.DeleteDisconnect(ctx, room.ID)}
	if room.StreamKey != "" {
		errs = append(errs,
			s.live.Delete(ctx, room.StreamKey),
			s.live.DeletePublishSession(ctx, room.StreamKey),
		)
	}
	errs = append(errs, s.broadcastEnded(ctx, room.ID, endedAt))
	return true, errors.Join(errs...)
}

// endRoom moves room to ended and reports whether this call did so; false
// means it was no longer active (another stop won) and nothing was changed.
func (s *LiveService) endRoom(ctx context.Context, room *model.Room, endedAt time.Time) (bool, error) {
	viewers, peak := int64(-1), int64(-1)
	if metrics, err := s.live.ViewerMetrics(ctx, room.ID); err == nil && metrics != nil {
		viewers = metrics.Viewers
		peak = metrics.Peak
	}
	if peak < room.PeakViewers {
		peak = room.PeakViewers
	}
	if viewers < 0 {
		viewers = room.Viewers
	}
	if peak < viewers {
		peak = viewers
	}
	ended, err := s.rooms.SetEndedWithMetrics(ctx, room.ID, endedAt, viewers, peak)
	if err != nil || !ended {
		return false, err
	}
	if s.appointments != nil {
		if err := s.appointments.MarkCompletedByRoom(ctx, room.ID, endedAt); err != nil {
			return true, err
		}
	}
	if s.replay != nil && room.ReplayUploadEnabled && normalizeReplayStatus(room.ReplayStatus) == model.ReplayStatusNone {
		if err := s.rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusPending, ""); err != nil {
			return true, err
		}
		room.ReplayStatus = model.ReplayStatusPending
	}
	return true, nil
}

func (s *LiveService) broadcastEnded(ctx context.Context, roomID string, endedAt time.Time) error {
	payload, err := json.Marshal(liveStatusMsg{
		Type:   "live_status",
		Status: model.StatusEnded,
		Text:   "Live has ended.",
		Ts:     endedAt.UnixMilli(),
	})
	if err != nil {
		return err
	}
	return s.live.PublishRoomEvent(ctx, roomID, payload)
}

func (s *LiveService) broadcastRoomUpdated(ctx context.Context, room *model.Room, updatedAt time.Time) error {
	payload, err := json.Marshal(liveMetadataMsg{
		Type:        "room_updated",
		Title:       room.Title,
		Description: room.Description,
		Cover:       room.Cover,
		Ts:          updatedAt.UnixMilli(),
	})
	if err != nil {
		return err
	}
	return s.live.PublishRoomEvent(ctx, room.ID, payload)
}

// generateKey produces a deterministic-but-unpredictable HMAC tag.
func (s *LiveService) generateKey(roomID, userID string, t time.Time) string {
	mac := hmac.New(sha256.New, s.keySecret)
	mac.Write([]byte(roomID + ":" + userID + ":" + strconv.FormatInt(t.UnixNano(), 10)))
	sum := hex.EncodeToString(mac.Sum(nil))
	// Prefix makes it easy to spot in SRS logs.
	return "lk_" + strings.ToLower(sum[:32])
}
