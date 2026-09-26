package service

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
	"github.com/qingwenwen777/golive/pkg/logger"
	"go.uber.org/zap"
)

const defaultBunnyAPIBase = "https://video.bunnycdn.com"
const defaultBunnyPlayerBase = "https://player.mediadelivery.net/embed"
const replayRecordingStableInterval = 2 * time.Second
const replayRecordingStableChecks = 5
const replayRecoveryLimit = 20

// replayRecordingSettleDelay gives SRS time to close a stream's DVR file
// after the room ends, before it is looked for.
const replayRecordingSettleDelay = 5 * time.Second
const defaultStaleRecordingAge = 24 * time.Hour

// replayUploadDeadline bounds one upload attempt, from waiting for the
// recording to publishing the video.
const replayUploadDeadline = 45 * time.Minute

// Failed uploads are retried up to defaultReplayUploadAttempts attempts in
// all, defaultReplayRetryDelay after the first failure and twice as long
// after each further one (at most maxReplayRetryDelay). Once none is left,
// the recording is kept defaultFailedRecordingRetention for a manual retry.
const defaultReplayUploadAttempts = 7
const defaultReplayRetryDelay = 15 * time.Minute
const maxReplayRetryDelay = 24 * time.Hour
const defaultFailedRecordingRetention = 7 * 24 * time.Hour

// replayRetryCheckInterval is how often RunUploadRetries looks for failed
// uploads whose retry is due.
const replayRetryCheckInterval = time.Minute

// replayAbandonedSegmentAge is how long a .tmp recording must be untouched
// before it counts as left behind by an SRS that died while writing it. SRS
// writes a live stream's recording every few seconds.
const replayAbandonedSegmentAge = time.Minute

// joinedRecordingSuffix ends the name of the temporary file a recording of
// several segments is joined into for its upload. No upload runs longer than
// joinedRecordingMaxAge, so an older one was left by a room-service restart.
const joinedRecordingSuffix = ".joined.flv.part"
const joinedRecordingMaxAge = 2 * replayUploadDeadline

var errRecordingNotFound = errors.New("recording file not found")
var errRecordingInProgress = errors.New("recording still being written")

type ReplayConfig struct {
	RecordDir       string
	BunnyLibraryID  string
	BunnyAPIKey     string
	BunnyAPIBase    string
	BunnyPlayerBase string
	UploadTimeout   time.Duration
	// StaleRecordingAge is how long a recording must be untouched before
	// CleanupStaleRecordings may remove it (default 24h).
	StaleRecordingAge time.Duration
	// UploadAttempts is how many times a replay upload is tried before it
	// gives up (default 7; 1 means no retries).
	UploadAttempts int
	// UploadRetryDelay is the wait before a failed upload is retried the
	// first time; it doubles with every further failure (default 15m).
	UploadRetryDelay time.Duration
	// FailedRecordingRetention is how long the recording of an upload that
	// gave up is kept, for a manual retry, before CleanupStaleRecordings may
	// remove it (default 7 days).
	FailedRecordingRetention time.Duration
}

type ReplayService struct {
	rooms             *repo.RoomRepo
	social            *repo.SocialRepo
	bunny             *BunnyClient
	recordDir         string
	libraryID         string
	playerBase        string
	staleRecordingAge time.Duration
	uploadAttempts    int
	retryDelay        time.Duration
	failedRetention   time.Duration
	settleDelay       time.Duration
	stableInterval    time.Duration
	retryInterval     time.Duration
	now               func() time.Time

	// uploads holds the rooms whose upload runs in this process, by id.
	uploadsMu sync.Mutex
	uploads   map[string]model.Room
}

