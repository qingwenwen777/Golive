package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
)

// connCaps bounds concurrent connections per user and per client IP on this
// instance. Without it one account could open unlimited sockets (each with
// its own queue, goroutines and room membership).
type connCaps struct {
	perUser int // <= 0 disables
	perIP   int // <= 0 disables

	mu    sync.Mutex
	users map[string]int
	ips   map[string]int
}

func newConnCaps(perUser, perIP int) *connCaps {
	return &connCaps{perUser: perUser, perIP: perIP, users: map[string]int{}, ips: map[string]int{}}
}

// acquire reserves a slot for (userID, ip). On success it returns a release
// func (idempotent); otherwise the reason ("user" or "ip").
func (c *connCaps) acquire(userID, ip string) (release func(), reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.perUser > 0 && userID != "" && c.users[userID] >= c.perUser {
		return nil, "user"
	}
	if c.perIP > 0 && ip != "" && c.ips[ip] >= c.perIP {
		return nil, "ip"
	}
	if userID != "" {
		c.users[userID]++
	}
	if ip != "" {
		c.ips[ip]++
	}
	var once sync.Once
	return func() { once.Do(func() { c.release(userID, ip) }) }, ""
}

func (c *connCaps) release(userID, ip string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if userID != "" {
		if c.users[userID]--; c.users[userID] <= 0 {
			delete(c.users, userID)
		}
	}
	if ip != "" {
		if c.ips[ip]--; c.ips[ip] <= 0 {
			delete(c.ips, ip)
		}
	}
}

// clientIP returns the caller's IP. Forwarding headers are only honoured
// when the direct peer is a trusted proxy (nginx sets X-Real-IP), otherwise
// any client could pick its own IP and dodge the per-IP cap.
func clientIP(r *http.Request, trusted []*net.IPNet) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(host)
	if peer == nil || !ipIn(peer, trusted) {
		return host
	}
	if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ip != nil {
		return ip.String()
	}
	if ip := forwardedFor(r.Header.Values("X-Forwarded-For"), trusted); ip != nil {
		return ip.String()
	}
	return host
}

// forwardedFor returns the right-most X-Forwarded-For hop that isn't a
// trusted proxy: the address the outermost trusted proxy saw. Each proxy
// appends to the list, so everything left of that hop came from the client.
func forwardedFor(values []string, trusted []*net.IPNet) net.IP {
	var hops []string
	for _, v := range values {
		hops = append(hops, strings.Split(v, ",")...)
	}
	for i := len(hops) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(hops[i]))
		if ip == nil {
			return nil
		}
		if !ipIn(ip, trusted) {
			return ip
		}
	}
	return nil
}

func ipIn(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
