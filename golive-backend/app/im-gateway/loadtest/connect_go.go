//go:build loadtest
// +build loadtest

// connect_go is a Go-native load tester (use when k6 is unavailable). Run:
//
//	go run -tags loadtest ./app/im-gateway/loadtest \
//	    -url ws://localhost:8081/ws -conns 50000 -rooms 20 -hold 60s
//
// It opens N connections in parallel, each in a goroutine, and prints
// handshake p50/p95/p99 plus message receive rate.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	wsURL := flag.String("url", "ws://localhost:8081/ws", "ws base url")
	conns := flag.Int("conns", 50000, "total connections")
	rooms := flag.Int("rooms", 20, "number of rooms to spread across")
	hold := flag.Duration("hold", 60*time.Second, "how long to keep each conn open")
	dialConcurrency := flag.Int("dial", 256, "concurrent dials")
	flag.Parse()

	fmt.Printf("dialing %d connections across %d rooms, holding %s\n", *conns, *rooms, *hold)

	var (
		dialed   atomic.Int64
		failed   atomic.Int64
		recvd    atomic.Int64
		hsMu     sync.Mutex
		hsTimes  = make([]time.Duration, 0, *conns)
	)

	sem := make(chan struct{}, *dialConcurrency)
	wg := sync.WaitGroup{}
	wg.Add(*conns)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	start := time.Now()
	for i := 0; i < *conns; i++ {
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()

			u, _ := url.Parse(*wsURL)
			q := u.Query()
			q.Set("roomId", fmt.Sprintf("room-%d", rand.Intn(*rooms)))
			u.RawQuery = q.Encode()

			t0 := time.Now()
			c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			if err != nil {
				failed.Add(1)
				return
			}
			elapsed := time.Since(t0)
			hsMu.Lock()
			hsTimes = append(hsTimes, elapsed)
			hsMu.Unlock()
			dialed.Add(1)

			go func() {
				for {
					_, _, err := c.ReadMessage()
					if err != nil {
						return
					}
					recvd.Add(1)
				}
			}()

			ticker := time.NewTicker(25 * time.Second)
			defer ticker.Stop()
			deadline := time.After(*hold)
			for {
				select {
				case <-deadline:
					_ = c.Close()
					return
				case <-ctx.Done():
					_ = c.Close()
					return
				case <-ticker.C:
					_ = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"heartbeat"}`))
				}
			}
		}(i)

		if i%5000 == 0 && i > 0 {
			fmt.Printf("  dialed=%d failed=%d elapsed=%s\n", dialed.Load(), failed.Load(), time.Since(start))
		}
	}

	wg.Wait()

	fmt.Printf("\ndone. dialed=%d failed=%d recv=%d total_time=%s\n",
		dialed.Load(), failed.Load(), recvd.Load(), time.Since(start))

	hsMu.Lock()
	sort.Slice(hsTimes, func(i, j int) bool { return hsTimes[i] < hsTimes[j] })
	hsMu.Unlock()
	if n := len(hsTimes); n > 0 {
		fmt.Printf("handshake p50=%s p95=%s p99=%s\n",
			hsTimes[n/2], hsTimes[n*95/100], hsTimes[n*99/100])
	}
}
