package service_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
	"github.com/qingwenwen777/golive/app/gift-service/internal/repo"
	"github.com/qingwenwen777/golive/app/gift-service/internal/service"
)

// flakyProducer fails the first `failN` calls, then succeeds.
type flakyProducer struct {
	mu        sync.Mutex
	failN     int
	calls     atomic.Int32
	published []*model.LocalMessage
}

func (f *flakyProducer) Publish(_ context.Context, m *model.LocalMessage) error {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failN > 0 {
		f.failN--
		return errors.New("kafka down")
	}
	f.published = append(f.published, m)
	return nil
}
func (f *flakyProducer) Close() error { return nil }

func TestOutbox_RetryThenSent(t *testing.T) {
	db := newTestDB(t, 0)
	or := repo.NewOutboxRepo(db)

	// seed one pending row in the past
	row := &model.LocalMessage{
		BizID:   "gift-1",
		Topic:   model.OutboxTopicGift,
		Payload: `{"type":"gift"}`,
		Status:  model.OutboxStatusPending,
		NextAt:  time.Now().Add(-time.Second),
	}
	require.NoError(t, db.Create(row).Error)

	prod := &flakyProducer{failN: 2}
	svc := service.NewOutboxService(or, prod, service.OutboxConfig{
		PollInterval: time.Hour, // we drive draining manually
		BatchSize:    10,
		MaxRetries:   5,
		BaseBackoff:  1 * time.Millisecond,
	})

	// 1st drain: producer fails → row should be rescheduled with retries=1
	require.Equal(t, 1, svc.DrainOnce(context.Background()))
	var got model.LocalMessage
	require.NoError(t, db.First(&got, row.ID).Error)
	require.Equal(t, model.OutboxStatusPending, got.Status)
	require.Equal(t, 1, got.Retries)

	// Roll next_at back so claim picks it up again.
	require.NoError(t, db.Model(&got).Update("next_at", time.Now().Add(-time.Second)).Error)

	// 2nd drain: still failing, retries=2
	require.Equal(t, 1, svc.DrainOnce(context.Background()))
	require.NoError(t, db.First(&got, row.ID).Error)
	require.Equal(t, 2, got.Retries)
	require.Equal(t, model.OutboxStatusPending, got.Status)

	// Force eligible again. 3rd drain: producer now succeeds → status=sent.
	require.NoError(t, db.Model(&got).Update("next_at", time.Now().Add(-time.Second)).Error)
	require.Equal(t, 1, svc.DrainOnce(context.Background()))
	require.NoError(t, db.First(&got, row.ID).Error)
	require.Equal(t, model.OutboxStatusSent, got.Status)
	require.Equal(t, 1, len(prod.published))
}

func TestOutbox_DeadAfterMaxRetries(t *testing.T) {
	db := newTestDB(t, 0)
	or := repo.NewOutboxRepo(db)

	row := &model.LocalMessage{
		BizID: "g-2", Topic: model.OutboxTopicGift, Payload: "{}",
		Status: model.OutboxStatusPending, NextAt: time.Now().Add(-time.Second),
	}
	require.NoError(t, db.Create(row).Error)

	prod := &flakyProducer{failN: 1000}
	svc := service.NewOutboxService(or, prod, service.OutboxConfig{
		PollInterval: time.Hour, BatchSize: 10,
		MaxRetries: 3, BaseBackoff: 1 * time.Millisecond,
	})

	for i := 0; i < 3; i++ {
		require.NoError(t, db.Model(&model.LocalMessage{}).
			Where("id = ?", row.ID).
			Update("next_at", time.Now().Add(-time.Second)).Error)
		svc.DrainOnce(context.Background())
	}
	var got model.LocalMessage
	require.NoError(t, db.First(&got, row.ID).Error)
	require.Equal(t, model.OutboxStatusDead, got.Status, "must be marked dead after MaxRetries")
}

func TestOutbox_ClaimSkipsFutureNextAt(t *testing.T) {
	db := newTestDB(t, 0)
	or := repo.NewOutboxRepo(db)

	require.NoError(t, db.Create(&model.LocalMessage{
		BizID: "future", Topic: "gift", Payload: "{}",
		Status: model.OutboxStatusPending, NextAt: time.Now().Add(time.Hour),
	}).Error)
	require.NoError(t, db.Create(&model.LocalMessage{
		BizID: "ready", Topic: "gift", Payload: "{}",
		Status: model.OutboxStatusPending, NextAt: time.Now().Add(-time.Second),
	}).Error)

	rows, err := or.Claim(context.Background(), 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "ready", rows[0].BizID)
}

func TestOutbox_DoesNotPickAlreadySent(t *testing.T) {
	db := newTestDB(t, 0)
	or := repo.NewOutboxRepo(db)
	require.NoError(t, db.Create(&model.LocalMessage{
		BizID: "x", Topic: "gift", Payload: "{}",
		Status: model.OutboxStatusSent, NextAt: time.Now().Add(-time.Second),
	}).Error)
	rows, err := or.Claim(context.Background(), 10, time.Minute)
	require.NoError(t, err)
	require.Empty(t, rows)
}

// gatedProducer blocks its first Publish until release is closed.
type gatedProducer struct {
	mu        sync.Mutex
	started   chan struct{}
	release   chan struct{}
	published map[uint64]int
}

func (g *gatedProducer) Publish(_ context.Context, m *model.LocalMessage) error {
	g.mu.Lock()
	first := len(g.published) == 0
	g.mu.Unlock()
	if first {
		close(g.started)
		<-g.release
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.published[m.ID]++
	return nil
}
func (g *gatedProducer) Close() error { return nil }

// A batch that outlives half its claim hold stops publishing, so rows another
// worker can re-claim once the hold lapses are not published twice.
func TestOutbox_SlowBatchStopsBeforeHoldLapses(t *testing.T) {
	db := newTestDB(t, 0)
	or := repo.NewOutboxRepo(db)
	for _, biz := range []string{"a", "b", "c"} {
		require.NoError(t, db.Create(&model.LocalMessage{
			BizID: biz, Topic: "gift", Payload: "{}",
			Status: model.OutboxStatusPending, NextAt: time.Now().Add(-time.Second),
		}).Error)
	}
	const hold = time.Second
	prod := &gatedProducer{started: make(chan struct{}), release: make(chan struct{}), published: map[uint64]int{}}
	svc := service.NewOutboxService(or, prod, service.OutboxConfig{BatchSize: 10, ClaimHold: hold})

	claimedBy := time.Now()
	drained := make(chan int, 1)
	go func() { drained <- svc.DrainOnce(context.Background()) }()
	<-prod.started
	time.Sleep(hold/2 + 50*time.Millisecond)
	close(prod.release)
	require.Equal(t, 3, <-drained)
	require.Len(t, prod.published, 1, "publishing must stop once half the hold is used")

	var pending int64
	require.NoError(t, db.Model(&model.LocalMessage{}).Where("status = ?", model.OutboxStatusPending).Count(&pending).Error)
	require.EqualValues(t, 2, pending)
	require.Zero(t, svc.DrainOnce(context.Background()), "held rows are not re-claimed early")

	// The unpublished rows come back once the hold has lapsed.
	time.Sleep(time.Until(claimedBy.Add(hold + 50*time.Millisecond)))
	require.Equal(t, 2, svc.DrainOnce(context.Background()))
	require.Len(t, prod.published, 3)
	for id, n := range prod.published {
		require.Equalf(t, 1, n, "row %d published %d times", id, n)
	}
}
