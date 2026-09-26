package service

import (
	"context"
	"errors"
	"time"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/logger"
	"go.uber.org/zap"
)

const defaultReconcileInterval = time.Minute

// RunReconciler periodically repairs live rooms that the SRS hooks alone
// leave inconsistent and removes stale recordings, until ctx is done. See
// Reconcile and ReplayService.CleanupStaleRecordings.
func (s *LiveService) RunReconciler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultReconcileInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		s.Reconcile(ctx)
		if s.replay != nil {
			s.replay.CleanupStaleRecordings(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// staleRoom is a live room SRS reported without a publisher for longer than
// the unpublish grace period.
type staleRoom struct {
	room       model.Room
	disconnect repo.PublishDisconnect
}

// Reconcile runs one pass over the active rooms:
//   - live rooms keep their stream key and publish session from expiring;
//   - live rooms whose publisher is gone are ended like a normal stop once the
//     unpublish grace period has passed, whether the on_unpublish hook was
//     lost or its grace timer died with a restart;
//   - rooms never published to are ended once their stream key has expired.
//
// A room counts as without publisher when SRS's stream list lacks its play
// name, or, while SRS cannot be asked, only when on_unpublish recorded a
// disconnect: an SRS outage alone never ends rooms.
func (s *LiveService) Reconcile(ctx context.Context) {
	rooms, err := s.rooms.ActiveRooms(ctx)
	if err != nil {
		logger.L().Warn("reconcile: load active rooms", zap.Error(err))
		return
	}
	var publishers map[string]string
	if s.srs != nil {
		if publishers, err = s.srs.activePublishers(ctx); err != nil {
			logger.L().Warn("reconcile: list srs streams; only ending rooms with a recorded disconnect", zap.Error(err))
		}
	}
	now := s.now()
	var stale []staleRoom
	for i := range rooms {
		st, err := s.reconcileRoom(ctx, &rooms[i], publishers, now)
		if err != nil {
			logger.L().Warn("reconcile room", zap.Error(err), zap.String("room_id", rooms[i].ID))
			continue
		}
		if st != nil {
			stale = append(stale, *st)
		}
	}
	if len(stale) > 0 {
		s.endStaleRooms(ctx, stale)
	}
}

// reconcileRoom handles one active room. publishers is SRS's stream list, nil
// when SRS could not be asked. It returns the room when SRS shows it has been
// without a publisher past the grace period, for endStaleRooms to confirm.
func (s *LiveService) reconcileRoom(ctx context.Context, room *model.Room, publishers map[string]string, now time.Time) (*staleRoom, error) {
	clientID, publishing := publishers[playStreamName(room.ID, room.StreamKey)]
	if room.Status == model.StatusPublishing {
		// Never went live. Its key expires keyTTL after GoLive/Start, after
		// which nobody can publish to it any more.
		if s.keyTTL > 0 && !publishing && now.Sub(room.StartedAt) > s.keyTTL {
			_, err := s.stopRoom(ctx, room, now, false)
			return nil, err
		}
		return nil, nil
	}

	if room.StreamKey != "" {
		// Keep the key and session alive however long the live runs.
		if err := s.live.Save(ctx, room.StreamKey, room.ID, s.keyTTL); err != nil {
			return nil, err
		}
		err := s.live.RefreshPublishSession(ctx, room.StreamKey, s.keyTTL)
		if errors.Is(err, repo.ErrStreamKeyNotFound) && clientID != "" {
			err = s.live.SavePublishSession(ctx, room.StreamKey, clientID, s.keyTTL)
		}
		if err != nil && !errors.Is(err, repo.ErrStreamKeyNotFound) {
			return nil, err
		}
	}
	if publishing {
		// Also drops a disconnect whose on_unpublish arrived late.
		return nil, s.live.DeleteDisconnect(ctx, room.ID)
	}

	disconnect, err := s.live.Disconnect(ctx, room.ID)
	if errors.Is(err, repo.ErrStreamKeyNotFound) {
		if publishers == nil {
			return nil, nil
		}
		// Gone from SRS without an on_unpublish: the grace period starts now.
		return nil, s.live.SaveDisconnectIfAbsent(ctx, room.ID, repo.PublishDisconnect{At: now}, disconnectRecordTTL)
	}
	if err != nil {
		return nil, err
	}
	if now.Sub(disconnect.At) < s.unpublishGrace {
		return nil, nil
	}
	if publishers != nil {
		return &staleRoom{room: *room, disconnect: *disconnect}, nil
	}
	if !disconnect.Hook || room.StreamKey == "" {
		// Only SRS saw it missing: wait until SRS can confirm.
		return nil, nil
	}
	return nil, s.finalizeUnpublish(ctx, room.StreamKey, room.ID)
}

// endStaleRooms ends rooms SRS showed without a publisher, after checking a
// fresh stream list, so a publisher that reconnected while this pass ran is
// not cut off.
func (s *LiveService) endStaleRooms(ctx context.Context, stale []staleRoom) {
	publishers, err := s.srs.activePublishers(ctx)
	if err != nil {
		logger.L().Warn("reconcile: confirm srs streams", zap.Error(err))
		return
	}
	for i := range stale {
		room := &stale[i].room
		if _, ok := publishers[playStreamName(room.ID, room.StreamKey)]; ok {
			continue
		}
		disconnect, err := s.live.Disconnect(ctx, room.ID)
		if err != nil || !disconnect.At.Equal(stale[i].disconnect.At) {
			// Reconnected (record cleared) or disconnected anew meanwhile.
			continue
		}
		logger.L().Info("reconcile: ending live room without publisher",
			zap.String("room_id", room.ID), zap.Time("disconnected_at", disconnect.At))
		if _, err := s.stopRoom(ctx, room, disconnect.At, false); err != nil {
			logger.L().Warn("reconcile: end room", zap.Error(err), zap.String("room_id", room.ID))
		}
	}
}
