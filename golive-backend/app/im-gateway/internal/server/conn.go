package server

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"github.com/qingwenwen777/golive/app/im-gateway/internal/auth"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/hub"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/metrics"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/moderation"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/producer"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/profile"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/rooms"
	"github.com/qingwenwen777/golive/pkg/chatfilter"
	"github.com/qingwenwen777/golive/pkg/logger"
)

// Conn is the per-client state. It implements hub.Sink.
type Conn struct {
	id       string
	roomID   string
	ownerID  string
	identity auth.Identity

	ws *websocket.Conn
	// send is never closed: fanout goroutines may call Send concurrently with
	// Close, and a send on a closed channel panics. done signals shutdown.
	send chan []byte
	done chan struct{}
	once sync.Once

	hub         *hub.Hub
	producer    producer.Producer
	moderation  moderation.Checker
	filter      *chatfilter.Filter
	chatLimiter UserLimiter
	profiles    profile.Resolver
	// profileRetry is when to resolve the viewer-list identity again because
	// the last attempt only produced a fallback; zero otherwise. Set before
	// the pumps start, then only touched by the readPump goroutine.
	profileRetry time.Time
	// limiter bounds inbound frames of every type on this connection;
	// droppedFrames counts rejections (readPump goroutine only).
	limiter       *rate.Limiter
	droppedFrames int
	cfg           WSConfig
}

// UserLimiter is the per-user chat rate limit. It is shared by all of a
// user's connections (and gateway instances), unlike Conn.limiter.
type UserLimiter interface {
	Allow(ctx context.Context, userID string) (bool, error)
}

// Deps are the collaborators shared by every connection.
type Deps struct {
	Hub        *hub.Hub
	Producer   producer.Producer
	Moderation moderation.Checker
	// Filter masks sensitive words in chat text. Nil disables masking.
	Filter *chatfilter.Filter
	// ChatLimiter enforces the per-user chat rate. Nil disables it.
	ChatLimiter UserLimiter
	// Rooms validates roomIds and supplies the trusted owner. Nil accepts
	// any well-formed id.
	Rooms rooms.Directory
	// Profiles resolves display name, avatar, level and fan badge from
	// server-side sources. Nil shows an id-derived name only.
	Profiles profile.Resolver
}

type WSConfig struct {
	ReadLimitBytes  int64
	ReadIdleTimeout time.Duration
	WriteDeadline   time.Duration
	SendBuffer      int
	PongWait        time.Duration
	MaxMessageRate  float64 // inbound frames/sec per connection, all types
	AllowedOrigins  []string
	// MaxConnsPerUser / MaxConnsPerIP cap concurrent connections on this
	// instance (<= 0 disables). TrustedProxies are the peers whose
	// X-Real-IP / X-Forwarded-For is believed.
	MaxConnsPerUser int
	MaxConnsPerIP   int
	TrustedProxies  []*net.IPNet
	// RequireKnownRoom rejects roomIds the room directory doesn't know.
	RequireKnownRoom bool
}

func newConn(ws *websocket.Conn, roomID, ownerID string, identity auth.Identity, d Deps, cfg WSConfig) *Conn {
	c := &Conn{
		id:          uuid.NewString(),
		roomID:      roomID,
		ownerID:     ownerID,
		identity:    identity,
		ws:          ws,
		send:        make(chan []byte, cfg.SendBuffer),
		done:        make(chan struct{}),
		hub:         d.Hub,
		producer:    d.Producer,
		moderation:  d.Moderation,
		filter:      d.Filter,
		chatLimiter: d.ChatLimiter,
		profiles:    d.Profiles,
		cfg:         cfg,
	}
	if cfg.MaxMessageRate > 0 {
		// burst = 1 second's allowance, minimum 1.
		burst := int(cfg.MaxMessageRate)
		if burst < 1 {
			burst = 1
		}
		c.limiter = rate.NewLimiter(rate.Limit(cfg.MaxMessageRate), burst)
	}
	return c
}

// Sink contract -----------------------------------------------------------

