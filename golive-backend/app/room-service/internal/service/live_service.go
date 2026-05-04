// Package service (live) handles publisher-side flows: issuing stream keys
// for RTMP publish, and verifying SRS on_publish / on_unpublish webhooks.
package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type LiveService struct {
	rooms          *repo.RoomRepo
	live           *repo.LiveRepo
	appointments   *repo.AppointmentRepo
	moderation     *repo.ModerationRepo
	replay         *ReplayService
	cloudflare     *CloudflareStreamClient
	keySecret      []byte
	keyTTL         time.Duration
	rtmpBase       string
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

func NewLiveService(rooms *repo.RoomRepo, live *repo.LiveRepo, secret string, ttl time.Duration, flvBase string, cloudflare ...*CloudflareStreamClient) *LiveService {
	if flvBase == "" {
		flvBase = "http://localhost:8082/live"
	}
	var cf *CloudflareStreamClient
	if len(cloudflare) > 0 && cloudflare[0] != nil && cloudflare[0].Enabled() {
		cf = cloudflare[0]
	}
	return &LiveService{
		rooms:          rooms,
		live:           live,
		cloudflare:     cf,
		keySecret:      []byte(secret),
		keyTTL:         ttl,
		rtmpBase:       "rtmp://localhost/live",
		flvBase:        strings.TrimRight(flvBase, "/"),
		unpublishGrace: 20 * time.Second,
		now:            time.Now,
	}
}

func (s *LiveService) SetRTMPBase(rtmpBase string) {
	rtmpBase = strings.TrimSpace(rtmpBase)
	if rtmpBase != "" {
		s.rtmpBase = strings.TrimRight(rtmpBase, "/")
	}
}

func (s *LiveService) SetAppointmentRepo(appointments *repo.AppointmentRepo) {
	s.appointments = appointments
}

func (s *LiveService) SetModerationRepo(moderation *repo.ModerationRepo) {
	s.moderation = moderation
}

func (s *LiveService) SetReplayService(replay *ReplayService) {
	s.replay = replay
}

type provisionedStreamInput struct {
	provider      string
	initialStatus string
	inputID       string
	rtmpServer    string
	streamKey     string
	playbackURL   string
}

// GoLiveReq is the body of POST /rooms/live.
type GoLiveReq struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	Category    string `json:"category" binding:"required"`
	Cover       string `json:"cover"`
	ChannelName string `json:"channelName"`
	Avatar      string `json:"avatar"`
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
	now := s.now()
	if s.appointments != nil {
		if appt, err := s.appointments.DueStartWindowForOwner(ctx, ownerID, now); err == nil && appt != nil {
			return nil, errcode.New(409, "An appointment is ready to start. Please start live from Live Appointments.")
		} else if err != nil && !errors.Is(err, repo.ErrAppointmentNotFound) {
			return nil, err
		}
	}
	if active, err := s.rooms.ActiveByOwner(ctx, ownerID); err == nil {
		if err := s.endRoom(ctx, active, now); err != nil {
			return nil, err
		}
		if active.StreamKey != "" {
			_ = s.live.Delete(ctx, active.StreamKey)
			_ = s.live.DeletePublishSession(ctx, active.StreamKey)
		}
		_ = s.releaseStreamInput(ctx, active)
		_ = s.broadcastEnded(ctx, active.ID, now)
	} else if !errors.Is(err, repo.ErrRoomNotFound) {
		return nil, err
	}

	roomID := "live-" + ownerID + "-" + strconv.FormatInt(now.UnixNano(), 36)
	channelID := "ch-" + ownerID
	ownerName := cleanDisplayName(req.ChannelName, ownerID)

	streamInput, err := s.provisionStreamInput(ctx, roomID, ownerID, ownerName, strings.TrimSpace(req.Title), now)
	if err != nil {
		return nil, err
	}

	room := &model.Room{
		ID:                  roomID,
		Title:               strings.TrimSpace(req.Title),
		Description:         cleanDescription(req.Description),
		Category:            strings.TrimSpace(req.Category),
		Cover:               req.Cover,
		Channel:             ownerName,
		ChannelID:           channelID,
		Verified:            false,
		Avatar:              cleanAvatar(req.Avatar, ownerName),
		Viewers:             0,
		PeakViewers:         0,
		StartedAt:           now,
		Status:              streamInput.initialStatus,
		OwnerID:             ownerID,
		StreamKey:           streamInput.streamKey,
		StreamProvider:      streamInput.provider,
		StreamInputID:       streamInput.inputID,
		StreamRTMPServer:    streamInput.rtmpServer,
		StreamPlaybackURL:   streamInput.playbackURL,
		ReplayUploadEnabled: false,
		ReplayStatus:        model.ReplayStatusNone,
		ReplayVisibility:    model.PostVisibilityPublic,
	}
	if err := s.rooms.Upsert(ctx, room); err != nil {
		return nil, err
	}
	if streamInput.provider == model.StreamProviderSRS && streamInput.streamKey != "" {
		if err := s.live.Save(ctx, streamInput.streamKey, roomID, s.keyTTL); err != nil {
			return nil, err
		}
	}
	if s.moderation != nil {
		if err := s.moderation.SyncRoomModerators(ctx, room.ID, ownerID); err != nil {
			return nil, err
		}
	}

	st := room.ToStream(now)
	st.PlaybackURL = s.playbackURL(room)
	st.StreamKey = streamInput.streamKey
	st.RTMPServer = streamInput.rtmpServer
	return &st, nil
}

