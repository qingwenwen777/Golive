// Package server hosts the WebSocket endpoint and Prometheus + debug routes.
package server

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/qingwenwen777/golive/app/im-gateway/internal/auth"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/hub"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/metrics"
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
	CheckOrigin:     func(*http.Request) bool { return true },
}

type WSHandler struct {
	hub      *hub.Hub
	verifier auth.Verifier
	producer producer.Producer
	cfg      WSConfig
	welcome  string
}

func NewWSHandler(h *hub.Hub, v auth.Verifier, p producer.Producer, cfg WSConfig, welcome string) *WSHandler {
	return &WSHandler{hub: h, verifier: v, producer: p, cfg: cfg, welcome: welcome}
}

// ServeHTTP performs the handshake. Failure modes (per spec):
//
//	roomId missing → 400
//	token invalid → 401   (token absent is OK → anonymous)
//	upgrade fails → upgrader writes the response itself
func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	roomID := strings.TrimSpace(r.URL.Query().Get("roomId"))
	if roomID == "" {
		metrics.HandshakeFailures.WithLabelValues("missing_room").Inc()
		http.Error(w, "missing roomId", http.StatusBadRequest)
		return
	}
	identity, err := h.verifier.Verify(r.URL.Query().Get("token"))
	if err != nil {
		metrics.HandshakeFailures.WithLabelValues("invalid_token").Inc()
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		metrics.HandshakeFailures.WithLabelValues("upgrade").Inc()
		return
	}

	c := newConn(ws, roomID, identity, h.hub, h.producer, h.cfg)
	if _, err := h.hub.Join(roomID, c); err != nil {
		logger.L().Error("hub join", zap.Error(err))
		_ = ws.Close()
		return
	}

	// Greet exactly like ws-server.ts: welcome + initial viewer_count. We
	// SendDirect so other instances don't echo the welcome.
	c.Send(hub.EncodeSystem(h.welcome))
	c.Send(hub.EncodeViewerCount(1))

	// Each conn gets its own pumps. readPump exits on disconnect → leave hub.
	go c.writePump()
	go c.readPump(context.Background())
}
