package server

import (
	"encoding/json"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/qingwenwen777/golive/app/im-gateway/internal/hub"
)

// NewMux composes /ws + /metrics + /healthz + /debug/rooms.
func NewMux(ws *WSHandler, h *hub.Hub) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/ws", ws)
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/debug/rooms", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(h.Snapshot(50))
	})
	return mux
}
