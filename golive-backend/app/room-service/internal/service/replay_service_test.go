package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

func TestWaitForStableFileWaitsThroughGrowth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording.flv")
	require.NoError(t, os.WriteFile(path, []byte("a"), 0o644))

	growthDone := make(chan struct{})
	go func() {
		defer close(growthDone)
		for i := 0; i < 3; i++ {
			time.Sleep(8 * time.Millisecond)
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
			if err != nil {
				return
			}
			_, _ = file.WriteString("a")
			_ = file.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	info, err := waitForStableFile(ctx, path, 5*time.Millisecond, 4)
	require.NoError(t, err)
	<-growthDone
	require.Equal(t, int64(4), info.Size())
}

func TestFindRecordingIgnoresTemporaryRecording(t *testing.T) {
	dir := t.TempDir()
	key := "lk_temp"
	require.NoError(t, os.WriteFile(filepath.Join(dir, key+".flv.tmp"), []byte("temp"), 0o644))

	svc := &ReplayService{recordDir: dir}
	_, err := svc.findRecording(key)
	require.ErrorIs(t, err, errRecordingNotFound)

	finalPath := filepath.Join(dir, key+".flv")
	require.NoError(t, os.WriteFile(finalPath, []byte("final"), 0o644))
	path, err := svc.findRecording(key)
	require.NoError(t, err)
	require.Equal(t, finalPath, path)
}

func TestWaitForUploadableRecordingWaitsForTemporaryRename(t *testing.T) {
	dir := t.TempDir()
	key := "lk_rename"
	tmpPath := filepath.Join(dir, key+".flv.tmp")
	finalPath := filepath.Join(dir, key+".flv")
	require.NoError(t, os.WriteFile(tmpPath, []byte("recording"), 0o644))

	go func() {
		time.Sleep(15 * time.Millisecond)
		_ = os.Rename(tmpPath, finalPath)
	}()

	svc := &ReplayService{recordDir: dir}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	path, err := svc.waitForUploadableRecordingWith(ctx, key, 5*time.Millisecond, 2)
	require.NoError(t, err)
	require.Equal(t, finalPath, path)
}

func TestUploadVideoUsesStableContentLengthWhenFileGrows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording.flv")
	require.NoError(t, os.WriteFile(path, []byte("abc"), 0o644))

	type uploadResult struct {
		bodyLength    int
		contentLength int64
		err           error
	}
	resultCh := make(chan uploadResult, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			resultCh <- uploadResult{err: fmt.Errorf("unexpected method %s", r.Method)}
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			resultCh <- uploadResult{err: err}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, err = file.WriteString("def")
		closeErr := file.Close()
		if err != nil {
			resultCh <- uploadResult{err: err}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if closeErr != nil {
			resultCh <- uploadResult{err: closeErr}
			http.Error(w, closeErr.Error(), http.StatusInternalServerError)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			resultCh <- uploadResult{err: err}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resultCh <- uploadResult{bodyLength: len(body), contentLength: r.ContentLength}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewBunnyClient(server.URL, "test-key", time.Second)
	require.NoError(t, client.UploadVideo(context.Background(), "lib", "video", path))

	result := <-resultCh
	require.NoError(t, result.err)
	require.Equal(t, int64(3), result.contentLength)
	require.Equal(t, 3, result.bodyLength)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, int64(6), info.Size())
}

// fakeBunny serves Bunny Stream's create, upload and delete video calls.
// onUpload runs while the upload request is in flight; failUploads makes
// that many uploads fail. uploads holds the bodies of the others.
type fakeBunny struct {
	mu          sync.Mutex
	onUpload    func() error
	errs        []error
	deleted     []string
	created     int
	failUploads int
	uploads     [][]byte
}

func (b *fakeBunny) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/library/lib/videos":
		b.mu.Lock()
		b.created++
		b.mu.Unlock()
		fmt.Fprint(w, `{"guid":"video-1"}`)
	case r.Method == http.MethodPut && r.URL.Path == "/library/lib/videos/video-1":
		body, _ := io.ReadAll(r.Body)
		b.mu.Lock()
		onUpload := b.onUpload
		fail := b.failUploads > 0
		if fail {
			b.failUploads--
		} else {
			b.uploads = append(b.uploads, body)
		}
		b.mu.Unlock()
		if fail {
			http.Error(w, "upload failed", http.StatusBadGateway)
			return
		}
		if onUpload != nil {
			if err := onUpload(); err != nil {
				b.mu.Lock()
				b.errs = append(b.errs, err)
				b.mu.Unlock()
			}
		}
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/library/lib/videos/"):
		b.mu.Lock()
		b.deleted = append(b.deleted, strings.TrimPrefix(r.URL.Path, "/library/lib/videos/"))
		b.mu.Unlock()
	default:
		http.NotFound(w, r)
	}
}

