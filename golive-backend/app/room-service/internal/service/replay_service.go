package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

var errRecordingNotFound = errors.New("recording file not found")

type ReplayConfig struct {
	RecordDir       string
	BunnyLibraryID  string
	BunnyAPIKey     string
	BunnyAPIBase    string
	BunnyPlayerBase string
	UploadTimeout   time.Duration
}

type ReplayService struct {
	rooms      *repo.RoomRepo
	social     *repo.SocialRepo
	bunny      *BunnyClient
	recordDir  string
	libraryID  string
	playerBase string
	now        func() time.Time
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
	return &ReplayService{
		rooms:      rooms,
		social:     social,
		bunny:      NewBunnyClient(apiBase, cfg.BunnyAPIKey, timeout),
		recordDir:  strings.TrimSpace(cfg.RecordDir),
		libraryID:  strings.TrimSpace(cfg.BunnyLibraryID),
		playerBase: playerBase,
		now:        time.Now,
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
	go s.uploadRoomReplay(room)
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

func (s *ReplayService) uploadRoomReplay(room model.Room) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	time.Sleep(5 * time.Second)
	if s.recordDir == "" {
		_ = s.rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusFailed, "record_dir is not configured")
		return
	}
	if s.libraryID == "" || !s.bunny.Configured() {
		_ = s.rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusFailed, "bunny stream library or api key is not configured")
		return
	}
	recordPath, err := s.waitForUploadableRecording(ctx, room.StreamKey)
	if err != nil {
		_ = s.rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusFailed, err.Error())
		return
	}
	if err := s.rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusUploading, ""); err != nil {
		return
	}
	videoID, err := s.bunny.CreateVideo(ctx, s.libraryID, replayVideoTitle(room))
	if err != nil {
		_ = s.rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusFailed, err.Error())
		return
	}
	if err := s.bunny.UploadVideo(ctx, s.libraryID, videoID, recordPath); err != nil {
		_ = s.rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusFailed, err.Error())
		return
	}
	if err := s.rooms.SetReplayUploaded(ctx, room.ID, s.libraryID, videoID, model.ReplayStatusReady, s.now()); err != nil {
		_ = s.rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusFailed, err.Error())
		return
	}
	if err := removeRecording(recordPath); err != nil {
		logger.L().Warn("remove replay recording", zap.Error(err), zap.String("room_id", room.ID), zap.String("path", recordPath))
	}
}

func (s *ReplayService) cleanupRoomRecording(room model.Room) {
	time.Sleep(5 * time.Second)
	if s.recordDir == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	recordPath, err := s.waitForUploadableRecording(ctx, room.StreamKey)
	if err != nil {
		return
	}
	if err := removeRecording(recordPath); err != nil {
		logger.L().Warn("remove replay recording", zap.Error(err), zap.String("room_id", room.ID), zap.String("path", recordPath))
	}
}

func (s *ReplayService) RecoverInterruptedUploads(ctx context.Context) {
	if s == nil || s.rooms == nil || s.recordDir == "" || s.libraryID == "" || !s.bunny.Configured() {
		return
	}
	rooms, err := s.rooms.ReplayRecoverableUploads(ctx, replayRecoveryLimit)
	if err != nil {
		logger.L().Warn("load recoverable replay uploads", zap.Error(err))
		return
	}
	for _, room := range rooms {
		recordPath, err := s.findRecording(room.StreamKey)
		if err != nil {
			logger.L().Info(
				"skip replay recovery without final recording",
				zap.String("room_id", room.ID),
				zap.String("stream_key", room.StreamKey),
				zap.Error(err),
			)
			continue
		}
		logger.L().Info(
			"recover interrupted replay upload",
			zap.String("room_id", room.ID),
			zap.String("stream_key", room.StreamKey),
			zap.String("path", recordPath),
		)
		_ = s.rooms.SetReplayStatus(ctx, room.ID, model.ReplayStatusPending, "")
		go s.uploadRoomReplay(room)
	}
}

func (s *ReplayService) findRecording(streamKey string) (string, error) {
	key := strings.TrimSpace(streamKey)
	if key == "" {
		return "", errors.New("recording stream key is empty")
	}
	exact := filepath.Join(s.recordDir, key+".flv")
	if info, err := os.Stat(exact); err == nil && !info.IsDir() && info.Size() > 0 {
		return exact, nil
	}
	matches, err := filepath.Glob(filepath.Join(s.recordDir, key+"*"))
	if err != nil {
		return "", err
	}
	var newest string
	var newestMod time.Time
	for _, match := range matches {
		if !isFinalRecordingPath(match) {
			continue
		}
		info, err := os.Stat(match)
		if err != nil || info.IsDir() || info.Size() == 0 {
			continue
		}
		if newest == "" || info.ModTime().After(newestMod) {
			newest = match
			newestMod = info.ModTime()
		}
	}
	if newest == "" {
		return "", fmt.Errorf("%w for stream %s", errRecordingNotFound, key)
	}
	return newest, nil
}

func isFinalRecordingPath(recordPath string) bool {
	name := strings.ToLower(filepath.Base(recordPath))
	return strings.HasSuffix(name, ".flv") && !strings.HasSuffix(name, ".tmp")
}

func (s *ReplayService) waitForUploadableRecording(ctx context.Context, streamKey string) (string, error) {
	return s.waitForUploadableRecordingWith(
		ctx,
		streamKey,
		replayRecordingStableInterval,
		replayRecordingStableChecks,
	)
}

func (s *ReplayService) waitForUploadableRecordingWith(
	ctx context.Context,
	streamKey string,
	interval time.Duration,
	requiredStableChecks int,
) (string, error) {
	if interval <= 0 {
		interval = time.Second
	}
	key := strings.TrimSpace(streamKey)
	for {
		recordPath, err := s.findRecording(key)
		if err == nil {
			if _, err := waitForStableFile(ctx, recordPath, interval, requiredStableChecks); err == nil {
				return recordPath, nil
			} else if !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		} else if !errors.Is(err, errRecordingNotFound) {
			return "", err
		}

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("recording final file not found for stream %s before timeout: %w", key, ctx.Err())
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

func waitForStableRecording(ctx context.Context, recordPath string) (os.FileInfo, error) {
	return waitForStableFile(ctx, recordPath, replayRecordingStableInterval, replayRecordingStableChecks)
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
