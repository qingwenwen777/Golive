package server

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"github.com/qingwenwen777/golive/app/im-gateway/internal/auth"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/hub"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/metrics"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/producer"
	"github.com/qingwenwen777/golive/pkg/logger"
)

// Conn is the per-client state. It implements hub.Sink.
type Conn struct {
	id       string
	roomID   string
	identity auth.Identity

	ws     *websocket.Conn
	send   chan []byte
	closed atomic.Bool
	once   sync.Once

	hub      *hub.Hub
	producer producer.Producer
	limiter  *rate.Limiter
	cfg      WSConfig
}

type WSConfig struct {
	ReadLimitBytes  int64
	ReadIdleTimeout time.Duration
	WriteDeadline   time.Duration
	SendBuffer      int
	PongWait        time.Duration
	MaxMessageRate  float64
}

func newConn(ws *websocket.Conn, roomID string, identity auth.Identity, h *hub.Hub, p producer.Producer, cfg WSConfig) *Conn {
	c := &Conn{
		id:       uuid.NewString(),
		roomID:   roomID,
		identity: identity,
		ws:       ws,
		send:     make(chan []byte, cfg.SendBuffer),
		hub:      h,
		producer: p,
		cfg:      cfg,
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
	if c.closed.Load() {
		return false
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
		c.closed.Store(true)
		close(c.send)
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

		var in hub.Inbound
		if err := json.Unmarshal(raw, &in); err != nil || in.Type == "" {
			metrics.MessagesDropped.WithLabelValues("bad_json").Inc()
			continue
		}
		c.dispatchInbound(ctx, in)
	}
}

// writePump drains the send chan onto the websocket. Closes the ws on any
// write error — readPump's Close will then no-op.
func (c *Conn) writePump() {
	defer c.Close()
	for payload := range c.send {
		_ = c.ws.SetWriteDeadline(time.Now().Add(c.cfg.WriteDeadline))
		if err := c.ws.WriteMessage(websocket.TextMessage, payload); err != nil {
			return
		}
	}
}

// dispatchInbound implements the client→server protocol from
// frontend src/mocks/ws-server.ts.
func (c *Conn) dispatchInbound(ctx context.Context, in hub.Inbound) {
	metrics.MessagesReceived.WithLabelValues(in.Type).Inc()
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
		c.handleViewerProfile(in.User, in.Avatar)
		return
	case "chat":
		c.handleChat(ctx, in.Text, in.User, in.Avatar, in.ClientID, in.FanBadge)
	default:
		metrics.MessagesDropped.WithLabelValues("unknown_type").Inc()
	}
}

func (c *Conn) handleViewerProfile(username, avatar string) {
	if c.hub == nil {
		return
	}
	username = safeUsername(username)
	avatar = safeAvatar(avatar)
	if username == "" {
		username = "Guest"
		if c.identity.UserID != "" {
			username = c.identity.UserID
		}
	}
	c.hub.UpdateViewer(c.roomID, c.id, hub.ViewerProfile{
		UserID: c.identity.UserID,
		User:   username,
		Avatar: avatar,
	})
}

func (c *Conn) handleChat(ctx context.Context, text, username, avatar, clientID string, fanBadge *hub.FanBadgePayload) {
	if !c.identity.CanChat() {
		_ = c.Send(hub.EncodeSystem("login required to chat"))
		metrics.MessagesDropped.WithLabelValues("anonymous_chat").Inc()
		return
	}
	if c.limiter != nil && !c.limiter.Allow() {
		metrics.MessagesDropped.WithLabelValues("rate_limited").Inc()
		return
	}
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > 200 {
		metrics.MessagesDropped.WithLabelValues("bad_chat").Inc()
		return
	}
	now := time.Now().UnixMilli()
	id := safeClientID(clientID)
	username = safeUsername(username)
	avatar = safeAvatar(avatar)
	hubFanBadge, producerFanBadge := safeFanBadge(fanBadge)
	if err := c.producer.PublishChat(ctx, producer.ChatEvent{
		RoomID:   c.roomID,
		UserID:   c.identity.UserID,
		Username: username,
		Avatar:   avatar,
		ClientID: id,
		Text:     text,
		FanBadge: producerFanBadge,
		Ts:       now,
	}); err != nil {
		logger.L().Warn("publish chat", zap.Error(err))
		metrics.MessagesDropped.WithLabelValues("producer_err").Inc()
		return
	}
	if id == "" {
		id = uuid.NewString()
	}
	if c.producer.LocalEcho() {
		display := username
		if display == "" {
			display = c.identity.UserID
		}
		_ = c.hub.Broadcast(ctx, c.roomID, hub.EncodeChat(id, c.identity.UserID, display, avatar, text, now, hubFanBadge))
	}
}

func safeFanBadge(in *hub.FanBadgePayload) (*hub.FanBadgePayload, *producer.FanBadgePayload) {
	if in == nil || strings.TrimSpace(in.CreatorID) == "" || in.Level < 1 {
		return nil, nil
	}
	level := in.Level
	if level > 99 {
		level = 99
	}
	creatorID := strings.TrimSpace(in.CreatorID)
	if len(creatorID) > 80 || strings.ContainsAny(creatorID, " \t\r\n") {
		return nil, nil
	}
	return &hub.FanBadgePayload{CreatorID: creatorID, Level: level},
		&producer.FanBadgePayload{CreatorID: creatorID, Level: level}
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

func safeUsername(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 64 {
		return ""
	}
	if strings.ContainsAny(name, "\r\n\t") {
		return ""
	}
	return name
}

func safeAvatar(avatar string) string {
	avatar = strings.TrimSpace(avatar)
	if avatar == "" || len([]rune(avatar)) > 500 {
		return ""
	}
	if strings.ContainsAny(avatar, "\r\n\t") {
		return ""
	}
	return avatar
}