func (c *Conn) ID() string { return c.id }

// Send is non-blocking. Returns false when the queue is full so the caller
// (room.fanout) can evict this connection.
func (c *Conn) Send(payload []byte) bool {
	select {
	case <-c.done:
		return false
	default:
	}
	select {
	case c.send <- payload:
		return true
	default:
		return false
	}
}

func (c *Conn) Close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.ws.Close()
	})
}

// pumps -------------------------------------------------------------------

// readPump reads inbound frames, enforces deadlines + rate-limit, and
// dispatches to the inbound handler. Exits on any read error or closed
// flag — at that point Conn.Close has either run or is about to.
func (c *Conn) readPump(ctx context.Context) {
	defer func() {
		c.hub.Leave(c.roomID, c.id)
		c.Close()
	}()

	c.ws.SetReadLimit(c.cfg.ReadLimitBytes)
	_ = c.ws.SetReadDeadline(time.Now().Add(c.cfg.ReadIdleTimeout))
	c.ws.SetPongHandler(func(string) error {
		_ = c.ws.SetReadDeadline(time.Now().Add(c.cfg.ReadIdleTimeout))
		return nil
	})

	for {
		_, raw, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				logger.L().Debug("ws read closed", zap.String("conn", c.id), zap.Error(err))
			}
			return
		}
		// Any frame bumps the idle timeout — heartbeat doubles as keep-alive.
		_ = c.ws.SetReadDeadline(time.Now().Add(c.cfg.ReadIdleTimeout))

		if !c.handleFrame(ctx, raw) {
			logger.L().Info("closing flooding connection", zap.String("conn", c.id), zap.String("user", c.identity.UserID))
			return
		}
	}
}

// maxDroppedFrames is how many rate-limited frames a connection may send
// before it is closed. Well-behaved clients stay far below the frame rate.
const maxDroppedFrames = 50

// handleFrame rate-limits, decodes and dispatches one inbound frame. Every
// frame type counts against the per-connection limit — not just chat — so
// cheap-looking frames (viewer_profile, resume) cannot be flooded. Returns
// false when the connection should be closed for flooding.
func (c *Conn) handleFrame(ctx context.Context, raw []byte) bool {
	if c.limiter != nil && !c.limiter.Allow() {
		metrics.MessagesDropped.WithLabelValues("rate_limited").Inc()
		c.droppedFrames++
		return c.droppedFrames <= maxDroppedFrames
	}
	if !c.profileRetry.IsZero() && !time.Now().Before(c.profileRetry) {
		// The viewer-list entry still shows a fallback identity.
		c.updateViewer(c.resolveProfile(ctx, false))
	}
	var in hub.Inbound
	if err := json.Unmarshal(raw, &in); err != nil || in.Type == "" {
		metrics.MessagesDropped.WithLabelValues("bad_json").Inc()
		return true
	}
	c.dispatchInbound(ctx, in)
	return true
}

// writePump drains the send chan onto the websocket until Close. Closes the
// ws on any write error — readPump's Close will then no-op.
func (c *Conn) writePump() {
	defer c.Close()
	for {
		select {
		case <-c.done:
			return
		case payload := <-c.send:
			_ = c.ws.SetWriteDeadline(time.Now().Add(c.cfg.WriteDeadline))
			if err := c.ws.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		}
	}
}

// inboundTypeLabel bounds the metric label to the known frame types; the
// type string is client-controlled and every distinct value would otherwise
// create a new Prometheus series.
func inboundTypeLabel(t string) string {
	switch t {
	case "heartbeat", "resume", "viewer_profile", "chat":
		return t
	default:
		return "unknown"
	}
}

