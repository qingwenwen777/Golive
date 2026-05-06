package service

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

func TestNotificationHash_IsStableShortAndOrderSensitive(t *testing.T) {
	first := notificationHash("reply", "post-1", "actor-1")
	second := notificationHash("reply", "post-1", "actor-1")
	reordered := notificationHash("reply", "actor-1", "post-1")

	require.Equal(t, first, second)
	require.Len(t, first, 32)
	require.NotEqual(t, first, reordered)
}

func TestCleanDirectMessage_TrimsNormalizesAndCapsByRune(t *testing.T) {
	text, err := cleanDirectMessage(" \r\n hello\r\nworld \n ")
	require.NoError(t, err)
	require.Equal(t, "hello\nworld", text)

	long := strings.Repeat("你", maxDirectMessageRunes+5)
	text, err = cleanDirectMessage(long)
	require.NoError(t, err)
	require.Len(t, []rune(text), maxDirectMessageRunes)
}

func TestCleanDirectMessage_RejectsBlankContentWithReason(t *testing.T) {
	_, err := cleanDirectMessage(" \r\n\t ")

	var appErr *errcode.AppError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, http.StatusBadRequest, appErr.HTTPStatus)
	require.Equal(t, "content_required", appErr.Reason)
}

func TestMessagePreferenceAndFanGroupRoleNormalization(t *testing.T) {
	require.Equal(t, model.MessagePreferenceScopeFollowing, normalizeMessageScope(" FOLLOWING "))
	require.Equal(t, model.MessagePreferenceScopeNone, normalizeMessageScope("none"))
	require.Equal(t, model.MessagePreferenceScopeAll, normalizeMessageScope("unexpected"))

	require.Equal(t, model.FanGroupRoleAdmin, normalizeFanGroupRole(" ADMIN "))
	require.Equal(t, model.FanGroupRoleMember, normalizeFanGroupRole("owner"))
	require.Equal(t, model.FanGroupRoleMember, normalizeFanGroupRole("something-else"))
}

func TestFanBadgeFromLevelBounds(t *testing.T) {
	require.Nil(t, fanBadgeFromLevel("", 3))
	require.Nil(t, fanBadgeFromLevel("creator-1", 0))
	require.Equal(t, &FanBadgeDTO{CreatorID: "creator-1", Level: 3}, fanBadgeFromLevel("creator-1", 3))
	require.Equal(t, &FanBadgeDTO{CreatorID: "creator-1", Level: 99}, fanBadgeFromLevel("creator-1", 120))
}

func TestFanGroupDTOMarksMuteKickAndRejoinState(t *testing.T) {
	createdAt := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	futureMute := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	pastMute := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	kickedAt := createdAt.Add(time.Hour)
	rejoinRequestedAt := createdAt.Add(2 * time.Hour)
	rejoinRejectedAt := createdAt.Add(3 * time.Hour)

	dto := fanGroupDTO(repo.FanGroupWithMembers{
		Group: model.FanGroupChat{
			ID:          "group-1",
			CreatorID:   "creator-1",
			GroupNo:     1,
			Name:        "Creator Fans",
			MemberCount: 2,
			CreatedAt:   createdAt,
			UpdatedAt:   createdAt.Add(time.Minute),
		},
		Members: []repo.FanGroupMemberRow{
			{
				GroupID:           "group-1",
				UserID:            "fan-1",
				Role:              model.FanGroupRoleMember,
				MutedUntil:        &futureMute,
				KickedAt:          &kickedAt,
				KickReason:        model.FanGroupKickReasonManual,
				RejoinRequestedAt: &rejoinRequestedAt,
				RejoinRejectedAt:  &rejoinRejectedAt,
				CreatedAt:         createdAt,
				Username:          "fan_one",
				DisplayName:       "",
				Name:              "",
				Avatar:            "fan.png",
				Verified:          true,
				FanBadgeLevel:     12,
			},
			{
				GroupID:       "group-1",
				UserID:        "fan-2",
				Role:          model.FanGroupRoleAdmin,
				MutedUntil:    &pastMute,
				CreatedAt:     createdAt,
				Username:      "fan_two",
				DisplayName:   "Fan Two",
				Name:          "Fan Two Display",
				FanBadgeLevel: 0,
			},
		},
	})

	require.Equal(t, "group-1", dto.ID)
	require.Equal(t, "Creator Fans", dto.Name)
	require.Len(t, dto.Members, 2)

	first := dto.Members[0]
	require.Equal(t, "fan_one", first.User.Name)
	require.True(t, first.User.Verified)
	require.True(t, first.Muted)
	require.Equal(t, futureMute.Format(time.RFC3339), first.MutedUntil)
	require.True(t, first.Kicked)
	require.True(t, first.PendingRejoin)
	require.Equal(t, model.FanGroupKickReasonManual, first.KickReason)
	require.Equal(t, &FanBadgeDTO{CreatorID: "creator-1", Level: 12}, first.FanBadge)
	require.Equal(t, first.FanBadge, first.User.FanBadge)
	require.Equal(t, rejoinRequestedAt.Format(time.RFC3339), first.RejoinRequestedAt)
	require.Equal(t, rejoinRejectedAt.Format(time.RFC3339), first.RejoinRejectedAt)

	second := dto.Members[1]
	require.Equal(t, "Fan Two Display", second.User.Name)
	require.False(t, second.Muted)
	require.Empty(t, second.MutedUntil)
	require.Nil(t, second.FanBadge)
}
