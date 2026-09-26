// Package httpserver builds the services' HTTP servers with timeouts and
// starts the optional pprof listener.
//
// A bare &http.Server{} has no timeouts: a client that never finishes its
// headers (slowloris) or keeps an idle connection open holds a goroutine and
// a file descriptor forever.
package httpserver

import (
	"errors"
	"net"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	// ReadHeaderTimeout bounds how long a client may take to send the
	// request line and headers.
	ReadHeaderTimeout = 10 * time.Second

	// ReadTimeout bounds reading a whole request, body included. Uploads are
	// at most 10 MB (nginx client_max_body_size) and nginx buffers request
	// bodies before proxying them, so the services receive uploads at LAN
	// speed; 60s still leaves room for a slow client talking to a service
	// directly (local development).
	ReadTimeout = 60 * time.Second

	// WriteTimeout bounds handling a request and writing its response,
	// counted from the end of the request headers. api-gateway abandons an
	// upstream after proxy.timeout (10s by default), so no proxied request
	// legitimately runs this long.
	WriteTimeout = 60 * time.Second

	// IdleTimeout closes keep-alive connections that carry no new request
	// for this long.
	IdleTimeout = 120 * time.Second
)

// New returns a server for request/response APIs with every timeout set.
func New(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: ReadHeaderTimeout,
		ReadTimeout:       ReadTimeout,
		WriteTimeout:      WriteTimeout,
		IdleTimeout:       IdleTimeout,
	}
}

// NewLongLived returns a server for handlers that keep connections open
// (WebSockets). Only the header and idle timeouts are set: ReadTimeout and
// WriteTimeout would apply to the upgrade request's connection, so an
// upgraded connection is governed by the handler's own deadlines
// (ping/pong, per-write deadlines) instead.
func NewLongLived(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: ReadHeaderTimeout,
		IdleTimeout:       IdleTimeout,
	}
}

// NewPprof returns a server that serves only the net/http/pprof handlers.
// It has no ReadTimeout or WriteTimeout because /debug/pprof/profile and
// /debug/pprof/trace stream for the requested number of seconds (pprof
// rejects a duration longer than the server's WriteTimeout).
func NewPprof(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return NewLongLived(addr, mux)
}

// StartPprof serves pprof on addr in the background and returns the server.
// An empty addr disables pprof: nothing is started and nil is returned
// (http.ListenAndServe("") would otherwise listen on port 80 of every
// interface). Profiles expose memory contents, so addr should be a loopback
// address such as "127.0.0.1:6060"; any other address is logged as a warning.
func StartPprof(addr string, log *zap.Logger) *http.Server {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		log.Info("pprof disabled (service.pprof_addr is empty)")
		return nil
	}
	if !IsLoopback(addr) {
		log.Warn("pprof listens beyond loopback; set service.pprof_addr to 127.0.0.1:<port>", zap.String("addr", addr))
	}
	srv := NewPprof(addr)
	go func() {
		log.Info("pprof listening", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn("pprof exit", zap.Error(err))
		}
	}()
	return srv
}

// IsLoopback reports whether addr ("host:port") listens on a loopback
// address only. An empty host (":6060") listens on every interface.
func IsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
