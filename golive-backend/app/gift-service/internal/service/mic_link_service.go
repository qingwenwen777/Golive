package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/go-redis/redis/v9"
	"github.com/google/uuid"

	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
)

// Voice mic-link lets viewers join the streamer's audio live. State is
// ephemeral and lives entirely in Redis (no durable rows): a single JSON
// document per room, guarded by a short Redis lock on every mutation. The
// guest microphone travels over WebRTC (WHIP) to SRS; the OBS mic-stage page
// subscribes (WHEP) and OBS mixes guest audio into the outgoing RTMP, so
// viewers hear it through the normal FLV/HLS playback with no client changes.
const (
	MicLinkSlotMax  = 3
	micLinkStateTTL = 6 * time.Hour
	micLinkLockTTL  = 3 * time.Second
	micLinkLockWait = 2 * time.Second

	MicEligibilityAll       = "all"
	MicEligibilityFollowers = "followers"
	MicEligibilityFans      = "fans"
	MicEligibilityFansLevel = "fans_level"

	MicStatusNone    = "none"
	MicStatusPending = "pending"
	MicStatusOnAir   = "on_air"
)

var (
	ErrMicLinkDisabled        = errors.New("mic link disabled")
	ErrMicLinkNotEligible     = errors.New("mic link not eligible")
	ErrMicLinkRequestExists   = errors.New("mic link request exists")
	ErrMicLinkSlotFull        = errors.New("mic link slot full")
	ErrMicLinkRequestNotFound = errors.New("mic link request not found")
	ErrMicLinkForbidden       = errors.New("mic link forbidden")
	ErrMicLinkBusy            = errors.New("mic link busy")
)

type MicLinkService struct {
	orders *repo.OrderRepo
	rdb    *redis.Client
}

func NewMicLinkService(o *repo.OrderRepo, rdb *redis.Client) *MicLinkService {
	return &MicLinkService{orders: o, rdb: rdb}
}

// MicRequest is a pending join request awaiting owner approval.
type MicRequest struct {
	UserID    string `json:"userId"`
	Name      string `json:"name"`
	Avatar    string `json:"avatar,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

// MicGuest is an approved guest currently on the mic roster.
type MicGuest struct {
	UserID   string `json:"userId"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar,omitempty"`
	Muted    bool   `json:"muted"`
	JoinedAt int64  `json:"joinedAt"`
}

// micLinkState is the full per-room document stored in Redis.
type micLinkState struct {
	RoomID      string       `json:"roomId"`
	OwnerID     string       `json:"ownerId"`
	Enabled     bool         `json:"enabled"`
	Eligibility string       `json:"eligibility"`
	MinFanLevel int          `json:"minFanLevel"`
	Requests    []MicRequest `json:"requests"`
	Roster      []MicGuest   `json:"roster"`
	UpdatedAt   int64        `json:"updatedAt"`
}

// MicLinkView is the per-caller projection returned by the API.
type MicLinkView struct {
	Enabled     bool         `json:"enabled"`
	Eligibility string       `json:"eligibility"`
	MinFanLevel int          `json:"minFanLevel"`
	SlotMax     int          `json:"slotMax"`
	Roster      []MicGuest   `json:"roster"`
	Requests    []MicRequest `json:"requests,omitempty"`
	IsOwner     bool         `json:"isOwner"`
	MyStatus    string       `json:"myStatus"`
	MyMuted     bool         `json:"myMuted"`
}

type MicConfigReq struct {
	RoomID      string `json:"roomId"`
	Enabled     bool   `json:"enabled"`
	Eligibility string `json:"eligibility"`
	MinFanLevel int    `json:"minFanLevel"`
}

func micLinkKey(roomID string) string     { return "miclink:" + roomID }
func micLinkLockKey(roomID string) string { return "miclink:lock:" + roomID }

func normalizeMicEligibility(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case MicEligibilityFollowers:
		return MicEligibilityFollowers
	case MicEligibilityFans:
		return MicEligibilityFans
	case MicEligibilityFansLevel:
		return MicEligibilityFansLevel
	default:
		return MicEligibilityAll
	}
}

