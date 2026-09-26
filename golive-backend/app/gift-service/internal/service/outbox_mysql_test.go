package service_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
	"github.com/qingwenwen777/golive/app/gift-service/internal/service"
)

func insertMySQLOutboxRows(t *testing.T, db *gorm.DB, n int) {
	t.Helper()
	past := time.Now().Add(-time.Minute)
	for i := range n {
		require.NoError(t, db.Create(&model.LocalMessage{
			BizID:   fmt.Sprintf("biz-%d", i),
			RoomID:  "r1",
			Topic:   model.OutboxTopicGift,
			Payload: `{"type":"gift"}`,
			Status:  model.OutboxStatusPending,
			NextAt:  past,
		}).Error)
	}
}

// A Claim that runs while another is between its SELECT and its UPDATE must
// not hand out the rows the first one is claiming.
func TestMySQLOutbox_ConcurrentClaimsAreDisjoint(t *testing.T) {
	db, _ := newMySQLTestDB(t)
	insertMySQLOutboxRows(t, db, 5)
	outbox := repo.NewOutboxRepo(db)
	ctx := context.Background()

	// Pause the first Claim right after its SELECT of local_messages.
	var armed atomic.Bool
	armed.Store(true)
	reached := make(chan struct{})
	release := make(chan struct{})
	unblock := closeOnce(release)
	t.Cleanup(unblock)
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:pause_claim", func(tx *gorm.DB) {
		if tx.Statement.Table == "local_messages" && armed.CompareAndSwap(true, false) {
			close(reached)
			<-release
		}
	}))
	type claimed struct {
		rows []model.LocalMessage
		err  error
	}
	first := make(chan claimed, 1)
	go func() {
		rows, err := outbox.Claim(ctx, 100, time.Minute)
		first <- claimed{rows, err}
	}()
	<-reached

	secondDone := make(chan struct{})
	var second claimed
	go func() {
		defer close(secondDone)
		second.rows, second.err = outbox.Claim(ctx, 100, time.Minute)
	}()
	// The second Claim either finishes (skipping the locked rows) or blocks
	// behind the first; in both cases let the first one go on.
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
	}
	unblock()
	a := <-first
	<-secondDone
	require.NoError(t, a.err)
	require.NoError(t, second.err)

	seen := map[uint64]int{}
	for _, m := range append(a.rows, second.rows...) {
		seen[m.ID]++
	}
	require.Len(t, seen, 5, "every row is claimed")
	for id, n := range seen {
		require.Equalf(t, 1, n, "row %d claimed by more than one worker", id)
	}
}

// countingProducer records how many times each outbox row was published.
type countingProducer struct {
	mu    sync.Mutex
	count map[uint64]int
	delay time.Duration
}

func (p *countingProducer) Publish(_ context.Context, m *model.LocalMessage) error {
	time.Sleep(p.delay)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.count[m.ID]++
	return nil
}

func (p *countingProducer) Close() error { return nil }

// Several gift-service replicas draining the same outbox must publish each
// row exactly once.
func TestMySQLOutbox_ParallelDrainPublishesOnce(t *testing.T) {
	db, _ := newMySQLTestDB(t)
	const rows = 200
	insertMySQLOutboxRows(t, db, rows)
	prod := &countingProducer{count: map[uint64]int{}}
	ctx := context.Background()

	var wg sync.WaitGroup
	for range 4 {
		svc := service.NewOutboxService(repo.NewOutboxRepo(db), prod, service.OutboxConfig{BatchSize: 20})
		wg.Go(func() {
			for svc.DrainOnce(ctx) > 0 {
			}
		})
	}
	wg.Wait()

	require.Len(t, prod.count, rows)
	for id, n := range prod.count {
		require.Equalf(t, 1, n, "row %d published %d times", id, n)
	}
	var sent int64
	require.NoError(t, db.Model(&model.LocalMessage{}).Where("status = ?", model.OutboxStatusSent).Count(&sent).Error)
	require.EqualValues(t, rows, sent)
}
