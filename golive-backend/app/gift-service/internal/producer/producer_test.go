package producer

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
)

func TestEventPayloadAddsOutboxID(t *testing.T) {
	msg := &model.LocalMessage{ID: 42, Payload: `{"type":"gift","id":"gift-1","totalCoin":500}`}
	var got map[string]any
	require.NoError(t, json.Unmarshal(EventPayload(msg), &got))
	require.Equal(t, map[string]any{"type": "gift", "id": "gift-1", "totalCoin": float64(500), "eventId": float64(42)}, got)

	require.Equal(t, `[1,2]`, string(EventPayload(&model.LocalMessage{ID: 1, Payload: `[1,2]`})))
	require.Equal(t, `{"a":1}`, string(EventPayload(&model.LocalMessage{Payload: `{"a":1}`})))
}

func TestRedisFanoutPublishesEventID(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()
	sub := rdb.Subscribe(ctx, "room:r1")
	t.Cleanup(func() { _ = sub.Close() })
	_, err := sub.Receive(ctx)
	require.NoError(t, err)

	require.NoError(t, NewRedisFanout(rdb).Publish(ctx, &model.LocalMessage{ID: 7, RoomID: "r1", Payload: `{"type":"gift"}`}))
	select {
	case m := <-sub.Channel():
		require.JSONEq(t, `{"type":"gift","eventId":7}`, m.Payload)
	case <-time.After(5 * time.Second):
		t.Fatal("no message published")
	}
}
