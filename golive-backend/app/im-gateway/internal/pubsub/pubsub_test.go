package pubsub

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
)

func newTestBroker(t *testing.T) (*RedisBroker, *miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	b := NewRedis(rdb)
	t.Cleanup(func() { _ = b.Close() })
	return b, mr, rdb
}

func receive(t *testing.T, s Subscription) string {
	t.Helper()
	select {
	case p := <-s.Channel():
		return string(p)
	case <-time.After(2 * time.Second):
		t.Fatal("no message")
		return ""
	}
}

// Each room used to open its own Redis connection, so one client joining
// many distinct roomIds could exhaust Redis maxclients for every service.
func TestRedisBroker_SharesOneConnectionAcrossRooms(t *testing.T) {
	b, mr, rdb := newTestBroker(t)
	ctx := context.Background()
	require.NoError(t, rdb.Ping(ctx).Err())
	baseline := mr.CurrentConnectionCount()

	subs := make([]Subscription, 0, 50)
	for i := 0; i < 50; i++ {
		s, err := b.Subscribe(ctx, RoomChannel(fmt.Sprintf("r%d", i)))
		require.NoError(t, err)
		subs = append(subs, s)
	}
	require.Eventually(t, func() bool { return len(mr.PubSubChannels("room:*")) == 50 }, 2*time.Second, 5*time.Millisecond)
	require.LessOrEqual(t, mr.CurrentConnectionCount(), baseline+1)

	require.NoError(t, b.Publish(ctx, RoomChannel("r7"), []byte("hello-7")))
	require.Equal(t, "hello-7", receive(t, subs[7]))
	for i, s := range subs {
		if i == 7 {
			continue
		}
		select {
		case p := <-s.Channel():
			t.Fatalf("room %d received %q meant for r7", i, p)
		default:
		}
	}
}

func TestRedisBroker_UnsubscribesOnLastClose(t *testing.T) {
	b, mr, _ := newTestBroker(t)
	ctx := context.Background()
	ch := RoomChannel("R1")

	a, err := b.Subscribe(ctx, ch)
	require.NoError(t, err)
	c, err := b.Subscribe(ctx, ch)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return mr.PubSubNumSub(ch)[ch] == 1 }, 2*time.Second, 5*time.Millisecond)

	require.NoError(t, b.Publish(ctx, ch, []byte("both")))
	require.Equal(t, "both", receive(t, a))
	require.Equal(t, "both", receive(t, c))

	require.NoError(t, a.Close())
	require.NoError(t, a.Close(), "Close is idempotent")
	_, open := <-a.Channel()
	require.False(t, open, "closed subscription's channel is closed")

	require.NoError(t, b.Publish(ctx, ch, []byte("still")))
	require.Equal(t, "still", receive(t, c))
	require.Equal(t, 1, mr.PubSubNumSub(ch)[ch], "remaining local subscriber keeps the channel")

	require.NoError(t, c.Close())
	require.Eventually(t, func() bool { return mr.PubSubNumSub(ch)[ch] == 0 }, 2*time.Second, 5*time.Millisecond)

	// Re-subscribing after the last close works.
	d, err := b.Subscribe(ctx, ch)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return mr.PubSubNumSub(ch)[ch] == 1 }, 2*time.Second, 5*time.Millisecond)
	require.NoError(t, b.Publish(ctx, ch, []byte("again")))
	require.Equal(t, "again", receive(t, d))
}