func newReplayUploadTest(t *testing.T) (*ReplayService, *repo.RoomRepo, *fakeBunny, model.Room, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// Keep one connection so the in-memory database is shared with the fake
	// Bunny handler, which deletes the replay mid-upload.
	sqlDB.SetMaxOpenConns(1)
	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())

	bunny := &fakeBunny{}
	server := httptest.NewServer(bunny)
	t.Cleanup(server.Close)

	dir := t.TempDir()
	svc := NewReplayService(rooms, nil, ReplayConfig{
		RecordDir:      dir,
		BunnyLibraryID: "lib",
		BunnyAPIKey:    "key",
		BunnyAPIBase:   server.URL,
	})

	startedAt := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(time.Hour)
	room := model.Room{
		ID:                  "room-replay",
		Title:               "Replay",
		Channel:             "Creator",
		ChannelID:           "ch-owner-replay",
		OwnerID:             "owner-replay",
		Status:              model.StatusEnded,
		StartedAt:           startedAt,
		EndedAt:             &endedAt,
		ReplayUploadEnabled: true,
		ReplayStatus:        model.ReplayStatusPending,
		ReplayVisibility:    model.PostVisibilityPublic,
	}
	require.NoError(t, rooms.Upsert(context.Background(), &room))

	recordPath := filepath.Join(dir, room.ID+".flv")
	require.NoError(t, os.WriteFile(recordPath, []byte("flv"), 0o644))
	return svc, rooms, bunny, room, recordPath
}

func TestUploadRecordingPublishesReplay(t *testing.T) {
	ctx := context.Background()
	svc, rooms, bunny, room, recordPath := newReplayUploadTest(t)

	svc.uploadRecording(ctx, room, recordPath)

	got, err := rooms.GetByID(ctx, room.ID)
	require.NoError(t, err)
	require.Equal(t, model.ReplayStatusReady, got.ReplayStatus)
	require.Equal(t, "video-1", got.ReplayBunnyVideoID)
	canView, err := svc.CanView(ctx, *got, "")
	require.NoError(t, err)
	require.True(t, canView)
	require.Empty(t, bunny.deleted)
	require.NoFileExists(t, recordPath)
}

func TestUploadRecordingDoesNotPublishReplayDeletedMidUpload(t *testing.T) {
	ctx := context.Background()
	svc, rooms, bunny, room, recordPath := newReplayUploadTest(t)
	bunny.onUpload = func() error {
		return svc.DeleteReplay(ctx, room.OwnerID, room.ID)
	}

	svc.uploadRecording(ctx, room, recordPath)

	bunny.mu.Lock()
	defer bunny.mu.Unlock()
	require.Empty(t, bunny.errs)
	got, err := rooms.GetByID(ctx, room.ID)
	require.NoError(t, err)
	require.Equal(t, model.ReplayStatusDeleted, got.ReplayStatus)
	require.Empty(t, got.ReplayBunnyVideoID)
	require.NotNil(t, got.ReplayDeletedAt)
	canView, err := svc.CanView(ctx, *got, "")
	require.NoError(t, err)
	require.False(t, canView)
	require.Equal(t, []string{"video-1"}, bunny.deleted)
	require.NoFileExists(t, recordPath)
}

// fastRecordingWaits drops the delays before a recording is looked for, so
// upload and cleanup goroutines finish quickly in tests.
func fastRecordingWaits(svc *ReplayService) {
	svc.settleDelay = 0
	svc.stableInterval = time.Millisecond
}

func (b *fakeBunny) createdCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.created
}

func (b *fakeBunny) uploaded() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.uploads)
}