// dispatchInbound implements the client→server protocol from
// frontend src/mocks/ws-server.ts.
func (c *Conn) dispatchInbound(ctx context.Context, in hub.Inbound) {
	metrics.MessagesReceived.WithLabelValues(inboundTypeLabel(in.Type)).Inc()
	switch in.Type {
	case "heartbeat":
		// already bumped read deadline above
		return
	case "resume":
		// MVP: no replay buffer yet. Acknowledge with a system message so
		// the client can clear its reconnect spinner.
		_ = c.Send(hub.EncodeSystem("resumed"))
		return
	case "viewer_profile":
		// The payload is ignored: identity comes from the server. The frame
		// only asks us to pick up a profile change (the resolver refetches
		// at most every few seconds per user).
		c.handleViewerProfile(ctx)
		return
	case "chat":
		c.handleChat(ctx, in.Text, in.ClientID)
	default:
		metrics.MessagesDropped.WithLabelValues("unknown_type").Inc()
	}
}

// profileRetryDelay is how long a connection whose identity was only a
// fallback waits before asking again; the resolver also backs off per user.
const profileRetryDelay = 5 * time.Second

// resolveProfile returns the connection's public identity as of now, from
// server-side sources keyed by the authenticated user id. The resolver
// caches it; the connection keeps no copy, so renames and a recovered
// user-service show up. refresh asks the resolver to refetch a profile the
// client says changed. ok is false for a fallback served because
// user-service couldn't be reached; the viewer-list entry is then resolved
// again on a later frame.
func (c *Conn) resolveProfile(ctx context.Context, refresh bool) (profile.Profile, bool) {
	if c.profiles == nil {
		return profile.Profile{UserID: c.identity.UserID}, true
	}
	var p profile.Profile
	var ok bool
	if refresh {
		p, ok = c.profiles.Refresh(ctx, c.identity.UserID)
	} else {
		p, ok = c.profiles.Profile(ctx, c.identity.UserID)
	}
	c.profileRetry = time.Time{}
	if !ok {
		c.profileRetry = time.Now().Add(profileRetryDelay)
	}
	return p, ok
}

// joinRoom resolves the public identity (cached; bounded by the resolver
// timeout), so the viewer list shows the real name from the first push, and
// joins the room with it. A fallback isn't kept: the connection asks again on
// a later frame.
func (c *Conn) joinRoom(ctx context.Context) (*hub.Room, error) {
	p, _ := c.resolveProfile(ctx, false)
	return c.hub.Join(c.roomID, c, c.viewerProfile(p))
}

// updateViewer puts p into this connection's viewer-list entry (a no-op in
// the hub when nothing changed). A fallback never replaces the entry.
func (c *Conn) updateViewer(p profile.Profile, ok bool) {
	if c.hub == nil || !ok {
		return
	}
	c.hub.UpdateViewer(c.roomID, c.id, c.viewerProfile(p))
}

func (c *Conn) handleViewerProfile(ctx context.Context) {
	if c.hub == nil {
		return
	}
	c.updateViewer(c.resolveProfile(ctx, true))
}

func (c *Conn) viewerProfile(p profile.Profile) hub.ViewerProfile {
	return hub.ViewerProfile{
		UserID:    c.identity.UserID,
		User:      p.Name,
		Avatar:    p.Avatar,
		UserLevel: p.Level,
		IsOwner:   c.isOwner(),
	}
}

func (c *Conn) isOwner() bool {
	return c.ownerID != "" && c.identity.UserID != "" && c.ownerID == c.identity.UserID
}

