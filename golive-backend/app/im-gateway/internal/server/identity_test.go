package server

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/im-gateway/internal/auth"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/hub"
	"github.com/qingwenwen777/golive/app/im-gateway/internal/profile"
)

// scriptedProfiles answers with whatever the test last set, like a resolver
// whose cache entry was just refetched.
type scriptedProfiles struct {
	mu        sync.Mutex
	p         profile.Profile
	ok        bool
	lookups   int
	refreshes int
}

func (s *scriptedProfiles) set(p profile.Profile, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.p, s.ok = p, ok
}

func (s *scriptedProfiles) Profile(_ context.Context, userID string) (profile.Profile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lookups++
	p := s.p
	p.UserID = userID
	return p, s.ok
}

func (s *scriptedProfiles) Refresh(ctx context.Context, userID string) (profile.Profile, bool) {
	s.mu.Lock()
	s.refreshes++
	s.mu.Unlock()
	return s.Profile(ctx, userID)
}

func (s *scriptedProfiles) FanBadge(context.Context, string, string) *profile.FanBadge { return nil }

func (s *scriptedProfiles) counts() (lookups, refreshes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lookups, s.refreshes
}

// joinTestRoom joins c to its room the way the handshake does.
func joinTestRoom(t *testing.T, c *Conn) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c.send = make(chan []byte, 256)
	c.hub = hub.New(ctx, &captureBroker{}, 0, hub.WithViewerFlushInterval(time.Millisecond))
	_, err := c.joinRoom(context.Background())
	require.NoError(t, err)
}

// waitForViewer reads c's queue until a viewer_list shows a viewer called
// name, and returns that entry.
func waitForViewer(t *testing.T, c *Conn, name string) hub.ViewerListItem {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case raw := <-c.send:
			var list hub.ViewerListMsg
			if json.Unmarshal(raw, &list) != nil || list.Type != "viewer_list" {
				continue
			}
			for _, v := range list.Viewers {
				if v.User == name {
					return v
				}
			}
		case <-deadline:
			t.Fatalf("no viewer_list shows %q", name)
		}
	}
}

// requireNoViewer fails if a viewer_list showing name reaches c soon.
func requireNoViewer(t *testing.T, c *Conn, name string) {
	t.Helper()
	deadline := time.After(100 * time.Millisecond)
	for {
		select {
		case raw := <-c.send:
			var list hub.ViewerListMsg
			if json.Unmarshal(raw, &list) != nil || list.Type != "viewer_list" {
				continue
			}
			for _, v := range list.Viewers {
				require.NotEqual(t, name, v.User)
			}
		case <-deadline:
			return
		}
	}
}

func heartbeat(t *testing.T, c *Conn) {
	t.Helper()
	require.True(t, c.handleFrame(context.Background(), []byte(`{"type":"heartbeat"}`)))
}

// A connection that resolved its identity while user-service was down (as
// every client does when they all reconnect after a gateway deploy) kept no
// name and level 0 for its whole life, in chat and in the viewer list.
func TestConn_FallbackIdentityIsResolvedAgain(t *testing.T) {
	res := &scriptedProfiles{}
	res.set(profile.Profile{}, false)
	prod := &fakeProducer{}
	c := newTestConn(auth.Identity{UserID: "u-7"}, prod)
	c.profiles = res
	joinTestRoom(t, c)
	require.Equal(t, "u-7", waitForViewer(t, c, "").UserID)

	// Retries wait for profileRetry.
	heartbeat(t, c)
	lookups, _ := res.counts()
	require.Equal(t, 1, lookups)

	// Still down when the retry is due: the entry stays, and the next retry
	// is scheduled.
	c.profileRetry = time.Now().Add(-time.Millisecond)
	heartbeat(t, c)
	lookups, _ = res.counts()
	require.Equal(t, 2, lookups)
	require.True(t, c.profileRetry.After(time.Now()))

	// Back up: the next due retry fixes the viewer list, and chat carries
	// the real identity.
	res.set(profile.Profile{Name: "Luna", Level: 3}, true)
	c.profileRetry = time.Now().Add(-time.Millisecond)
	heartbeat(t, c)
	require.Equal(t, hub.ViewerListItem{UserID: "u-7", User: "Luna", UserLevel: 3}, waitForViewer(t, c, "Luna"))
	require.True(t, c.profileRetry.IsZero())

	c.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: "hi"})
	require.Len(t, prod.snapshot(), 1)
	require.Equal(t, "Luna", prod.snapshot()[0].Username)
	require.Equal(t, 3, prod.snapshot()[0].UserLevel)
}

// The identity was resolved once per connection, so after a rename (or with
// two tabs that resolved at different times) chat kept the old name.
func TestConn_ChatUsesIdentityAsOfSending(t *testing.T) {
	res := &scriptedProfiles{}
	res.set(profile.Profile{Name: "Luna", Level: 3}, true)
	prod := &fakeProducer{}
	c := newTestConn(auth.Identity{UserID: "u-7"}, prod)
	c.profiles = res
	joinTestRoom(t, c)
	waitForViewer(t, c, "Luna")

	res.set(profile.Profile{Name: "Luna Moon", Level: 4}, true)
	c.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: "hi"})
	require.Len(t, prod.snapshot(), 1)
	require.Equal(t, "Luna Moon", prod.snapshot()[0].Username)
	require.Equal(t, 4, prod.snapshot()[0].UserLevel)
	require.Equal(t, 4, waitForViewer(t, c, "Luna Moon").UserLevel, "the viewer list follows")

	// A fallback served during an outage never replaces the entry.
	res.set(profile.Profile{}, false)
	c.dispatchInbound(context.Background(), hub.Inbound{Type: "chat", Text: "still here"})
	requireNoViewer(t, c, "")
}

// viewer_profile, which the client sends after the user edits their profile,
// went through the 30s cache, so a rename didn't show. It now asks the
// resolver to refetch (the resolver rate-limits that per user).
func TestConn_ViewerProfileRefreshesIdentity(t *testing.T) {
	res := &scriptedProfiles{}
	res.set(profile.Profile{Name: "Luna"}, true)
	c := newTestConn(auth.Identity{UserID: "u-7"}, &fakeProducer{})
	c.profiles = res
	joinTestRoom(t, c)

	res.set(profile.Profile{Name: "Luna Moon"}, true)
	require.True(t, c.handleFrame(context.Background(), []byte(`{"type":"viewer_profile"}`)))
	waitForViewer(t, c, "Luna Moon")
	_, refreshes := res.counts()
	require.Equal(t, 1, refreshes)
}