func (b *fakeBunny) deletedVideos() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.deleted)
}

// addFailedReplay stores an ended room like room whose replay upload failed,
// with a recording on disk last written at recordedAt, and returns its path.
func addFailedReplay(t *testing.T, svc *ReplayService, rooms *repo.RoomRepo, room model.Room, recordedAt time.Time) string {
	t.Helper()
	endedAt := recordedAt
	room.Title, room.OwnerID = room.ID, "owner-"+room.ID
	room.Status, room.EndedAt, room.StartedAt = model.StatusEnded, &endedAt, endedAt.Add(-time.Hour)
	if room.ReplayStatus == "" {
		room.ReplayStatus, room.ReplayError = model.ReplayStatusFailed, "bunny stream request failed"
	}
	require.NoError(t, rooms.Upsert(context.Background(), &room))
	path := filepath.Join(svc.recordDir, svc.recordingStreamName(room)+".flv")
	require.NoError(t, os.WriteFile(path, []byte("flv"), 0o644))
	require.NoError(t, os.Chtimes(path, recordedAt, recordedAt))
	return path
}

// uploadingDuringUpload makes bunny note which of the rooms ids are
// uploading while it receives a video, and returns them in order.
func uploadingDuringUpload(t *testing.T, bunny *fakeBunny, rooms *repo.RoomRepo, ids ...string) func() []string {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	bunny.onUpload = func() error {
		mu.Lock()
		defer mu.Unlock()
		for _, id := range ids {
			got, err := rooms.GetByID(context.Background(), id)
			if err != nil {
				return err
			}
			if got.ReplayStatus == model.ReplayStatusUploading {
				seen = append(seen, id)
			}
		}
		return nil
	}
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(seen)
	}
}

func TestRecoverInterruptedUploadsRetriesFailedUploads(t *testing.T) {
	ctx := context.Background()
	svc, rooms, bunny, room, recordPath := newReplayUploadTest(t)
	fastRecordingWaits(svc)
	uploading := uploadingDuringUpload(t, bunny, rooms, room.ID, "room-scheduled")
	// Failed before retries were counted: the old code retried it on every
	// restart, so the first start after the deploy does too.
	require.NoError(t, rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusFailed, "bunny stream request failed"))
	now := time.Now()
	later, failedAt := now.Add(time.Hour), now.Add(-time.Hour)
	// A restart retries right away, not when the backoff ends.
	scheduled := addFailedReplay(t, svc, rooms, model.Room{
		ID: "room-scheduled", ReplayUploadEnabled: true, ReplayAttempts: 2, ReplayRetryAt: &later, ReplayFailedAt: &failedAt,
	}, now.Add(-2*time.Hour))
	notRetried := map[string]string{
		"room-gave-up": addFailedReplay(t, svc, rooms, model.Room{
			ID: "room-gave-up", ReplayUploadEnabled: true, ReplayAttempts: defaultReplayUploadAttempts, ReplayFailedAt: &failedAt,
		}, now.Add(-2*time.Hour)),
		"room-upload-off": addFailedReplay(t, svc, rooms, model.Room{ID: "room-upload-off"}, now.Add(-2*time.Hour)),
		"room-deleted": addFailedReplay(t, svc, rooms, model.Room{
			ID: "room-deleted", ReplayStatus: model.ReplayStatusDeleted, ReplayDeletedAt: &failedAt,
		}, now.Add(-2*time.Hour)),
	}

	svc.RecoverInterruptedUploads(ctx)

	for _, id := range []string{room.ID, "room-scheduled"} {
		got, err := rooms.GetByID(ctx, id)
		require.NoError(t, err)
		require.Equal(t, model.ReplayStatusReady, got.ReplayStatus, id)
		require.Empty(t, got.ReplayError, id)
	}
	require.Equal(t, []string{room.ID, "room-scheduled"}, uploading(), "each retry passes through uploading")
	require.Equal(t, 2, bunny.createdCount())
	require.NoFileExists(t, recordPath)
	require.NoFileExists(t, scheduled)
	for id, path := range notRetried {
		got, err := rooms.GetByID(ctx, id)
		require.NoError(t, err)
		require.NotEqual(t, model.ReplayStatusReady, got.ReplayStatus, id)
		require.FileExists(t, path, id)
	}
}