type ReplayListResp struct {
	Items []model.Stream `json:"items"`
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"size"`
}

type UpdateLiveReplaySettingsReq struct {
	UploadAfterEnd bool   `json:"uploadAfterEnd"`
	Visibility     string `json:"visibility"`
}

type UpdateReplayReq struct {
	Visibility string `json:"visibility"`
}

func NewReplayService(rooms *repo.RoomRepo, social *repo.SocialRepo, cfg ReplayConfig) *ReplayService {
	apiBase := strings.TrimRight(cfg.BunnyAPIBase, "/")
	if apiBase == "" {
		apiBase = defaultBunnyAPIBase
	}
	playerBase := strings.TrimRight(cfg.BunnyPlayerBase, "/")
	if playerBase == "" {
		playerBase = defaultBunnyPlayerBase
	}
	timeout := cfg.UploadTimeout
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	staleAge := cfg.StaleRecordingAge
	if staleAge <= 0 {
		staleAge = defaultStaleRecordingAge
	}
	attempts := cfg.UploadAttempts
	if attempts <= 0 {
		attempts = defaultReplayUploadAttempts
	}
	retryDelay := cfg.UploadRetryDelay
	if retryDelay <= 0 {
		retryDelay = defaultReplayRetryDelay
	}
	failedRetention := cfg.FailedRecordingRetention
	if failedRetention <= 0 {
		failedRetention = defaultFailedRecordingRetention
	}
	return &ReplayService{
		rooms:             rooms,
		social:            social,
		bunny:             NewBunnyClient(apiBase, cfg.BunnyAPIKey, timeout),
		recordDir:         strings.TrimSpace(cfg.RecordDir),
		libraryID:         strings.TrimSpace(cfg.BunnyLibraryID),
		playerBase:        playerBase,
		staleRecordingAge: staleAge,
		uploadAttempts:    attempts,
		retryDelay:        retryDelay,
		failedRetention:   failedRetention,
		settleDelay:       replayRecordingSettleDelay,
		stableInterval:    replayRecordingStableInterval,
		retryInterval:     replayRetryCheckInterval,
		now:               time.Now,
	}
}

func (s *ReplayService) UpdateActiveSettings(ctx context.Context, ownerID string, req UpdateLiveReplaySettingsReq) (*model.Replay, error) {
	if ownerID == "" {
		return nil, errcode.ErrUnauthorized
	}
	room, err := s.rooms.ActiveByOwner(ctx, ownerID)
	if errors.Is(err, repo.ErrRoomNotFound) {
		return nil, errcode.New(http.StatusNotFound, "active live not found").WithReason("active_live_not_found")
	}
	if err != nil {
		return nil, err
	}
	visibility := normalizeReplayVisibility(req.Visibility)
	status := room.ReplayStatus
	if status == "" || status == model.ReplayStatusDeleted {
		status = model.ReplayStatusNone
	}
	if req.UploadAfterEnd {
		status = model.ReplayStatusPending
	} else if status == model.ReplayStatusPending || status == model.ReplayStatusNone {
		status = model.ReplayStatusNone
	}
	updated, err := s.rooms.UpdateReplaySettings(ctx, room.ID, req.UploadAfterEnd, visibility, status)
	if err != nil {
		return nil, err
	}
	return s.ReplayDTO(ctx, *updated, ownerID)
}

func (s *ReplayService) ListMine(ctx context.Context, ownerID string, page, size int) (*ReplayListResp, error) {
	if ownerID == "" {
		return nil, errcode.ErrUnauthorized
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 24
	}
	rooms, total, err := s.rooms.ReplayRoomsByOwner(ctx, ownerID, page, size)
	if err != nil {
		return nil, err
	}
	items := make([]model.Stream, 0, len(rooms))
	now := s.now()
	for i := range rooms {
		st := rooms[i].ToStream(now)
		replay, err := s.ReplayDTO(ctx, rooms[i], ownerID)
		if err != nil {
			return nil, err
		}
		st.Replay = replay
		items = append(items, st)
	}
	return &ReplayListResp{Items: items, Total: total, Page: page, Size: size}, nil
}

func (s *ReplayService) UpdateReplay(ctx context.Context, ownerID, roomID string, req UpdateReplayReq) (*model.Replay, error) {
	if ownerID == "" {
		return nil, errcode.ErrUnauthorized
	}
	room, err := s.rooms.UpdateReplayVisibility(ctx, ownerID, roomID, normalizeReplayVisibility(req.Visibility))
	if errors.Is(err, repo.ErrRoomNotFound) {
		return nil, ErrRoomNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.ReplayDTO(ctx, *room, ownerID)
}

func (s *ReplayService) DeleteReplay(ctx context.Context, ownerID, roomID string) error {
	if ownerID == "" {
		return errcode.ErrUnauthorized
	}
	room, err := s.rooms.EndedRoomByOwner(ctx, ownerID, roomID)
	if errors.Is(err, repo.ErrRoomNotFound) {
		return ErrRoomNotFound
	}
	if err != nil {
		return err
	}
	if room.ReplayBunnyVideoID != "" && room.ReplayBunnyLibraryID != "" {
		if err := s.bunny.DeleteVideo(ctx, room.ReplayBunnyLibraryID, room.ReplayBunnyVideoID); err != nil && !errors.Is(err, ErrBunnyNotConfigured) {
			return err
		}
	}
	return s.rooms.MarkReplayDeleted(ctx, room.ID, s.now())
}

func (s *ReplayService) EnqueueUpload(ctx context.Context, room model.Room) {
	if !room.ReplayUploadEnabled {
		go s.cleanupRoomRecording(room)
		return
	}
	_ = s.rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusPending, "")
	s.startUpload(room)
}

// startUpload runs room's replay upload in the background, unless one runs
// for it already.
func (s *ReplayService) startUpload(room model.Room) {
	if !s.trackUpload(room) {
		return
	}
	go func() {
		defer s.untrackUpload(room.ID)
		s.uploadRoomReplay(room)
	}()
}

// trackUpload records that room's upload runs in this process, so no second
// one starts and CleanupStaleRecordings keeps its recording even if the
// replay is deleted meanwhile. It reports false when one runs already.
func (s *ReplayService) trackUpload(room model.Room) bool {
	s.uploadsMu.Lock()
	defer s.uploadsMu.Unlock()
	if _, ok := s.uploads[room.ID]; ok {
		return false
	}
	if s.uploads == nil {
		s.uploads = map[string]model.Room{}
	}
	s.uploads[room.ID] = room
	return true
}

func (s *ReplayService) untrackUpload(roomID string) {
	s.uploadsMu.Lock()
	defer s.uploadsMu.Unlock()
	delete(s.uploads, roomID)
}

// uploadingRooms returns the rooms whose upload runs in this process.
func (s *ReplayService) uploadingRooms() []model.Room {
	s.uploadsMu.Lock()
	defer s.uploadsMu.Unlock()
	rooms := make([]model.Room, 0, len(s.uploads))
	for _, room := range s.uploads {
		rooms = append(rooms, room)
	}
	return rooms
}

func (s *ReplayService) ReplayDTO(ctx context.Context, room model.Room, viewerID string) (*model.Replay, error) {
	isOwner := viewerID != "" && viewerID == room.OwnerID
	status := normalizeReplayStatus(room.ReplayStatus)
	if status == model.ReplayStatusNone && !room.ReplayUploadEnabled && !isOwner {
		return nil, nil
	}
	if status == model.ReplayStatusNone && !room.ReplayUploadEnabled && isOwner {
		return &model.Replay{
			RoomID:         room.ID,
			Status:         model.ReplayStatusNone,
			Visibility:     normalizeReplayVisibility(room.ReplayVisibility),
			UploadAfterEnd: false,
			CanManage:      true,
		}, nil
	}
	if status == model.ReplayStatusDeleted && !isOwner {
		return nil, nil
	}

	canWatch, err := s.CanView(ctx, room, viewerID)
	if err != nil {
		return nil, err
	}
	canManage := isOwner
	replay := &model.Replay{
		RoomID:         room.ID,
		Status:         status,
		Visibility:     normalizeReplayVisibility(room.ReplayVisibility),
		UploadAfterEnd: room.ReplayUploadEnabled,
		UploadedAt:     formatOptionalTime(room.ReplayUploadedAt),
		DeletedAt:      formatOptionalTime(room.ReplayDeletedAt),
		CanWatch:       canWatch,
		CanManage:      canManage,
	}
	if canManage {
		replay.Error = room.ReplayError
		replay.BunnyVideoID = room.ReplayBunnyVideoID
	}
	if canWatch || canManage {
		replay.EmbedURL = s.embedURL(room)
	}
	if replay.EmbedURL == "" && !canManage {
		return nil, nil
	}
	return replay, nil
}

func (s *ReplayService) CanView(ctx context.Context, room model.Room, viewerID string) (bool, error) {
	if room.Status != model.StatusEnded {
		return false, nil
	}
	if normalizeReplayStatus(room.ReplayStatus) != model.ReplayStatusReady || room.ReplayBunnyVideoID == "" {
		return false, nil
	}
	if viewerID != "" && viewerID == room.OwnerID {
		return true, nil
	}
	switch normalizeReplayVisibility(room.ReplayVisibility) {
	case model.PostVisibilityPublic:
		return true, nil
	case model.PostVisibilityFollowers:
		if viewerID == "" || s.social == nil {
			return false, nil
		}
		return s.social.IsFollowing(ctx, viewerID, room.ChannelID)
	case model.PostVisibilityPrivate:
		return false, nil
	default:
		return false, nil
	}
}

func (s *ReplayService) embedURL(room model.Room) string {
	videoID := strings.TrimSpace(room.ReplayBunnyVideoID)
	if videoID == "" {
		return ""
	}
	libraryID := strings.TrimSpace(room.ReplayBunnyLibraryID)
	if libraryID == "" {
		libraryID = s.libraryID
	}
	if libraryID == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", s.playerBase, libraryID, videoID)
}

// uploadRoomReplay waits for SRS to close the room's recording, then uploads
// it (see runUpload).
func (s *ReplayService) uploadRoomReplay(room model.Room) {
	time.Sleep(s.settleDelay)
	s.runUpload(room)
}

// runUpload makes one attempt at uploading room's replay. A failed attempt
// is retried later while attempts are left (see failUpload).
func (s *ReplayService) runUpload(room model.Room) {
	ctx, cancel := context.WithTimeout(context.Background(), replayUploadDeadline)
	defer cancel()
	if s.recordDir == "" {
		s.failUpload(ctx, room, "record_dir is not configured")
		return
	}
	if s.libraryID == "" || !s.bunny.Configured() {
		s.failUpload(ctx, room, "bunny stream library or api key is not configured")
		return
	}
	segments, err := s.waitForUploadableRecording(ctx, s.recordingStreamName(room))
	if err != nil {
		s.failUpload(ctx, room, err.Error())
		return
	}
	s.uploadRecording(ctx, room, segments)
}

// uploadRecording uploads a room's finished DVR files to Bunny and publishes
// the video as the room's replay, unless the creator deleted the replay in
// the meantime. The files are removed afterwards.
func (s *ReplayService) uploadRecording(ctx context.Context, room model.Room, segments []string) {
	if err := s.rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusUploading, ""); err != nil {
		return
	}
	videoID, err := s.uploadVideo(ctx, room, segments)
	if err != nil {
		s.failUpload(ctx, room, err.Error())
		return
	}
	updated, err := s.rooms.SetReplayUploaded(ctx, room.ID, s.libraryID, videoID, model.ReplayStatusReady, s.now())
	if err != nil {
		s.failUpload(ctx, room, err.Error())
		return
	}
	if !updated {
		// Deleted mid-upload: DeleteReplay had no video id to remove yet, so
		// drop the video here rather than leave it in the library.
		if err := s.bunny.DeleteVideo(ctx, s.libraryID, videoID); err != nil {
			logger.L().Warn("delete replay video after replay was deleted", zap.Error(err), zap.String("room_id", room.ID), zap.String("video_id", videoID))
		}
	}
	removeRecordings(room.ID, segments)
}

// uploadVideo creates room's Bunny video and uploads its recording. Several
// segments (the publisher reconnected) are joined into one temporary FLV
// next to them first, removed as soon as the upload is over; a single one
// is uploaded as it is. A video whose upload failed is deleted again, so
// retries don't leave one behind each.
func (s *ReplayService) uploadVideo(ctx context.Context, room model.Room, segments []string) (string, error) {
	uploadPath := segments[0]
	if len(segments) > 1 {
		joined, err := s.joinRecording(room, segments)
		if err != nil {
			return "", err
		}
		defer removeRecordings(room.ID, []string{joined})
		uploadPath = joined
	}
	videoID, err := s.bunny.CreateVideo(ctx, s.libraryID, replayVideoTitle(room))
	if err != nil {
		return "", err
	}
	if err := s.bunny.UploadVideo(ctx, s.libraryID, videoID, uploadPath); err != nil {
		s.deleteVideo(ctx, room.ID, videoID)
		return "", err
	}
	return videoID, nil
}

// joinRecording joins room's recording segments into one FLV in the record
// dir and returns its path. It takes as much disk space as the segments.
func (s *ReplayService) joinRecording(room model.Room, segments []string) (string, error) {
	file, err := os.CreateTemp(s.recordDir, room.ID+".*"+joinedRecordingSuffix)
	if err != nil {
		return "", fmt.Errorf("join recording segments: %w", err)
	}
	joined, err := joinFLV(file, segments)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		removeRecordings(room.ID, []string{file.Name()})
		return "", fmt.Errorf("join recording segments: %w", err)
	}
	for _, damage := range joined.damaged {
		logger.L().Warn("recording segment cut short or skipped", zap.String("room_id", room.ID), zap.String("segment", damage))
	}
	logger.L().Info("joined recording segments for replay upload",
		zap.String("room_id", room.ID),
		zap.Strings("segments", segments),
		zap.String("path", file.Name()),
		zap.Int64("bytes", joined.size),
		zap.Duration("duration", joined.duration),
	)
	return file.Name(), nil
}

// removeRecordings removes recording files, logging those it cannot.
func removeRecordings(roomID string, paths []string) {
	for _, path := range paths {
		if err := removeRecording(path); err != nil {
			logger.L().Warn("remove replay recording", zap.Error(err), zap.String("room_id", roomID), zap.String("path", path))
		}
	}
}

// deleteVideo removes the video of a failed upload, best effort. The
// attempt's context may have run out, so it gets one of its own.
func (s *ReplayService) deleteVideo(ctx context.Context, roomID, videoID string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := s.bunny.DeleteVideo(ctx, s.libraryID, videoID); err != nil {
		logger.L().Warn("delete video of failed replay upload", zap.Error(err), zap.String("room_id", roomID), zap.String("video_id", videoID))
	}
}

// failUpload records a failed attempt at room's replay upload. While
// attempts are left it schedules a retry, retryDelay after the first failure
// and twice as long after each further one; the last attempt gives up, and
// the recording is then kept failedRetention for a manual retry. The reason
// the creator sees says which it is.
func (s *ReplayService) failUpload(ctx context.Context, room model.Room, reason string) {
	attempts := room.ReplayAttempts + 1
	now := s.now()
	var retryAt *time.Time
	message := reason
	if attempts < s.uploadAttempts {
		at := now.Add(s.retryDelayAfter(attempts))
		retryAt = &at
		message = fmt.Sprintf("%s (attempt %d of %d, retrying automatically)", reason, attempts, s.uploadAttempts)
	} else if attempts > 1 {
		message = fmt.Sprintf("%s (gave up after %d attempts)", reason, attempts)
	}
	// The attempt's context may have run out (e.g. waiting for the recording).
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	updated, err := s.rooms.SetReplayFailed(ctx, room.ID, message, attempts, now, retryAt)
	if err != nil {
		logger.L().Warn("record failed replay upload", zap.Error(err), zap.String("room_id", room.ID), zap.String("reason", reason))
		return
	}
	if !updated {
		// Deleted meanwhile.
		return
	}
	fields := []zap.Field{
		zap.String("room_id", room.ID),
		zap.Int("attempt", attempts),
		zap.Int("max_attempts", s.uploadAttempts),
		zap.String("reason", reason),
	}
	if retryAt != nil {
		logger.L().Warn("replay upload failed; retry scheduled", append(fields, zap.Time("retry_at", *retryAt))...)
		return
	}
	logger.L().Error("replay upload failed; giving up, recording kept for a manual retry",
		append(fields, zap.Time("recording_kept_until", now.Add(s.failedRetention)))...)
}

// retryDelayAfter is how long after its attempts-th failure an upload is
// retried: retryDelay, doubled for each failure before (up to
// maxReplayRetryDelay).
func (s *ReplayService) retryDelayAfter(attempts int) time.Duration {
	delay := s.retryDelay
	for i := 1; i < attempts && delay < maxReplayRetryDelay; i++ {
		delay = min(2*delay, maxReplayRetryDelay)
	}
	return delay
}

func (s *ReplayService) cleanupRoomRecording(room model.Room) {
	time.Sleep(s.settleDelay)
	if s.recordDir == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	segments, err := s.waitForUploadableRecording(ctx, s.recordingStreamName(room))
	if err != nil {
		return
	}
	removeRecordings(room.ID, segments)
}

// cleanupStreamRecording removes the DVR files of a stream that never becomes
// a replay, such as a mic-link guest stream published over RTMP.
func (s *ReplayService) cleanupStreamRecording(stream string) {
	if s.recordDir == "" || stream == "" || strings.ContainsAny(stream, `/\*?[`) {
		return
	}
	time.Sleep(s.settleDelay)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	segments, err := s.waitForUploadableRecording(ctx, stream)
	if err != nil {
		return
	}
	for _, recordPath := range segments {
		if err := removeRecording(recordPath); err != nil {
			logger.L().Warn("remove stream recording", zap.Error(err), zap.String("stream", stream), zap.String("path", recordPath))
		}
	}
}

// RunUploadRetries resumes interrupted replay uploads and retries failed
// ones right away (RecoverInterruptedUploads), then retries failed uploads
// as their retries come due, until ctx is done.
func (s *ReplayService) RunUploadRetries(ctx context.Context) {
	if s == nil {
		return
	}
	s.RecoverInterruptedUploads(ctx)
	interval := s.retryInterval
	if interval <= 0 {
		interval = replayRetryCheckInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		s.retryFailedUploads(ctx, s.now())
	}
}

// RecoverInterruptedUploads resumes the replay uploads a restart interrupted
// and retries the failed ones with attempts left right away, as a restart
// always did; each retry counts as an attempt.
func (s *ReplayService) RecoverInterruptedUploads(ctx context.Context) {
	if s == nil || s.rooms == nil || s.recordDir == "" {
		return
	}
	if s.libraryID != "" && s.bunny.Configured() {
		s.resumeInterruptedUploads(ctx)
	}
	s.retryFailedUploads(ctx, time.Time{})
}

// retryFailedUploads retries the failed replay uploads that have attempts
// left and are due by dueBy (all of them for the zero time). They run one
// after the other, so retries of large recordings don't compete for
// bandwidth, nor for the disk space their joined copies take.
func (s *ReplayService) retryFailedUploads(ctx context.Context, dueBy time.Time) {
	if s == nil || s.rooms == nil || s.recordDir == "" {
		return
	}
	rooms, err := s.rooms.ReplayRetryableUploads(ctx, dueBy, replayRecoveryLimit)
	if err != nil {
		logger.L().Warn("load failed replay uploads to retry", zap.Error(err))
		return
	}
	for _, room := range rooms {
		if ctx.Err() != nil {
			return
		}
		s.retryUpload(ctx, room)
	}
}

// retryUpload makes another attempt at a failed replay upload, which moves
// it through pending and uploading to ready or failed like the first. A
// replay whose recording is gone stops being retried.
func (s *ReplayService) retryUpload(ctx context.Context, room model.Room) {
	if !s.trackUpload(room) {
		return
	}
	defer s.untrackUpload(room.ID)
	if _, err := s.findRecording(s.recordingStreamName(room)); err != nil && !errors.Is(err, errRecordingInProgress) {
		if !errors.Is(err, errRecordingNotFound) {
			logger.L().Warn("look for recording of failed replay upload", zap.Error(err), zap.String("room_id", room.ID))
			return
		}
		// Nothing left to upload: stop retrying.
		logger.L().Warn("failed replay upload not retried: recording is gone", zap.Error(err), zap.String("room_id", room.ID))
		if _, err := s.rooms.SetReplayFailed(ctx, room.ID, err.Error(), room.ReplayAttempts+1, s.now(), nil); err != nil {
			logger.L().Warn("record failed replay upload", zap.Error(err), zap.String("room_id", room.ID))
		}
		return
	}
	claimed, err := s.rooms.ClaimReplayRetry(ctx, room.ID, room.ReplayAttempts)
	if err != nil {
		logger.L().Warn("claim failed replay upload for retry", zap.Error(err), zap.String("room_id", room.ID))
		return
	}
	if !claimed {
		return
	}
	logger.L().Info("retry failed replay upload",
		zap.String("room_id", room.ID),
		zap.Int("attempt", room.ReplayAttempts+1),
		zap.Int("max_attempts", s.uploadAttempts),
		zap.String("last_error", room.ReplayError),
	)
	s.runUpload(room)
}

func (s *ReplayService) resumeInterruptedUploads(ctx context.Context) {
	rooms, err := s.rooms.ReplayRecoverableUploads(ctx, replayRecoveryLimit)
	if err != nil {
		logger.L().Warn("load recoverable replay uploads", zap.Error(err))
		return
	}
	for _, room := range rooms {
		segments, err := s.findRecording(s.recordingStreamName(room))
		if err != nil && !errors.Is(err, errRecordingInProgress) {
			logger.L().Info(
				"skip replay recovery without final recording",
				zap.String("room_id", room.ID),
				zap.Error(err),
			)
			continue
		}
		logger.L().Info(
			"recover interrupted replay upload",
			zap.String("room_id", room.ID),
			zap.Strings("paths", segments),
		)
		_ = s.rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusPending, "")
		s.startUpload(room)
	}
}

// CleanupStaleRecordings removes DVR files nothing will upload or clean up
// any more: files untouched for staleRecordingAge (SRS keeps writing a live
// stream's file) that belong to no active room, no pending, uploading or
// retryable replay upload, no failed upload within failedRetention of giving
// up, and no upload running in this process (whose replay may have been
// deleted meanwhile). These are left by uploads that gave up, mic-link
// streams, and restarts before a room's recording cleanup ran. It also
// removes joined copies of recordings a restart left behind.
func (s *ReplayService) CleanupStaleRecordings(ctx context.Context) {
	if s == nil || s.rooms == nil || s.recordDir == "" {
		return
	}
	entries, err := os.ReadDir(s.recordDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logger.L().Warn("list recordings", zap.Error(err), zap.String("dir", s.recordDir))
		}
		return
	}
	now := s.now()
	cutoff := now.Add(-s.staleRecordingAge)
	var stale []os.FileInfo
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		joined := strings.HasSuffix(name, joinedRecordingSuffix)
		if !entry.Type().IsRegular() || !(joined || strings.HasSuffix(name, ".flv") || strings.HasSuffix(name, ".flv.tmp")) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if joined {
			if info.ModTime().Before(now.Add(-joinedRecordingMaxAge)) {
				s.removeStaleRecording(info)
			}
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		stale = append(stale, info)
	}
	if len(stale) == 0 {
		return
	}
	rooms, err := s.rooms.RecordingRooms(ctx, now.Add(-s.failedRetention))
	if err != nil {
		logger.L().Warn("load rooms that need recordings", zap.Error(err))
		return
	}
	rooms = append(rooms, s.uploadingRooms()...)
	var keep []string
	for _, room := range rooms {
		keep = append(keep, room.ID, s.recordingStreamName(room))
		if room.StreamKey != "" {
			keep = append(keep, room.StreamKey)
		}
	}
	for _, info := range stale {
		if slices.ContainsFunc(keep, func(stream string) bool { return isRecordingOf(info.Name(), stream) }) {
			continue
		}
		s.removeStaleRecording(info)
	}
}

func (s *ReplayService) removeStaleRecording(info os.FileInfo) {
	recordPath := filepath.Join(s.recordDir, info.Name())
	if err := removeRecording(recordPath); err != nil {
		logger.L().Warn("remove stale recording", zap.Error(err), zap.String("path", recordPath))
		return
	}
	logger.L().Info("removed stale recording", zap.String("path", recordPath), zap.Int64("bytes", info.Size()),
		zap.Time("modified_at", info.ModTime()))
}

// recordingStreamName is the SRS stream name a room's DVR file is named after.
// Rooms publish as <roomID>?key=<secret>, so recordings use the room id; rooms
// that went live before that change published under the raw key, so use it
// when such a recording is still on disk.
func (s *ReplayService) recordingStreamName(room model.Room) string {
	if room.StreamKey != "" && s.recordDir != "" {
		if matches, _ := filepath.Glob(filepath.Join(s.recordDir, room.StreamKey+"*")); len(matches) > 0 {
			return room.StreamKey
		}
	}
	return room.ID
}

// findRecording returns the DVR files of a stream's publish sessions, oldest
// first. It fails with errRecordingInProgress while SRS still writes one,
// unless SRS left that .tmp file untouched for replayAbandonedSegmentAge: SRS
// died while recording it, and it is taken as it is.
func (s *ReplayService) findRecording(streamKey string) ([]string, error) {
	key := strings.TrimSpace(streamKey)
	if key == "" {
		return nil, errors.New("recording stream key is empty")
	}
	entries, err := os.ReadDir(s.recordDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var segments []recordingSegment
	for _, entry := range entries {
		segment, ok := parseRecordingName(entry.Name(), key)
		if !ok || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if segment.temp && time.Since(info.ModTime()) < replayAbandonedSegmentAge {
			return nil, fmt.Errorf("%w: %s", errRecordingInProgress, entry.Name())
		}
		if info.Size() == 0 {
			continue
		}
		segment.path = filepath.Join(s.recordDir, entry.Name())
		segments = append(segments, segment)
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("%w for stream %s", errRecordingNotFound, key)
	}
	slices.SortFunc(segments, func(a, b recordingSegment) int {
		return cmp.Or(cmp.Compare(a.start, b.start), strings.Compare(a.path, b.path))
	})
	paths := make([]string, len(segments))
	for i, segment := range segments {
		paths[i] = segment.path
	}
	return paths, nil
}

// recordingSegment is the DVR file of one publish session: <stream>.<start>.flv
// (dvr_path in deploy/srs.conf), start being the Unix time in ms SRS opened
// it, or <stream>.flv as recorded before dvr_path had a timestamp (start 0).
// temp means it still has SRS's .tmp suffix: SRS is writing it, or died
// while doing so.
type recordingSegment struct {
	path  string
	start int64
	temp  bool
}

// parseRecordingName reports whether name is a DVR file of stream, which
// exact form keeps another stream whose name starts with this one apart.
func parseRecordingName(name, stream string) (recordingSegment, bool) {
	rest, ok := strings.CutPrefix(name, stream+".")
	if !ok || stream == "" {
		return recordingSegment{}, false
	}
	rest, temp := strings.CutSuffix(rest, ".tmp")
	if rest == "flv" {
		return recordingSegment{temp: temp}, true
	}
	digits, ok := strings.CutSuffix(rest, ".flv")
	if !ok || digits == "" || strings.Trim(digits, "0123456789") != "" {
		return recordingSegment{}, false
	}
	start, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return recordingSegment{}, false
	}
	return recordingSegment{start: start, temp: temp}, true
}

// isRecordingOf reports whether name is a DVR file of stream or of one of
// its transcoded variants.
func isRecordingOf(name, stream string) bool {
	if _, ok := parseRecordingName(name, stream); ok {
		return true
	}
	for suffix := range streamVariantSuffixes {
		if _, ok := parseRecordingName(name, stream+suffix); ok {
			return true
		}
	}
	return false
}

func (s *ReplayService) waitForUploadableRecording(ctx context.Context, streamKey string) ([]string, error) {
	return s.waitForUploadableRecordingWith(
		ctx,
		streamKey,
		s.stableInterval,
		replayRecordingStableChecks,
	)
}

// waitForUploadableRecordingWith waits until the stream's recording is
// complete: no segment is being written any more and the last one has
// stopped changing. It returns the segments, oldest first.
func (s *ReplayService) waitForUploadableRecordingWith(
	ctx context.Context,
	streamKey string,
	interval time.Duration,
	requiredStableChecks int,
) ([]string, error) {
	if interval <= 0 {
		interval = time.Second
	}
	key := strings.TrimSpace(streamKey)
	for {
		segments, err := s.findRecording(key)
		if err == nil {
			if _, err = waitForStableFile(ctx, segments[len(segments)-1], interval, requiredStableChecks); err == nil {
				// Still the same segments, none started meanwhile.
				var again []string
				if again, err = s.findRecording(key); err == nil && slices.Equal(again, segments) {
					return segments, nil
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		} else if !errors.Is(err, errRecordingNotFound) && !errors.Is(err, errRecordingInProgress) {
			return nil, err
		}

		select {
		case <-ctx.Done():
			if errors.Is(err, errRecordingInProgress) {
				return nil, fmt.Errorf("recording of stream %s is still being written: %w", key, ctx.Err())
			}
			return nil, fmt.Errorf("recording final file not found for stream %s before timeout: %w", key, ctx.Err())
		case <-time.After(interval):
		}
	}
}

func removeRecording(recordPath string) error {
	path := strings.TrimSpace(recordPath)
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func waitForStableFile(
	ctx context.Context,
	recordPath string,
	interval time.Duration,
	requiredStableChecks int,
) (os.FileInfo, error) {
	if interval <= 0 {
		interval = time.Second
	}
	if requiredStableChecks <= 0 {
		requiredStableChecks = 1
	}

	var lastSize int64 = -1
	var lastMod time.Time
	stableChecks := 0

	for {
		info, err := os.Stat(recordPath)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			return nil, fmt.Errorf("recording path is a directory: %s", recordPath)
		}
		if info.Size() <= 0 {
			stableChecks = 0
		} else if info.Size() == lastSize && info.ModTime().Equal(lastMod) {
			stableChecks++
			if stableChecks >= requiredStableChecks {
				return info, nil
			}
		} else {
			stableChecks = 0
			lastSize = info.Size()
			lastMod = info.ModTime()
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("recording file did not become stable before timeout: %w", ctx.Err())
		case <-time.After(interval):
		}
	}
}

func replayVideoTitle(room model.Room) string {
	title := strings.TrimSpace(room.Title)
	if title == "" {
		title = room.ID
	}
	return title
}

func normalizeReplayVisibility(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case model.PostVisibilityFollowers:
		return model.PostVisibilityFollowers
	case model.PostVisibilityPrivate:
		return model.PostVisibilityPrivate
	default:
		return model.PostVisibilityPublic
	}
}

func normalizeReplayStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case model.ReplayStatusPending:
		return model.ReplayStatusPending
	case model.ReplayStatusUploading:
		return model.ReplayStatusUploading
	case model.ReplayStatusProcessing:
		return model.ReplayStatusProcessing
	case model.ReplayStatusReady:
		return model.ReplayStatusReady
	case model.ReplayStatusFailed:
		return model.ReplayStatusFailed
	case model.ReplayStatusDeleted:
		return model.ReplayStatusDeleted
	default:
		return model.ReplayStatusNone
	}
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

var ErrBunnyNotConfigured = errors.New("bunny stream is not configured")

type BunnyClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewBunnyClient(baseURL, apiKey string, timeout time.Duration) *BunnyClient {
	return &BunnyClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  strings.TrimSpace(apiKey),
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *BunnyClient) Configured() bool {
	return c != nil && c.baseURL != "" && c.apiKey != ""
}

func (c *BunnyClient) CreateVideo(ctx context.Context, libraryID, title string) (string, error) {
	if !c.Configured() {
		return "", ErrBunnyNotConfigured
	}
	body, err := json.Marshal(map[string]string{"title": title})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.videoURL(libraryID, ""), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("AccessKey", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", bunnyHTTPError(resp)
	}
	var payload struct {
		GUID      string `json:"guid"`
		VideoGUID string `json:"videoGuid"`
		ID        string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	videoID := firstNonEmpty(payload.GUID, payload.VideoGUID, payload.ID)
	if videoID == "" {
		return "", errors.New("bunny did not return a video id")
	}
	return videoID, nil
}

func (c *BunnyClient) UploadVideo(ctx context.Context, libraryID, videoID, filePath string) error {
	if !c.Configured() {
		return ErrBunnyNotConfigured
	}
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("video upload path is a directory: %s", filePath)
	}
	if info.Size() <= 0 {
		return fmt.Errorf("video upload file is empty: %s", filePath)
	}
	body := io.NewSectionReader(file, 0, info.Size())
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.videoURL(libraryID, videoID), body)
	if err != nil {
		return err
	}
	req.Header.Set("AccessKey", c.apiKey)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = info.Size()
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return bunnyHTTPError(resp)
	}
	return nil
}

func (c *BunnyClient) DeleteVideo(ctx context.Context, libraryID, videoID string) error {
	if !c.Configured() {
		return ErrBunnyNotConfigured
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.videoURL(libraryID, videoID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("AccessKey", c.apiKey)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return bunnyHTTPError(resp)
	}
	return nil
}

func (c *BunnyClient) videoURL(libraryID, videoID string) string {
	base := fmt.Sprintf("%s/library/%s/videos", c.baseURL, strings.TrimSpace(libraryID))
	if strings.TrimSpace(videoID) == "" {
		return base
	}
	return base + "/" + strings.TrimSpace(videoID)
}

func bunnyHTTPError(resp *http.Response) error {
	defer io.Copy(io.Discard, resp.Body)
	limited := io.LimitReader(resp.Body, 2048)
	body, _ := io.ReadAll(limited)
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = resp.Status
	}
	return fmt.Errorf("bunny stream request failed: %s", msg)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
