//go:build loadtest

// Standalone load generator. Produces N events/sec to the danmu topic and
// reports end-to-end p50/p95/p99 by subscribing to "room:<id>" on Redis and
// timing each round-trip.
//
//	go run -tags loadtest ./app/chat-service/loadtest \
//	    -brokers 127.0.0.1:9092 -redis 127.0.0.1:6379 \
//	    -qps 5000 -rooms 50 -duration 60s
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v9"
	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
)

func main() {
	brokers := flag.String("brokers", "127.0.0.1:9092", "kafka brokers, comma separated")
	redisAddr := flag.String("redis", "127.0.0.1:6379", "redis addr")
	topic := flag.String("topic", "danmu", "kafka topic")
	qps := flag.Int("qps", 5000, "events per second")
	rooms := flag.Int("rooms", 50, "number of rooms")
	dur := flag.Duration("duration", 60*time.Second, "test duration")
	flag.Parse()

	cl, err := kgo.NewClient(
		kgo.SeedBrokers(splitCSV(*brokers)...),
		kgo.DefaultProduceTopic(*topic),
		kgo.ProducerLinger(2*time.Millisecond),
	)
	if err != nil {
		panic(err)
	}
	defer cl.Close()

	rdb := redis.NewClient(&redis.Options{Addr: *redisAddr})
	defer rdb.Close()

	roomIDs := make([]string, *rooms)
	for i := range roomIDs {
		roomIDs[i] = fmt.Sprintf("room-%d", i)
	}

	// Subscribe to all room channels, record arrival time per requestId.
	ctx, cancel := context.WithTimeout(context.Background(), *dur+10*time.Second)
	defer cancel()

	subChannels := make([]string, len(roomIDs))
	for i, r := range roomIDs {
		subChannels[i] = "room:" + r
	}
	ps := rdb.Subscribe(ctx, subChannels...)
	defer ps.Close()

	var (
		mu       sync.Mutex
		sentAt   = make(map[string]time.Time, *qps**rooms)
		latencies []time.Duration
		dropped  atomic.Int64
		received atomic.Int64
	)

	go func() {
		ch := ps.Channel()
		for msg := range ch {
			var probe struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &probe); err != nil {
				continue
			}
			mu.Lock()
			t0, ok := sentAt[probe.ID]
			if ok {
				latencies = append(latencies, time.Since(t0))
				delete(sentAt, probe.ID)
			}
			mu.Unlock()
			received.Add(1)
		}
	}()

	interval := time.Second / time.Duration(*qps)
	t := time.NewTicker(interval)
	defer t.Stop()
	deadline := time.After(*dur)
	start := time.Now()

	var sent atomic.Int64
loop:
	for {
		select {
		case <-deadline:
			break loop
		case <-t.C:
			id := uuid.NewString()
			room := roomIDs[rand.Intn(len(roomIDs))]
			ev := map[string]any{
				"roomId":   room,
				"userId":   fmt.Sprintf("u-%d", rand.Intn(10000)),
				"username": "tester",
				"text":     "hello hello",
				"ts":       time.Now().UnixMilli(),
				// Tag each event with a request id; the consumer keeps a NEW id
				// in `id` after persistence, so we cannot end-to-end correlate
				// without a transparent passthrough field. Trick: piggyback on
				// `text` so the published msg keeps the marker.
			}
			ev["text"] = "loadtest:" + id
			body, _ := json.Marshal(ev)
			mu.Lock()
			sentAt[id] = time.Now()
			mu.Unlock()
			cl.Produce(ctx, &kgo.Record{Topic: *topic, Key: []byte(room), Value: body},
				func(_ *kgo.Record, err error) {
					if err != nil {
						dropped.Add(1)
					}
				})
			sent.Add(1)
		}
	}

	// drain a bit
	time.Sleep(3 * time.Second)

	mu.Lock()
	stats := append([]time.Duration(nil), latencies...)
	mu.Unlock()
	sort.Slice(stats, func(i, j int) bool { return stats[i] < stats[j] })

	elapsed := time.Since(start)
	fmt.Printf("\nsent=%d recv=%d drop=%d elapsed=%s actual_qps=%.0f\n",
		sent.Load(), received.Load(), dropped.Load(), elapsed,
		float64(sent.Load())/elapsed.Seconds())
	if n := len(stats); n > 0 {
		fmt.Printf("e2e latency p50=%s p95=%s p99=%s max=%s\n",
			stats[n/2], stats[n*95/100], stats[n*99/100], stats[n-1])
	}
}

func splitCSV(s string) []string {
	out := []string{}
	cur := ""
	for _, r := range s {
		if r == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
