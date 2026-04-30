package hub

// Sink is the minimal write contract a connection must satisfy. Anything that
// can accept bytes asynchronously and report whether it's still alive can
// stand in for a *Conn during testing.
type Sink interface {
	// ID is opaque but must be unique among sinks attached to a Hub.
	ID() string
	// Send enqueues a payload non-blockingly. Returns false if the queue is
	// full (the caller will then evict the sink).
	Send(payload []byte) bool
	// Close terminates the sink. Idempotent.
	Close()
}