// lock acquires a short-lived per-room mutex so read-modify-write mutations on
// the Redis state document stay atomic across concurrent requests.
func (s *MicLinkService) lock(ctx context.Context, roomID string) (func(), error) {
	key := micLinkLockKey(roomID)
	token := uuid.NewString()
	deadline := time.Now().Add(micLinkLockWait)
	for {
		ok, err := s.rdb.SetNX(ctx, key, token, micLinkLockTTL).Result()
		if err != nil {
			return nil, err
		}
		if ok {
			return func() {
				_ = s.rdb.Eval(ctx,
					`if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`,
					[]string{key}, token).Err()
			}, nil
		}
		if time.Now().After(deadline) {
			return nil, ErrMicLinkBusy
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (s *MicLinkService) load(ctx context.Context, roomID string) (*micLinkState, error) {
	raw, err := s.rdb.Get(ctx, micLinkKey(roomID)).Result()
	if errors.Is(err, redis.Nil) {
		return &micLinkState{RoomID: roomID, Eligibility: MicEligibilityAll}, nil
	}
	if err != nil {
		return nil, err
	}
	var st micLinkState
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return nil, err
	}
	if st.RoomID == "" {
		st.RoomID = roomID
	}
	if st.Eligibility == "" {
		st.Eligibility = MicEligibilityAll
	}
	return &st, nil
}

func (s *MicLinkService) save(ctx context.Context, st *micLinkState) error {
	st.UpdatedAt = time.Now().UnixMilli()
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, micLinkKey(st.RoomID), b, micLinkStateTTL).Err()
}

// broadcast publishes a mic_link event to the room channel; im-gateway fans it
// out to all connected clients, mirroring chat/gift/lucky_bag.
func (s *MicLinkService) broadcast(ctx context.Context, roomID, event, userID string) {
	payload, err := json.Marshal(map[string]any{
		"type":   "mic_link",
		"event":  event,
		"roomId": roomID,
		"userId": userID,
		"ts":     time.Now().UnixMilli(),
	})
	if err != nil {
		return
	}
	_ = s.rdb.Publish(ctx, "room:"+roomID, payload).Err()
}

// Latest returns the per-caller view. Read-only, no lock; polled by clients.
func (s *MicLinkService) Latest(ctx context.Context, roomID, userID string) (*MicLinkView, error) {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return nil, ErrMicLinkForbidden
	}
	st, err := s.load(ctx, roomID)
	if err != nil {
		return nil, err
	}
	isOwner := userID != "" && userID == st.OwnerID
	return s.viewFor(st, userID, isOwner), nil
}

func (s *MicLinkService) viewFor(st *micLinkState, userID string, isOwner bool) *MicLinkView {
	v := &MicLinkView{
		Enabled:     st.Enabled,
		Eligibility: st.Eligibility,
		MinFanLevel: st.MinFanLevel,
		SlotMax:     MicLinkSlotMax,
		Roster:      st.Roster,
		IsOwner:     isOwner,
		MyStatus:    MicStatusNone,
	}
	if v.Roster == nil {
		v.Roster = []MicGuest{}
	}
	if isOwner {
		v.Requests = st.Requests
		if v.Requests == nil {
			v.Requests = []MicRequest{}
		}
	}
	if userID != "" {
		for _, g := range st.Roster {
			if g.UserID == userID {
				v.MyStatus = MicStatusOnAir
				v.MyMuted = g.Muted
				return v
			}
		}
		for _, r := range st.Requests {
			if r.UserID == userID {
				v.MyStatus = MicStatusPending
				return v
			}
		}
	}
	return v
}

