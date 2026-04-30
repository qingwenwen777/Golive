// Package etcd implements discovery.Registry on top of etcd v3.
//
// Layout: /golive/services/<name>/<id> -> JSON(ServiceInfo)
// A lease (default 15s) keeps each key alive; the registrar auto-renews.
package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/qingwenwen777/golive/pkg/discovery"
)

const (
	keyPrefix       = "/golive/services"
	defaultLeaseTTL = 15 // seconds
)

type Registry struct {
	cli    *clientv3.Client
	ttl    int64
	leases map[string]clientv3.LeaseID
}

// New builds a Registry from etcd endpoints (e.g. []string{"127.0.0.1:2379"}).
func New(endpoints []string) (*Registry, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("etcd dial: %w", err)
	}
	return &Registry{cli: cli, ttl: defaultLeaseTTL, leases: map[string]clientv3.LeaseID{}}, nil
}

func serviceKey(name, id string) string { return path.Join(keyPrefix, name, id) }

func (r *Registry) Register(ctx context.Context, info discovery.ServiceInfo) error {
	lease, err := r.cli.Grant(ctx, r.ttl)
	if err != nil {
		return fmt.Errorf("grant lease: %w", err)
	}
	body, _ := json.Marshal(info)
	if _, err := r.cli.Put(ctx, serviceKey(info.Name, info.ID), string(body), clientv3.WithLease(lease.ID)); err != nil {
		return fmt.Errorf("put: %w", err)
	}
	ch, err := r.cli.KeepAlive(ctx, lease.ID)
	if err != nil {
		return fmt.Errorf("keepalive: %w", err)
	}
	r.leases[info.ID] = lease.ID
	go func() {
		for range ch {
			// drain; stops when ctx is canceled
		}
	}()
	return nil
}

func (r *Registry) Deregister(ctx context.Context, info discovery.ServiceInfo) error {
	_, err := r.cli.Delete(ctx, serviceKey(info.Name, info.ID))
	return err
}

func (r *Registry) Resolve(ctx context.Context, name string) ([]discovery.ServiceInfo, error) {
	resp, err := r.cli.Get(ctx, path.Join(keyPrefix, name)+"/", clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	out := make([]discovery.ServiceInfo, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var s discovery.ServiceInfo
		if err := json.Unmarshal(kv.Value, &s); err == nil {
			out = append(out, s)
		}
	}
	return out, nil
}

func (r *Registry) Watch(ctx context.Context, name string) (<-chan []discovery.ServiceInfo, error) {
	out := make(chan []discovery.ServiceInfo, 1)
	prefix := path.Join(keyPrefix, name) + "/"
	w := r.cli.Watch(ctx, prefix, clientv3.WithPrefix())
	go func() {
		defer close(out)
		for range w {
			if list, err := r.Resolve(ctx, name); err == nil {
				out <- list
			}
		}
	}()
	return out, nil
}

func (r *Registry) Close() error { return r.cli.Close() }
