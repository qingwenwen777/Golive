// Package discovery defines a minimal service registry abstraction.
// Concrete implementations live in subpackages (e.g. discovery/etcd).
package discovery

import "context"

// ServiceInfo describes one service instance.
type ServiceInfo struct {
	Name    string            // e.g. "user-service"
	ID      string            // unique instance id
	Addr    string            // host:port (gRPC)
	Meta    map[string]string // optional tags
	Version string
}

// Registry is implemented by etcd / consul / static backends.
type Registry interface {
	// Register announces this instance and keeps it alive until ctx is done.
	Register(ctx context.Context, info ServiceInfo) error
	// Deregister removes the instance.
	Deregister(ctx context.Context, info ServiceInfo) error
	// Resolve lists live instances of name.
	Resolve(ctx context.Context, name string) ([]ServiceInfo, error)
	// Watch emits updates when the instance set of name changes.
	Watch(ctx context.Context, name string) (<-chan []ServiceInfo, error)
	Close() error
}
