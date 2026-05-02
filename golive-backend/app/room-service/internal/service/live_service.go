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
)

type LiveService struct {
	rooms     *repo.RoomRepo
	live      *repo.LiveRepo
	keySecret []byte
	keyTTL    time.Duration
	flvBase   string
	now       func() time.Time
}

type liveStatusMsg struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Text   string `json:"text,omitempty"`
	Ts     int64  `json:"ts"`
}

func NewLiveService(rooms *repo.RoomRepo, live *repo.LiveRepo, secret string, ttl time.Duration, flvBase string) *LiveService {
	if flvBase == "" {
		flvBase = "http://localhost:8082/live"
	}
	return &LiveService{
		rooms:     rooms,
		live:      live,
		keySecret: []byte(secret),
		keyTTL:    ttl,
		flvBase:   strings.TrimRight(flvBase, "/"),
		now:       time.Now,
	}
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

// GoLive provisions or refreshes a streaming session for the user. Returns a
// Stream WITH streamKey populated — only the publisher ever sees this.
func (s *LiveService) GoLive(ctx context.Context, ownerID string, req GoLiveReq) (*model.Stream, error) {
	now := s.now()
	if active, err := s.rooms.ActiveByOwner(ctx, ownerID); err == nil {
		if err := s.endRoom(ctx, active, now); err != nil {
			return nil, err
		}
		if active.StreamKey != "" {
			_ = s.live.Delete(ctx, active.StreamKey)
		}
		_ = s.broadcastEnded(ctx, active.ID, now)
	} else if !errors.Is(err, repo.ErrRoomNotFound) {
		return nil, err
	}

	roomID := "live-" + ownerID + "-" + strconv.FormatInt(now.UnixNano(), 36)
	channelID := "ch-" + ownerID
	ownerName := cleanDisplayName(req.ChannelName, ownerID)

	streamKey := s.generateKey(roomID, ownerID, now)

	room := &model.Room{
		ID:          roomID,
		Title:       strings.TrimSpace(req.Title),
		Description: cleanDescription(req.Description),
		Category:    strings.TrimSpace(req.Category),
		Cover:       req.Cover,
		Channel:     ownerName,
		ChannelID:   channelID,
		Verified:    false,
		Avatar:      cleanAvatar(req.Avatar, ownerName),
		Viewers:     0,
		PeakViewers: 0,
		StartedAt:   now,
		Status:      model.StatusPublishing,
		OwnerID:     ownerID,
		StreamKey:   streamKey,
	}
	if err := s.rooms.Upsert(ctx, room); err != nil {
		return nil, err
	}
	if err := s.live.Save(ctx, streamKey, roomID, s.keyTTL); err != nil {
		return nil, err
	}

	st := room.ToStream(now)
	st.StreamKey = streamKey
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
		if room.StreamKey != "" {
			if err := s.live.Delete(ctx, room.StreamKey); err != nil && !errors.Is(err, repo.ErrStreamKeyNotFound) {
				return err
			}
		}
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
	switch room.Status {
	case model.StatusPublishing:
		return s.rooms.SetLive(ctx, roomID, s.now())
	case model.StatusLive:
		return nil
	case model.StatusEnding, model.StatusEnded:
		_ = s.live.Delete(ctx, streamKey)
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
		return s.live.Delete(ctx, streamKey)
	}
	if err != nil {
		return err
	}
	if room.StreamKey != streamKey {
		_ = s.live.Delete(ctx, streamKey)
		return nil
	}
	if isVariant {
		return nil
	}
	if room.Status == model.StatusEnded {
		return s.live.Delete(ctx, streamKey)
	}
	endedAt := s.now()
	if err := s.endRoom(ctx, room, endedAt); err != nil {
		return err
	}
	if err := s.broadcastEnded(ctx, roomID, endedAt); err != nil {
		return err
	}
	return s.live.Delete(ctx, streamKey)
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
	return s.rooms.SetEndedWithMetrics(ctx, room.ID, endedAt, viewers, peak)
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

// generateKey produces a deterministic-but-unpredictable HMAC tag.
func (s *LiveService) generateKey(roomID, userID string, t time.Time) string {
	mac := hmac.New(sha256.New, s.keySecret)
	mac.Write([]byte(roomID + ":" + userID + ":" + strconv.FormatInt(t.UnixNano(), 10)))
	sum := hex.EncodeToString(mac.Sum(nil))
	// Prefix makes it easy to spot in SRS logs.
	return "lk_" + strings.ToLower(sum[:32])
}
