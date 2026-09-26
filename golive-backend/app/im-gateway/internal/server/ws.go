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
	"github.com/qingwenwen777/golive/app/im-gateway/internal/rooms"
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
	hub      *hub.Hub
	verifier auth.Verifier
	deps     Deps
	cfg      WSConfig
	welcome  string
	caps     *connCaps
}

func NewWSHandler(d Deps, v auth.Verifier, cfg WSConfig, welcome string) *WSHandler {
	return &WSHandler{
		hub:      d.Hub,
		verifier: v,
		deps:     d,
		cfg:      cfg,
		welcome:  welcome,
		caps:     newConnCaps(cfg.MaxConnsPerUser, cfg.MaxConnsPerIP),
	}
}

// ServeHTTP performs the handshake. Failure modes (per spec):
//
//	roomId missing / malformed -> 400
//	token missing/invalid -> 401
//	room unknown (with RequireKnownRoom) -> 404
//	too many connections for the user / IP -> 429
//	room lookup unavailable -> 503
//	upgrade fails -> upgrader writes the response itself
func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	roomID := strings.TrimSpace(r.URL.Query().Get("roomId"))
	if roomID == "" {
		metrics.HandshakeFailures.WithLabelValues("missing_room").Inc()
		http.Error(w, "missing roomId", http.StatusBadRequest)
		return
	}
	if !rooms.ValidID(roomID) {
		metrics.HandshakeFailures.WithLabelValues("bad_room").Inc()
		http.Error(w, "invalid roomId", http.StatusBadRequest)
		return
	}
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

	// The owner comes from room-service's data when the room is known; the
	// client-supplied ownerId is only a fallback for unknown rooms when they
	// are allowed at all.
	ownerID := ""
	known := false
	if h.deps.Rooms != nil {
		info, found, err := h.deps.Rooms.Lookup(r.Context(), roomID)
		if err != nil {
			logger.L().Warn("room lookup", zap.String("room", roomID), zap.Error(err))
			metrics.HandshakeFailures.WithLabelValues("room_lookup").Inc()
			http.Error(w, "room lookup unavailable", http.StatusServiceUnavailable)
			return
		}
		known = found
		ownerID = info.OwnerID
	}
	if !known {
		if h.cfg.RequireKnownRoom {
			metrics.HandshakeFailures.WithLabelValues("unknown_room").Inc()
			http.Error(w, "unknown room", http.StatusNotFound)
			return
		}
		ownerID = strings.TrimSpace(r.URL.Query().Get("ownerId"))
	}

	release, capped := h.caps.acquire(identity.UserID, clientIP(r, h.cfg.TrustedProxies))
	if capped != "" {
		metrics.HandshakeFailures.WithLabelValues("too_many_conns_" + capped).Inc()
		http.Error(w, "too many connections", http.StatusTooManyRequests)
		return
	}

	up := upgrader
	up.CheckOrigin = h.checkOrigin
	ws, err := up.Upgrade(w, r, nil)
	if err != nil {
		release()
		metrics.HandshakeFailures.WithLabelValues("upgrade").Inc()
		return
	}

	c := newConn(ws, roomID, ownerID, identity, h.deps, h.cfg)
	// Resolve the public identity (cached; bounded by the resolver timeout)
	// so the viewer list shows the real name from the first push.
	c.loadProfile(r.Context())
	room, err := h.hub.Join(roomID, c, c.viewerProfile())
	if err != nil {
		release()
		logger.L().Error("hub join", zap.Error(err))
		_ = ws.Close()
		return
	}

	// Greet exactly like ws-server.ts: welcome + initial viewer_count. We
	// SendDirect so other instances don't echo the welcome.
	c.Send(hub.EncodeSystem(h.welcome))
	c.Send(hub.EncodeViewerCount(room.Size()))

	// Each conn gets its own pumps. readPump exits on disconnect → leave hub
	// and free the connection slot.
	go c.writePump()
	go func() {
		defer release()
		c.readPump(context.Background())
	}()
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
