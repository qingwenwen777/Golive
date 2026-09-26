package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
// onUpload runs while the upload request is in flight.
type fakeBunny struct {
	mu       sync.Mutex
	onUpload func() error
	errs     []error
	deleted  []string
	created  int
}

func (b *fakeBunny) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/library/lib/videos":
		b.mu.Lock()
		b.created++
		b.mu.Unlock()
		fmt.Fprint(w, `{"guid":"video-1"}`)
	case r.Method == http.MethodPut && r.URL.Path == "/library/lib/videos/video-1":
		_, _ = io.Copy(io.Discard, r.Body)
		b.mu.Lock()
		onUpload := b.onUpload
		b.mu.Unlock()
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

func TestRecoverInterruptedUploadsDoesNotRetryFailed(t *testing.T) {
	ctx := context.Background()
	svc, rooms, bunny, room, recordPath := newReplayUploadTest(t)
	fastRecordingWaits(svc)
	require.NoError(t, rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusFailed, "bunny stream request failed"))

	svc.RecoverInterruptedUploads(ctx)

	got, err := rooms.GetByID(ctx, room.ID)
	require.NoError(t, err)
	require.Equal(t, model.ReplayStatusFailed, got.ReplayStatus)
	time.Sleep(50 * time.Millisecond)
	require.Zero(t, bunny.createdCount())
	require.FileExists(t, recordPath)
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
		"room-failed.flv":             old,   // failed upload, never retried
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

	for _, name := range []string{pending.ID + ".flv", "room-live.flv", "room-live_q720.flv", "lk_live.flv", "room-none.flv", "notes.txt"} {
		require.FileExists(t, filepath.Join(dir, name), name)
	}
	require.DirExists(t, filepath.Join(dir, "sub.flv"))
	for _, name := range []string{"room-failed.flv", "room-none.flv.tmp", "miclink-live-room-guest.flv"} {
		require.NoFileExists(t, filepath.Join(dir, name), name)
	}
}