func TestFailedUploadIsRetriedWithBackoffUntilItGivesUp(t *testing.T) {
	ctx := context.Background()
	svc, rooms, bunny, room, recordPath := newReplayUploadTest(t)
	fastRecordingWaits(svc)
	svc.uploadAttempts = 3
	now := time.Date(2026, 5, 4, 14, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	bunny.failUploads = 3
	load := func() *model.Room {
		got, err := rooms.GetByID(ctx, room.ID)
		require.NoError(t, err)
		return got
	}

	svc.runUpload(room)

	got := load()
	require.Equal(t, model.ReplayStatusFailed, got.ReplayStatus)
	require.Equal(t, 1, got.ReplayAttempts)
	require.Equal(t, "bunny stream request failed: upload failed (attempt 1 of 3, retrying automatically)", got.ReplayError)
	require.NotNil(t, got.ReplayRetryAt)
	require.True(t, got.ReplayRetryAt.Equal(now.Add(15*time.Minute)), got.ReplayRetryAt)
	require.Equal(t, []string{"video-1"}, bunny.deletedVideos(), "the empty video of a failed upload is deleted")

	svc.retryFailedUploads(ctx, now.Add(14*time.Minute))
	require.Equal(t, 1, bunny.createdCount(), "retried before its backoff ended")

	now = now.Add(15 * time.Minute)
	svc.retryFailedUploads(ctx, now)
	got = load()
	require.Equal(t, 2, got.ReplayAttempts)
	require.Contains(t, got.ReplayError, "(attempt 2 of 3, retrying automatically)")
	require.True(t, got.ReplayRetryAt.Equal(now.Add(30*time.Minute)), "the delay doubles: %v", got.ReplayRetryAt)

	now = now.Add(30 * time.Minute)
	svc.retryFailedUploads(ctx, now)
	got = load()
	require.Equal(t, model.ReplayStatusFailed, got.ReplayStatus)
	require.Equal(t, 3, got.ReplayAttempts)
	require.Nil(t, got.ReplayRetryAt)
	require.Equal(t, "bunny stream request failed: upload failed (gave up after 3 attempts)", got.ReplayError)
	require.Equal(t, 3, bunny.createdCount())
	require.Len(t, bunny.deletedVideos(), 3)

	svc.retryFailedUploads(ctx, now.Add(48*time.Hour))
	svc.RecoverInterruptedUploads(ctx)
	require.Equal(t, 3, bunny.createdCount(), "retried after giving up")

	// Kept for failedRetention after giving up, then removed as stale.
	recordedAt := now.Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(recordPath, recordedAt, recordedAt))
	now = now.Add(defaultFailedRecordingRetention - time.Hour)
	svc.CleanupStaleRecordings(ctx)
	require.FileExists(t, recordPath)
	now = now.Add(2 * time.Hour)
	svc.CleanupStaleRecordings(ctx)
	require.NoFileExists(t, recordPath)
}

func TestRetryFailedUploadsStopsWhenRecordingIsGone(t *testing.T) {
	ctx := context.Background()
	svc, rooms, bunny, room, recordPath := newReplayUploadTest(t)
	fastRecordingWaits(svc)
	require.NoError(t, rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusFailed, "bunny stream request failed"))
	require.NoError(t, os.Remove(recordPath))

	svc.RecoverInterruptedUploads(ctx)

	got, err := rooms.GetByID(ctx, room.ID)
	require.NoError(t, err)
	require.Equal(t, model.ReplayStatusFailed, got.ReplayStatus)
	require.Contains(t, got.ReplayError, errRecordingNotFound.Error())
	require.Equal(t, 1, got.ReplayAttempts)
	require.Nil(t, got.ReplayRetryAt)
	require.Zero(t, bunny.createdCount())
	retryable, err := rooms.ReplayRetryableUploads(ctx, time.Time{}, 0)
	require.NoError(t, err)
	require.Empty(t, retryable)
}

