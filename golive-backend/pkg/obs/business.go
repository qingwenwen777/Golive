package obs

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Business / cross-cutting metrics that don't fit neatly into a single
// service's package. Services import these and increment from the relevant
// code paths.

var (
	// gift-service: total gift revenue (coin) per status. Sum over a
	// minute = revenue rate. status ∈ {success, failed}.
	GiftRevenueTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gift_revenue_coin_total",
		Help: "Cumulative gift revenue in coins.",
	}, []string{"status"})

	// gift-service: super-chat amount counter, by tier.
	SuperChatRevenueTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "super_chat_revenue_coin_total",
		Help: "Cumulative super-chat revenue in coins.",
	}, []string{"tier", "status"})

	// gift-service outbox health.
	OutboxRowsByStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "outbox_rows",
		Help: "Outbox row counts by status.",
	}, []string{"status"})

	OutboxPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "outbox_published_total",
		Help: "Outbox messages successfully published.",
	}, []string{"topic"})

	OutboxFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "outbox_failed_total",
		Help: "Outbox publish failures.",
	}, []string{"topic"})

	// chat-service: danmu throughput (NOT keyed by roomId — too high
	// cardinality for Prom). Per-room top-N is exposed separately via
	// /debug/rooms in im-gateway.
	DanmuProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "danmu_processed_total",
		Help: "Danmu events processed by chat-service.",
	}, []string{"outcome"}) // success / rate_limited / filtered / persist_error

	// shared: DB / Redis / Kafka client-side counters. Each service
	// instruments its own callsites; labels stay coarse.
	DBQueriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "db_queries_total",
		Help: "Database operation count.",
	}, []string{"service", "op", "outcome"})

	DBQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "db_query_duration_seconds",
		Help:    "Database operation latency.",
		Buckets: prometheus.ExponentialBuckets(0.0005, 2, 14),
	}, []string{"service", "op"})

	RedisOpsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "redis_ops_total",
		Help: "Redis operation count.",
	}, []string{"service", "op", "outcome"})

	KafkaProducedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_produced_total",
		Help: "Kafka records produced.",
	}, []string{"service", "topic", "outcome"})

	KafkaConsumedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_consumed_total",
		Help: "Kafka records consumed.",
	}, []string{"service", "topic", "outcome"})
)
