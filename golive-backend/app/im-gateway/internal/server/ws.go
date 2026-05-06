// Package server hosts the WebSocket endpoint and Prometheus + debug routes.
package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/qingwenwen777/golive/app/im-gateway/internal/auth"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/hub"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/metrics"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/moderation"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/producer"
	"github.com/qingwenwen777/golive/pkg/logger"
)

// upgrader is shared across all upgrades. WriteBufferPool reduces alloc
// pressure at high fan-out (gorilla acquires a [WriteBufferSize]byte per
// write otherwise). CheckOrigin allows all in dev — production should swap
// this for the same allow-list api-gateway uses.
var writeBufPool = &sync.Pool{}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	WriteBufferPool: writeBufPool,
}

type WSHandler struct {
	hub        *hub.Hub
	verifier   auth.Verifier
	producer   producer.Producer
	moderation moderation.Checker
	cfg        WSConfig
	welcome    string
}

func NewWSHandler(h *hub.Hub, v auth.Verifier, p producer.Producer, m moderation.Checker, cfg WSConfig, welcome string) *WSHandler {
	return &WSHandler{hub: h, verifier: v, producer: p, moderation: m, cfg: cfg, welcome: welcome}
}

// ServeHTTP performs the handshake. Failure modes (per spec):
//
//	roomId missing -> 400
//	token missing/invalid -> 401
//	upgrade fails -> upgrader writes the response itself
func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	roomID := strings.TrimSpace(r.URL.Query().Get("roomId"))
	if roomID == "" {
		metrics.HandshakeFailures.WithLabelValues("missing_room").Inc()
		http.Error(w, "missing roomId", http.StatusBadRequest)
		return
	}
	ownerID := strings.TrimSpace(r.URL.Query().Get("ownerId"))
	identity, err := h.verifier.Verify(r.URL.Query().Get("token"))
	if err != nil {
		reason := "invalid_token"
		message := "invalid token"
		if errors.Is(err, auth.ErrMissingToken) {
			reason = "missing_token"
			message = "missing token"
		}
		metrics.HandshakeFailures.WithLabelValues(reason).Inc()
		http.Error(w, message, http.StatusUnauthorized)
		return
	}

	up := upgrader
	up.CheckOrigin = h.checkOrigin
	ws, err := up.Upgrade(w, r, nil)
	if err != nil {
		metrics.HandshakeFailures.WithLabelValues("upgrade").Inc()
		return
	}

	c := newConn(ws, roomID, ownerID, identity, h.hub, h.producer, h.moderation, h.cfg)
	room, err := h.hub.Join(roomID, c, c.initialViewerProfile())
	if err != nil {
		logger.L().Error("hub join", zap.Error(err))
		_ = ws.Close()
		return
	}

	// Greet exactly like ws-server.ts: welcome + initial viewer_count. We
	// SendDirect so other instances don't echo the welcome.
	c.Send(hub.EncodeSystem(h.welcome))
	c.Send(hub.EncodeViewerCount(room.Size()))

	// Each conn gets its own pumps. readPump exits on disconnect → leave hub.
	go c.writePump()
	go c.readPump(context.Background())
}

func (h *WSHandler) checkOrigin(r *http.Request) bool {
	return originAllowed(r, h.cfg.AllowedOrigins)
}

func originAllowed(r *http.Request, allowedOrigins []string) bool {
	origin, ok := canonicalOrigin(r.Header.Get("Origin"))
	if !ok {
		return false
	}
	if origin == "" {
		return true
	}
	if sameOrigin, ok := requestOrigin(r); ok && origin == sameOrigin {
		return true
	}
	for _, allowed := range allowedOrigins {
		if normalized, ok := canonicalOrigin(allowed); ok && normalized == origin {
			return true
		}
	}
	return false
}

func requestOrigin(r *http.Request) (string, bool) {
	proto := firstHeaderValue(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := firstHeaderValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return canonicalOrigin(proto + "://" + host)
}

func canonicalOrigin(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	return scheme + "://" + strings.ToLower(u.Host), true
}

func firstHeaderValue(raw string) string {
	if idx := strings.IndexByte(raw, ','); idx >= 0 {
		raw = raw[:idx]
	}
	return strings.TrimSpace(raw)
}
