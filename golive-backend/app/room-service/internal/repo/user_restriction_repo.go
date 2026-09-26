package repo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/pkg/contentpolicy"
	"gorm.io/gorm"
)

type UserRestriction struct {
	Banned        bool
	Muted         bool
	MuteRemaining time.Duration
	MuteExpiresAt *time.Time
	BanReason     string
	MuteReason    string
}

// RecordUserSanction logs a sanction in user_sanction_logs and refreshes
// the Redis restriction flags from the current state. The ban / mute itself
// is applied by user-service, which owns users and user_moderation_states.
func (r *ModerationRepo) RecordUserSanction(ctx context.Context, targetUserID, targetName, operatorID, action, sourceReportID, note string, durationMinutes int, now time.Time) error {
	targetUserID = strings.TrimSpace(targetUserID)
	if targetUserID == "" {
		return nil
	}
	if profile, err := r.UserProfile(ctx, targetUserID); err == nil && strings.TrimSpace(targetName) == "" {
		targetName = profile.Name
	}
	var expiresAt *time.Time
	if action == model.UserSanctionSiteMute && durationMinutes > 0 {
		expires := now.Add(time.Duration(durationMinutes) * time.Minute)
		expiresAt = &expires
	}
	err := r.db.WithContext(ctx).Create(&model.UserSanctionLog{
		ID:              uuid.NewString(),
		TargetUserID:    targetUserID,
		TargetUserName:  trimForDB(targetName, 128),
		Action:          action,
		OperatorID:      operatorID,
		SourceReportID:  sourceReportID,
		Note:            note,
		DurationMinutes: durationMinutes,
		ExpiresAt:       expiresAt,
		CreatedAt:       now,
	}).Error
	if err != nil {
		return err
	}
	return r.syncUserRestriction(ctx, targetUserID, now)
}

func (r *ModerationRepo) UserRestriction(ctx context.Context, userID string, now time.Time) (UserRestriction, error) {
	var state model.UserModerationState
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Take(&state).Error
	restriction := UserRestriction{}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return UserRestriction{}, err
	}
	if err == nil {
		restriction.Banned = state.Banned
		restriction.BanReason = state.BanReason
		restriction.MuteReason = state.MuteReason
	}
	if err == nil && state.MutedUntil != nil && state.MutedUntil.After(now) {
		restriction.Muted = true
		restriction.MuteExpiresAt = state.MutedUntil
		restriction.MuteRemaining = state.MutedUntil.Sub(now)
	}
	var userState struct {
		Banned    bool
		BanReason string
	}
	userErr := r.db.WithContext(ctx).
		Table("users").
		Select("COALESCE(banned, false) AS banned, COALESCE(ban_reason, '') AS ban_reason").
		Where("id = ?", userID).
		Take(&userState).Error
	if userErr != nil && !errors.Is(userErr, gorm.ErrRecordNotFound) && !isMissingTableName(userErr) && !isMissingColumn(userErr) {
		return UserRestriction{}, userErr
	}
	if userErr == nil && userState.Banned {
		restriction.Banned = true
		if strings.TrimSpace(restriction.BanReason) == "" {
			restriction.BanReason = userState.BanReason
		}
	}
	return restriction, nil
}

// BannedUserIDs returns which of userIDs are banned, by the same rule as
// UserRestriction: user_moderation_states.banned or users.banned.
func (r *ModerationRepo) BannedUserIDs(ctx context.Context, userIDs []string) (map[string]bool, error) {
	banned := make(map[string]bool)
	if len(userIDs) == 0 {
		return banned, nil
	}
	var stateIDs, userRowIDs []string
	if err := r.db.WithContext(ctx).Model(&model.UserModerationState{}).
		Where("banned = ? AND user_id IN ?", true, userIDs).
		Pluck("user_id", &stateIDs).Error; err != nil {
		return nil, err
	}
	err := r.db.WithContext(ctx).Table("users").
		Where("banned = ? AND id IN ?", true, userIDs).
		Pluck("id", &userRowIDs).Error
	if err != nil && !isMissingTableName(err) && !isMissingColumn(err) {
		return nil, err
	}
	for _, id := range append(stateIDs, userRowIDs...) {
		banned[id] = true
	}
	return banned, nil
}

func (r *ModerationRepo) SyncUserRestrictions(ctx context.Context, now time.Time) error {
	if r.rdb == nil {
		return nil
	}
	var states []model.UserModerationState
	if err := r.db.WithContext(ctx).
		Where("banned = ? OR muted_until > ?", true, now).
		Find(&states).Error; err != nil {
		return err
	}
	for _, state := range states {
		if err := r.syncUserRestriction(ctx, state.UserID, now); err != nil {
			return err
		}
	}
	return nil
}

func (r *ModerationRepo) syncUserRestriction(ctx context.Context, userID string, now time.Time) error {
	if r.rdb == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	restriction, err := r.UserRestriction(ctx, userID, now)
	if err != nil {
		return err
	}
	pipe := r.rdb.Pipeline()
	if restriction.Banned {
		pipe.Set(ctx, contentpolicy.RedisSiteBanPrefix+userID, "1", 0)
	} else {
		pipe.Del(ctx, contentpolicy.RedisSiteBanPrefix+userID)
	}
	if restriction.Muted && restriction.MuteRemaining > 0 {
		pipe.Set(ctx, contentpolicy.RedisSiteMutePrefix+userID, restriction.MuteExpiresAt.UTC().Format(time.RFC3339), restriction.MuteRemaining)
	} else {
		pipe.Del(ctx, contentpolicy.RedisSiteMutePrefix+userID)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (r *ModerationRepo) CreateUnbanAppeal(ctx context.Context, appeal *model.UnbanAppeal) error {
	if appeal == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(appeal).Error
}