func TestRunUploadRetriesRetriesFailedUploadsWhenDue(t *testing.T) {
	ctx := context.Background()
	svc, rooms, bunny, room, recordPath := newReplayUploadTest(t)
	fastRecordingWaits(svc)
	svc.retryInterval = 5 * time.Millisecond
	var clock atomic.Int64
	clock.Store(time.Date(2026, 5, 4, 14, 0, 0, 0, time.UTC).UnixNano())
	svc.now = func() time.Time { return time.Unix(0, clock.Load()).UTC() }
	bunny.failUploads = 1
	require.NoError(t, rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusFailed, "bunny stream request failed"))
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.RunUploadRetries(runCtx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	// Retried at start, where it fails again.
	require.Eventually(t, func() bool {
		got, err := rooms.GetByID(ctx, room.ID)
		return err == nil && got.ReplayStatus == model.ReplayStatusFailed && got.ReplayAttempts == 1
	}, 2*time.Second, 5*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	require.Equal(t, 1, bunny.createdCount(), "retried before its backoff ended")

	clock.Add(int64(defaultReplayRetryDelay))
	require.Eventually(t, func() bool {
		got, err := rooms.GetByID(ctx, room.ID)
		return err == nil && got.ReplayStatus == model.ReplayStatusReady
	}, 2*time.Second, 5*time.Millisecond)
	require.Equal(t, 2, bunny.createdCount())
	require.NoFileExists(t, recordPath)
}

func TestRecoverInterruptedUploadsResumesPending(t *testing.T) {
	ctx := context.Background()
	svc, rooms, bunny, room, recordPath := newReplayUploadTest(t)
	fastRecordingWaits(svc)

	svc.RecoverInterruptedUploads(ctx)

	require.Eventually(t, func() bool {
		got, err := rooms.GetByID(ctx, room.ID)
		return err == nil && got.ReplayStatus == model.ReplayStatusReady
	}, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, 1, bunny.createdCount())
	require.NoFileExists(t, recordPath)
}

func TestCleanupStaleRecordingsKeepsRecordingsStillNeeded(t *testing.T) {
	ctx := context.Background()
	svc, rooms, _, pending, _ := newReplayUploadTest(t)
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	endedAt := now.Add(-48 * time.Hour)
	for _, room := range []model.Room{
		{ID: "room-live", Status: model.StatusLive, StreamKey: "lk_live", StartedAt: endedAt},
		{ID: "room-failed", Status: model.StatusEnded, EndedAt: &endedAt, ReplayUploadEnabled: true, ReplayStatus: model.ReplayStatusFailed},
		{ID: "room-off-failed", Status: model.StatusEnded, EndedAt: &endedAt, ReplayStatus: model.ReplayStatusFailed},
		{ID: "room-none", Status: model.StatusEnded, EndedAt: &endedAt, ReplayStatus: model.ReplayStatusNone},
	} {
		room.Title, room.OwnerID = room.ID, "owner-"+room.ID
		require.NoError(t, rooms.Upsert(ctx, &room))
	}

	old, fresh := now.Add(-25*time.Hour), now.Add(-time.Hour)
	files := map[string]time.Time{
		pending.ID + ".flv":           old,   // pending upload
		"room-live.flv":               old,   // active room
		"room-live_q720.flv":          old,   // its transcoded variant
		"lk_live.flv":                 old,   // legacy key-named recording of the active room
		"room-failed.flv":             old,   // failed upload, to be retried
		"room-off-failed.flv":         old,   // failed upload, replay upload since turned off
		"room-none.flv.tmp":           old,   // SRS died mid-recording
		"room-none.flv":               fresh, // cleanup may still be on its way
		"miclink-live-room-guest.flv": old,   // mic-link guest stream
		"notes.txt":                   old,   // not a recording
	}
	dir := svc.recordDir
	for name, mod := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("flv"), 0o644))
		require.NoError(t, os.Chtimes(path, mod, mod))
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub.flv"), 0o755))

	svc.CleanupStaleRecordings(ctx)

	for _, name := range []string{pending.ID + ".flv", "room-live.flv", "room-live_q720.flv", "lk_live.flv", "room-failed.flv", "room-none.flv", "notes.txt"} {
		require.FileExists(t, filepath.Join(dir, name), name)
	}
	require.DirExists(t, filepath.Join(dir, "sub.flv"))
	for _, name := range []string{"room-off-failed.flv", "room-none.flv.tmp", "miclink-live-room-guest.flv"} {
		require.NoFileExists(t, filepath.Join(dir, name), name)
	}
}