// handleChat moderates and publishes one chat message. Everything other
// viewers see besides the text — id, name, avatar, level, fan badge, role —
// is decided here from server-side data; clientID is only echoed back to
// the sender in a chat_ack so it can match its own message.
func (c *Conn) handleChat(ctx context.Context, text, clientID string) {
	if !c.identity.CanChat() {
		_ = c.Send(hub.EncodeSystem("login required to chat"))
		metrics.MessagesDropped.WithLabelValues("anonymous_chat").Inc()
		return
	}
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > 200 {
		metrics.MessagesDropped.WithLabelValues("bad_chat").Inc()
		return
	}
	if c.chatLimiter != nil {
		ok, err := c.chatLimiter.Allow(ctx, c.identity.UserID)
		if err != nil {
			// Fail open: the per-connection limiter still bounds each socket.
			logger.L().Warn("per-user chat rate limit", zap.String("user", c.identity.UserID), zap.Error(err))
		} else if !ok {
			_ = c.Send(hub.EncodeSystem("You are sending messages too fast."))
			metrics.MessagesDropped.WithLabelValues("user_rate_limited").Inc()
			return
		}
	}
	role := ""
	if c.moderation != nil {
		state, err := c.moderation.State(ctx, c.roomID, c.identity.UserID)
		if err != nil {
			logger.L().Warn("load moderation state", zap.String("room", c.roomID), zap.Error(err))
		} else {
			if state.SiteBanned {
				_ = c.Send(hub.EncodeSystem("Your account is banned from chat interactions."))
				metrics.MessagesDropped.WithLabelValues("site_banned").Inc()
				return
			}
			if state.SiteMuted {
				_ = c.Send(hub.EncodeSystem("You are site-muted for " + formatMuteTTL(state.SiteMuteTTL) + "."))
				metrics.MessagesDropped.WithLabelValues("site_muted").Inc()
				return
			}
			if state.Muted {
				_ = c.Send(hub.EncodeSystem("You are muted for " + formatMuteTTL(state.MuteTTL) + "."))
				metrics.MessagesDropped.WithLabelValues("muted").Inc()
				return
			}
			if state.IsModerator {
				role = "moderator"
			}
		}
		blocked, err := c.moderation.ContainsBlockedWord(ctx, text)
		if err != nil {
			logger.L().Warn("check blocked word", zap.String("room", c.roomID), zap.Error(err))
		} else if blocked {
			_ = c.Send(hub.EncodeSystem("Message contains blocked words."))
			metrics.MessagesDropped.WithLabelValues("blocked_word").Inc()
			return
		}
	}
	if c.filter != nil {
		text = c.filter.Replace(text)
	}
	// The identity as of sending, not as of the handshake; the viewer list
	// follows it.
	p, ok := c.resolveProfile(ctx, false)
	c.updateViewer(p, ok)
	var hubBadge *hub.FanBadgePayload
	var producerBadge *producer.FanBadgePayload
	if c.profiles != nil && !c.isOwner() {
		if b := c.profiles.FanBadge(ctx, c.roomID, c.identity.UserID); b != nil {
			hubBadge = &hub.FanBadgePayload{CreatorID: b.CreatorID, Level: b.Level}
			producerBadge = &producer.FanBadgePayload{CreatorID: b.CreatorID, Level: b.Level}
		}
	}
	now := time.Now().UnixMilli()
	id := uuid.NewString()
	if err := c.producer.PublishChat(ctx, producer.ChatEvent{
		ID:        id,
		RoomID:    c.roomID,
		UserID:    c.identity.UserID,
		Username:  p.Name,
		Avatar:    p.Avatar,
		Text:      text,
		Role:      role,
		FanBadge:  producerBadge,
		UserLevel: p.Level,
		Ts:        now,
	}); err != nil {
		logger.L().Warn("publish chat", zap.Error(err))
		metrics.MessagesDropped.WithLabelValues("producer_err").Inc()
		return
	}
	if c.producer.LocalEcho() {
		_ = c.hub.Broadcast(ctx, c.roomID, hub.EncodeChat(id, c.identity.UserID, p.Name, p.Avatar, text, now, hubBadge, role, p.Level))
	}
	if cid := safeClientID(clientID); cid != "" {
		_ = c.Send(hub.EncodeChatAck(cid, id))
	}
}

func formatMuteTTL(ttl time.Duration) string {
	if ttl <= 0 {
		return "a moment"
	}
	minutes := int(ttl.Minutes())
	if minutes < 1 {
		return "less than 1 minute"
	}
	if minutes == 1 {
		return "1 minute"
	}
	return strconv.Itoa(minutes) + " minutes"
}

func safeClientID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 80 {
		return ""
	}
	if strings.ContainsAny(id, " \t\r\n") {
		return ""
	}
	return id
}