func (s *LiveService) playbackURL(r *model.Room) string {
	if r != nil && r.StreamPlaybackURL != "" && (r.Status == model.StatusLive || r.Status == model.StatusPublishing) {
		return r.StreamPlaybackURL
	}
	if r == nil || r.Status != model.StatusLive || r.StreamKey == "" {
		return ""
	}
	return s.flvBase + "/" + r.StreamKey + ".flv"
}

func (s *LiveService) provisionStreamInput(ctx context.Context, roomID, ownerID, channelName, title string, now time.Time) (provisionedStreamInput, error) {
	if s.cloudflare != nil && s.cloudflare.Enabled() {
		input, err := s.cloudflare.CreateLiveInput(ctx, fmt.Sprintf("%s - %s", channelName, title))
		if err != nil {
			return provisionedStreamInput{}, errcode.New(502, "Cloudflare Stream live input creation failed")
		}
		return provisionedStreamInput{
			provider:      model.StreamProviderCloudflare,
			initialStatus: model.StatusLive,
			inputID:       input.UID,
			rtmpServer:    input.RTMPServer,
			streamKey:     input.StreamKey,
			playbackURL:   input.PlaybackURL,
		}, nil
	}
	return provisionedStreamInput{
		provider:      model.StreamProviderSRS,
		initialStatus: model.StatusPublishing,
		rtmpServer:    s.rtmpBase,
		streamKey:     s.generateKey(roomID, ownerID, now),
	}, nil
}

func (s *LiveService) releaseStreamInput(ctx context.Context, room *model.Room) error {
	if room == nil || room.StreamProvider != model.StreamProviderCloudflare || room.StreamInputID == "" {
		return nil
	}
	if s.cloudflare == nil || !s.cloudflare.Enabled() {
		return nil
	}
	return s.cloudflare.DeleteLiveInput(ctx, room.StreamInputID)
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
	st.StreamKey = room.StreamKey
	st.RTMPServer = room.StreamRTMPServer
	_ = s.broadcastRoomUpdated(ctx, room, now)
	return &st, nil
}

func cleanAvatar(raw, ownerName string) string {
	avatar := strings.TrimSpace(raw)
	if avatar != "" {
		return trimRunes(avatar, 500)
	}
	return "https://api.dicebear.com/7.x/initials/svg?seed=" + url.QueryEscape(ownerName)
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
		if err := s.endRoom(ctx, &room, endedAt); err != nil {
			return err
		}
		if err := s.broadcastEnded(ctx, room.ID, endedAt); err != nil {
			return err
		}
		if s.replay != nil && room.StreamProvider != model.StreamProviderCloudflare {
			s.replay.EnqueueUpload(ctx, room)
		}
		if room.StreamKey != "" {
			if err := s.live.Delete(ctx, room.StreamKey); err != nil && !errors.Is(err, repo.ErrStreamKeyNotFound) {
				return err
			}
			if err := s.live.DeletePublishSession(ctx, room.StreamKey); err != nil && !errors.Is(err, repo.ErrStreamKeyNotFound) {
				return err
			}
		}
		_ = s.releaseStreamInput(ctx, &room)
	}
	return nil
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