// Config (owner) sets the master toggle + eligibility in one call. Disabling
// clears the roster and pending requests.
func (s *MicLinkService) Config(ctx context.Context, ownerID string, req MicConfigReq) (*MicLinkView, error) {
	roomID := strings.TrimSpace(req.RoomID)
	if roomID == "" || ownerID == "" {
		return nil, ErrMicLinkForbidden
	}
	actualOwner, err := s.orders.RoomOwner(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if actualOwner != ownerID {
		return nil, ErrMicLinkForbidden
	}
	unlock, err := s.lock(ctx, roomID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	st, err := s.load(ctx, roomID)
	if err != nil {
		return nil, err
	}
	st.OwnerID = ownerID
	st.Enabled = req.Enabled
	st.Eligibility = normalizeMicEligibility(req.Eligibility)
	if st.Eligibility == MicEligibilityFansLevel {
		st.MinFanLevel = req.MinFanLevel
		if st.MinFanLevel < 1 {
			st.MinFanLevel = 1
		}
	} else {
		st.MinFanLevel = 0
	}
	if !st.Enabled {
		st.Roster = nil
		st.Requests = nil
	}
	if err := s.save(ctx, st); err != nil {
		return nil, err
	}
	if st.Enabled {
		s.broadcast(ctx, roomID, "feature_enabled", "")
	} else {
		// Always announce a disable so guests are cleared client-side, even if
		// the toggle was already off (idempotent re-disable).
		s.broadcast(ctx, roomID, "feature_disabled", "")
	}
	return s.viewFor(st, ownerID, true), nil
}

// Request (viewer) creates a pending join request.
func (s *MicLinkService) Request(ctx context.Context, userID, roomID string) (*MicLinkView, error) {
	roomID = strings.TrimSpace(roomID)
	if userID == "" || roomID == "" {
		return nil, ErrMicLinkForbidden
	}
	unlock, err := s.lock(ctx, roomID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	st, err := s.load(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if !st.Enabled {
		return nil, ErrMicLinkDisabled
	}
	owner := st.OwnerID
	if owner == "" {
		owner, err = s.orders.RoomOwner(ctx, roomID)
		if err != nil {
			return nil, err
		}
		st.OwnerID = owner
	}
	if userID == owner {
		return nil, ErrMicLinkNotEligible
	}
	if hasRequest(st, userID) || onRoster(st, userID) {
		return nil, ErrMicLinkRequestExists
	}
	eligible, err := s.checkEligibility(ctx, userID, owner, roomID, st)
	if err != nil {
		return nil, err
	}
	if !eligible {
		return nil, ErrMicLinkNotEligible
	}
	name, avatar, err := s.orders.UserProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	st.Requests = append(st.Requests, MicRequest{
		UserID:    userID,
		Name:      name,
		Avatar:    avatar,
		CreatedAt: time.Now().UnixMilli(),
	})
	if err := s.save(ctx, st); err != nil {
		return nil, err
	}
	s.broadcast(ctx, roomID, "requested", userID)
	return s.viewFor(st, userID, userID == owner), nil
}

// Cancel (viewer) withdraws own pending request.
func (s *MicLinkService) Cancel(ctx context.Context, userID, roomID string) (*MicLinkView, error) {
	return s.mutateSelf(ctx, userID, roomID, "cancelled", func(st *micLinkState) error {
		if !removeRequest(st, userID) {
			return ErrMicLinkRequestNotFound
		}
		return nil
	})
}

// Leave (guest) takes self off the roster.
func (s *MicLinkService) Leave(ctx context.Context, userID, roomID string) (*MicLinkView, error) {
	return s.mutateSelf(ctx, userID, roomID, "left", func(st *micLinkState) error {
		if !removeGuest(st, userID) {
			return ErrMicLinkRequestNotFound
		}
		return nil
	})
}

// Mute (guest) toggles own mute flag.
func (s *MicLinkService) Mute(ctx context.Context, userID, roomID string, muted bool) (*MicLinkView, error) {
	event := "guest_unmuted"
	if muted {
		event = "guest_muted"
	}
	return s.mutateSelf(ctx, userID, roomID, event, func(st *micLinkState) error {
		for i := range st.Roster {
			if st.Roster[i].UserID == userID {
				st.Roster[i].Muted = muted
				return nil
			}
		}
		return ErrMicLinkRequestNotFound
	})
}

func (s *MicLinkService) mutateSelf(
	ctx context.Context,
	userID, roomID, event string,
	fn func(*micLinkState) error,
) (*MicLinkView, error) {
	roomID = strings.TrimSpace(roomID)
	if userID == "" || roomID == "" {
		return nil, ErrMicLinkForbidden
	}
	unlock, err := s.lock(ctx, roomID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	st, err := s.load(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if err := fn(st); err != nil {
		return nil, err
	}
	if err := s.save(ctx, st); err != nil {
		return nil, err
	}
	s.broadcast(ctx, roomID, event, userID)
	return s.viewFor(st, userID, userID == st.OwnerID), nil
}

// Approve (owner) promotes a pending request onto the roster (cap of 3).
func (s *MicLinkService) Approve(ctx context.Context, ownerID, roomID, targetID string) (*MicLinkView, error) {
	return s.ownerAct(ctx, ownerID, roomID, "approved", targetID, func(st *micLinkState) error {
		if len(st.Roster) >= MicLinkSlotMax {
			return ErrMicLinkSlotFull
		}
		req := takeRequest(st, targetID)
		if req == nil {
			return ErrMicLinkRequestNotFound
		}
		st.Roster = append(st.Roster, MicGuest{
			UserID:   req.UserID,
			Name:     req.Name,
			Avatar:   req.Avatar,
			JoinedAt: time.Now().UnixMilli(),
		})
		return nil
	})
}

// Reject (owner) drops a pending request.
func (s *MicLinkService) Reject(ctx context.Context, ownerID, roomID, targetID string) (*MicLinkView, error) {
	return s.ownerAct(ctx, ownerID, roomID, "rejected", targetID, func(st *micLinkState) error {
		if !removeRequest(st, targetID) {
			return ErrMicLinkRequestNotFound
		}
		return nil
	})
}

// Remove (owner) kicks an on-air guest off the mic.
func (s *MicLinkService) Remove(ctx context.Context, ownerID, roomID, targetID string) (*MicLinkView, error) {
	return s.ownerAct(ctx, ownerID, roomID, "removed", targetID, func(st *micLinkState) error {
		if !removeGuest(st, targetID) {
			return ErrMicLinkRequestNotFound
		}
		return nil
	})
}

func (s *MicLinkService) ownerAct(
	ctx context.Context,
	ownerID, roomID, event, targetID string,
	fn func(*micLinkState) error,
) (*MicLinkView, error) {
	roomID = strings.TrimSpace(roomID)
	targetID = strings.TrimSpace(targetID)
	if ownerID == "" || roomID == "" || targetID == "" {
		return nil, ErrMicLinkForbidden
	}
	actualOwner, err := s.orders.RoomOwner(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if actualOwner != ownerID {
		return nil, ErrMicLinkForbidden
	}
	unlock, err := s.lock(ctx, roomID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	st, err := s.load(ctx, roomID)
	if err != nil {
		return nil, err
	}
	st.OwnerID = ownerID
	if err := fn(st); err != nil {
		return nil, err
	}
	if err := s.save(ctx, st); err != nil {
		return nil, err
	}
	s.broadcast(ctx, roomID, event, targetID)
	return s.viewFor(st, ownerID, true), nil
}

func (s *MicLinkService) checkEligibility(
	ctx context.Context,
	userID, ownerID, roomID string,
	st *micLinkState,
) (bool, error) {
	switch st.Eligibility {
	case MicEligibilityAll:
		return true, nil
	case MicEligibilityFollowers:
		return s.isFollowing(ctx, userID, roomID)
	case MicEligibilityFans:
		_, ok, err := s.orders.FanBadgeLevelFor(ctx, userID, ownerID)
		return ok, err
	case MicEligibilityFansLevel:
		level, ok, err := s.orders.FanBadgeLevelFor(ctx, userID, ownerID)
		if err != nil {
			return false, err
		}
		return ok && level >= st.MinFanLevel, nil
	default:
		return true, nil
	}
}

func (s *MicLinkService) isFollowing(ctx context.Context, userID, roomID string) (bool, error) {
	if s.rdb == nil || userID == "" {
		return false, nil
	}
	channelID, err := s.orders.RoomChannelID(ctx, roomID)
	if err != nil {
		return false, err
	}
	if channelID == "" {
		return false, nil
	}
	_, err = s.rdb.ZScore(ctx, "user:"+userID+":follows", channelID).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func hasRequest(st *micLinkState, userID string) bool {
	for _, r := range st.Requests {
		if r.UserID == userID {
			return true
		}
	}
	return false
}

func onRoster(st *micLinkState, userID string) bool {
	for _, g := range st.Roster {
		if g.UserID == userID {
			return true
		}
	}
	return false
}

func takeRequest(st *micLinkState, userID string) *MicRequest {
	for i, r := range st.Requests {
		if r.UserID == userID {
			out := r
			st.Requests = append(st.Requests[:i], st.Requests[i+1:]...)
			return &out
		}
	}
	return nil
}

func removeRequest(st *micLinkState, userID string) bool {
	for i, r := range st.Requests {
		if r.UserID == userID {
			st.Requests = append(st.Requests[:i], st.Requests[i+1:]...)
			return true
		}
	}
	return false
}

func removeGuest(st *micLinkState, userID string) bool {
	for i, g := range st.Roster {
		if g.UserID == userID {
			st.Roster = append(st.Roster[:i], st.Roster[i+1:]...)
			return true
		}
	}
	return false
}
