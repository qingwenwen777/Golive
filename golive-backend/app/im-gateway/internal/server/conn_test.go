package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/im-gateway/internal/auth"
)

// newWSPair returns the server side of a real websocket connection.
func newWSPair(t *testing.T) *websocket.Conn {
	t.Helper()
	serverSide := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		require.NoError(t, err)
		serverSide <- ws
	}))
	t.Cleanup(srv.Close)

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return <-serverSide
}

// Fanout goroutines call Send while writePump or an evicting fanout calls
// Close; that used to panic with "send on closed channel" and kill the process.
func TestConnSendConcurrentWithCloseDoesNotPanic(t *testing.T) {
	for i := 0; i < 200; i++ {
		c := newConn(newWSPair(t), "R1", "", auth.Identity{}, Deps{}, WSConfig{SendBuffer: 1})

		start := make(chan struct{})
		closed := make(chan struct{})
		var wg sync.WaitGroup
		for s := 0; s < 8; s++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				// Keep sending until Close has finished so sends straddle it.
				for {
					c.Send([]byte("x"))
					select {
					case <-closed:
						return
					default:
					}
				}
			}()
		}
		go func() {
			<-start
			c.Close()
			close(closed)
		}()
		close(start)
		wg.Wait()

		require.False(t, c.Send([]byte("after close")))
	}
}