func canonicalStreamKey(stream string) (base string, isVariant bool) {
	for suffix := range streamVariantSuffixes {
		if strings.HasSuffix(stream, suffix) {
			return strings.TrimSuffix(stream, suffix), true
		}
	}
	return stream, false
}

// OnPublish authorizes the incoming RTMP publish. Returns nil on accept.
func (s *LiveService) OnPublish(ctx context.Context, req SRSPublishReq) error {
	if req.Stream == "" {
		return errors.New("missing stream key")
	}
	streamKey, isVariant := canonicalStreamKey(req.Stream)
	roomID, err := s.live.Resolve(ctx, streamKey)
	if err != nil {
		return fmt.Errorf("resolve stream key: %w", err)
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
	if err := s.live.SavePublishSession(ctx, streamKey, req.ClientID, s.keyTTL); err != nil {
		return err
	}
	switch room.Status {
	case model.StatusPublishing:
		return s.rooms.SetLive(ctx, roomID, s.now())
	case model.StatusLive:
		return nil
	case model.StatusEnding:
		return s.rooms.SetLive(ctx, roomID, s.now())
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
	streamKey, isVariant := canonicalStreamKey(req.Stream)
	roomID, err := s.live.Resolve(ctx, streamKey)
	if err != nil {
		// Unknown key: ignore — nothing to update.
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
	disconnectedAt := s.now()
	if s.unpublishGrace > 0 {
		go s.finalizeUnpublishAfterGrace(streamKey, roomID, req.ClientID, disconnectedAt)
		return nil
	}
	return s.finalizeUnpublish(ctx, streamKey, roomID, req.ClientID, disconnectedAt)
}

func (s *LiveService) finalizeUnpublishAfterGrace(streamKey, roomID, clientID string, disconnectedAt time.Time) {
	time.Sleep(s.unpublishGrace)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = s.finalizeUnpublish(ctx, streamKey, roomID, clientID, disconnectedAt)
}

func (s *LiveService) finalizeUnpublish(ctx context.Context, streamKey, roomID, clientID string, endedAt time.Time) error {
	if clientID != "" {
		activeClient, err := s.live.PublishSession(ctx, streamKey)
		if err == nil && activeClient != clientID {
			return nil
		}
		if err != nil && !errors.Is(err, repo.ErrStreamKeyNotFound) {
			return err
		}
	}
	activeRoomID, err := s.live.Resolve(ctx, streamKey)
	if errors.Is(err, repo.ErrStreamKeyNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if activeRoomID != roomID {
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
		_ = s.live.DeletePublishSession(ctx, streamKey)
		_ = s.live.Delete(ctx, streamKey)
		return nil
	}
	if room.Status == model.StatusEnded {
		_ = s.live.DeletePublishSession(ctx, streamKey)
		return s.live.Delete(ctx, streamKey)
	}
	if err := s.endRoom(ctx, room, endedAt); err != nil {
		return err
	}
	if err := s.broadcastEnded(ctx, roomID, endedAt); err != nil {
		return err
	}
	if s.replay != nil && room.StreamProvider != model.StreamProviderCloudflare {
		s.replay.EnqueueUpload(ctx, *room)
	}
	if err := s.live.DeletePublishSession(ctx, streamKey); err != nil {
		return err
	}
	if err := s.live.Delete(ctx, streamKey); err != nil {
		return err
	}
	_ = s.releaseStreamInput(ctx, room)
	return nil
}

func (s *LiveService) endRoom(ctx context.Context, room *model.Room, endedAt time.Time) error {
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
	if err := s.rooms.SetEndedWithMetrics(ctx, room.ID, endedAt, viewers, peak); err != nil {
		return err
	}
	if s.appointments != nil {
		if err := s.appointments.MarkCompletedByRoom(ctx, room.ID, endedAt); err != nil {
			return err
		}
	}
	if s.replay != nil && room.StreamProvider != model.StreamProviderCloudflare && room.ReplayUploadEnabled && normalizeReplayStatus(room.ReplayStatus) == model.ReplayStatusNone {
		if err := s.rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusPending, ""); err != nil {
			return err
		}
		room.ReplayStatus = model.ReplayStatusPending
	}
	return nil
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
