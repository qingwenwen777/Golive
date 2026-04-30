// Package metrics exports Prometheus counters / gauges for the WS layer.
//
// All metrics are registered on the default registry so /metrics serves them
// out of the box. Cardinality is kept tiny: roomId is NOT a label (would
// blow up high-fan-out scrapes); per-room top-N is exposed via a separate
// /debug/rooms endpoint.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ConnectionsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "im_connections_active",
		Help: "Number of currently open WebSocket connections.",
	})
	RoomsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "im_rooms_active",
		Help: "Number of rooms with at least one connection.",
	})
	ConnectionsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "im_connections_total",
		Help: "Cumulative count of accepted WebSocket connections.",
	})
	HandshakeFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "im_handshake_failures_total",
		Help: "Failed WS handshakes by reason.",
	}, []string{"reason"})

	MessagesSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "im_messages_sent_total",
		Help: "Server→client messages by type.",
	}, []string{"type"})
	MessagesReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "im_messages_received_total",
		Help: "Client→server messages by type.",
	}, []string{"type"})
	MessagesDropped = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "im_messages_dropped_total",
		Help: "Server-side drops by reason.",
	}, []string{"reason"})

	BroadcastLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "im_broadcast_latency_seconds",
		Help:    "Time from receiving a Redis pubsub message to fan-out completion.",
		Buckets: prometheus.ExponentialBuckets(0.0001, 2, 12),
	})
)