// The cleanup runs when room-service starts: it must not delete the
// recordings of failed uploads, which the old code retried on every restart,
// until they have given up and failedRetention has passed.
func TestCleanupStaleRecordingsKeepsFailedUploadRecordings(t *testing.T) {
	ctx := context.Background()
	svc, rooms, _, _, _ := newReplayUploadTest(t)
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	retryAt := now.Add(time.Hour)
	withinRetention := now.Add(-defaultFailedRecordingRetention + time.Hour)
	pastRetention := now.Add(-defaultFailedRecordingRetention - time.Hour)
	deletedAt := now.Add(-time.Hour)
	recordedAt := now.Add(-30 * 24 * time.Hour)
	kept := map[string]model.Room{
		"retrying":         {ID: "room-retrying", ReplayUploadEnabled: true, ReplayAttempts: 2, ReplayRetryAt: &retryAt, ReplayFailedAt: &withinRetention},
		"gave up recently": {ID: "room-gave-up-recently", ReplayUploadEnabled: true, ReplayAttempts: 7, ReplayFailedAt: &withinRetention},
	}
	removed := map[string]model.Room{
		"gave up long ago":  {ID: "room-gave-up-long-ago", ReplayUploadEnabled: true, ReplayAttempts: 7, ReplayFailedAt: &pastRetention},
		"upload turned off": {ID: "room-upload-off", ReplayAttempts: 1, ReplayFailedAt: &withinRetention},
		"replay deleted":    {ID: "room-deleted", ReplayStatus: model.ReplayStatusDeleted, ReplayDeletedAt: &deletedAt},
	}
	paths := map[string]string{}
	for name, room := range kept {
		paths[name] = addFailedReplay(t, svc, rooms, room, recordedAt)
	}
	for name, room := range removed {
		paths[name] = addFailedReplay(t, svc, rooms, room, recordedAt)
	}
	// Failed before retries were counted, and recorded under the raw stream
	// key as rooms were before they published under their id.
	legacy := model.Room{ID: "room-legacy", StreamKey: "lk_0123456789abcdef0123456789abcdef", ReplayUploadEnabled: true,
		ReplayError: "bunny stream library or api key is not configured"}
	paths["legacy"] = filepath.Join(svc.recordDir, legacy.StreamKey+".flv")
	require.NoError(t, os.WriteFile(paths["legacy"], []byte("flv"), 0o644))
	require.Equal(t, paths["legacy"], addFailedReplay(t, svc, rooms, legacy, recordedAt))
	kept["legacy"] = legacy

	svc.CleanupStaleRecordings(ctx)

	for name := range kept {
		require.FileExists(t, paths[name], name)
	}
	for name := range removed {
		require.NoFileExists(t, paths[name], name)
	}
}

// A replay deleted while its upload runs is neither pending nor uploading
// any more, but its recording is still being read.
func TestCleanupStaleRecordingsKeepsRecordingOfDeletedReplayStillUploading(t *testing.T) {
	ctx := context.Background()
	svc, rooms, bunny, room, recordPath := newReplayUploadTest(t)
	fastRecordingWaits(svc)
	require.NoError(t, rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusFailed, "bunny stream request failed"))
	recordedAt := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(recordPath, recordedAt, recordedAt))
	var existedMidUpload bool
	bunny.onUpload = func() error {
		if err := svc.DeleteReplay(ctx, room.OwnerID, room.ID); err != nil {
			return err
		}
		svc.CleanupStaleRecordings(ctx)
		_, err := os.Stat(recordPath)
		existedMidUpload = err == nil
		return nil
	}

	svc.RecoverInterruptedUploads(ctx)

	bunny.mu.Lock()
	defer bunny.mu.Unlock()
	require.Empty(t, bunny.errs)
	require.True(t, existedMidUpload, "the cleanup removed the recording of an upload in progress")
	got, err := rooms.GetByID(ctx, room.ID)
	require.NoError(t, err)
	require.Equal(t, model.ReplayStatusDeleted, got.ReplayStatus)
	require.Equal(t, []string{"video-1"}, bunny.deleted)
	require.NoFileExists(t, recordPath)
}
