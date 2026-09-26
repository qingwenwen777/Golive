package server_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/app/room-service/internal/server"
	"github.com/qingwenwen777/golive/app/room-service/internal/service"
)

type allowLivePermission struct{}

func (allowLivePermission) HasApprovedLivePermission(context.Context, string) (bool, error) {
	return true, nil
}

// Approved creators who get banned must not be able to go live again, neither
// instantly nor through an appointment.
func TestLiveHTTP_BannedCreatorCannotGoLive(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE users (id varchar(36) primary key, username varchar(64), display_name varchar(64), avatar varchar(500), verified boolean, role varchar(16))`).Error)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	moderationRepo := repo.NewModerationRepo(db, rdb)
	require.NoError(t, moderationRepo.AutoMigrate())
	// user-service owns the ban state; seed it the way its ban API writes it.
	now := time.Now()
	require.NoError(t, db.Create(&model.UserModerationState{UserID: "banned-creator", Banned: true, BanReason: "abuse", UpdatedBy: "admin-1", UpdatedAt: now, CreatedAt: now}).Error)

	router := server.NewRouter(server.Deps{
		JWTSecret:  jwtSecret,
		Permission: allowLivePermission{},
		Moderation: service.NewModerationService(moderationRepo, nil, nil),
	})
	token := signedToken(t, "banned-creator")
	body := `{"scheduledAt":"2030-01-01T00:00:00Z","title":"Back again","description":"ban evasion","cover":"/c.jpg"}`

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/rooms/live", `{"title":"Back again","category":"Gaming"}`},
		{http.MethodPost, "/rooms/appointments", body},
		{http.MethodPatch, "/rooms/appointments/appt-1", body},
		{http.MethodPost, "/rooms/appointments/appt-1/start", ""},
	}
	for _, tc := range cases {
		rec := requestJSON(router, tc.method, tc.path, token, tc.body)
		require.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", tc.method, tc.path, rec.Body.String())
		require.Contains(t, rec.Body.String(), "user_banned")
	}
}
