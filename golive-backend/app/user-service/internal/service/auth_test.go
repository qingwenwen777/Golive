package service_test

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/user-service/internal/repo"
	"github.com/qingwenwen777/golive/app/user-service/internal/service"
)

// helpers ----------------------------------------------------------------

func newGormSQLMock(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	// GORM 1.5 issues a SELECT VERSION() probe on Open; allow it.
	mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
		WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow("8.0.33"))

	gdb, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: false,
	}), &gorm.Config{})
	require.NoError(t, err)
	return gdb, mock, db
}

func newMiniredis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	m, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(m.Close)
	c := redis.NewClient(&redis.Options{Addr: m.Addr()})
	return m, c
}

func newSvc(t *testing.T) (*service.AuthService, sqlmock.Sqlmock, *miniredis.Miniredis) {
	t.Helper()
	gdb, mock, _ := newGormSQLMock(t)
	mr, rdb := newMiniredis(t)
	svc := service.NewAuthService(
		repo.NewUserRepo(gdb),
		repo.NewTokenRepo(rdb),
		service.Options{
			JWTSecret:  "test-secret",
			AccessTTL:  time.Hour,
			RefreshTTL: 24 * time.Hour,
		},
	)
	return svc, mock, mr
}

func expectFindByUsername(mock sqlmock.Sqlmock, username, hash string) {
	rows := sqlmock.NewRows([]string{"id", "username", "display_name", "password_hash", "avatar", "coin_balance", "verified", "created_at", "updated_at"}).
		AddRow("u-1", username, username, hash, "", int64(100), true, time.Now(), time.Now())
	mock.ExpectQuery(`SELECT \* FROM .users. WHERE username = \? LIMIT \?`).
		WithArgs(username, 1).
		WillReturnRows(rows)
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(amount\), 0\) FROM .coin_transactions. WHERE user_id = \? AND type = \? AND amount > 0`).
		WithArgs("u-1", "topup").
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(int64(0)))
}

func expectFindBannedByUsername(mock sqlmock.Sqlmock, username, hash string) {
	rows := sqlmock.NewRows([]string{"id", "username", "display_name", "password_hash", "avatar", "coin_balance", "banned", "ban_reason", "verified", "created_at", "updated_at"}).
		AddRow("u-1", username, username, hash, "", int64(100), true, "appeal test", true, time.Now(), time.Now())
	mock.ExpectQuery(`SELECT \* FROM .users. WHERE username = \? LIMIT \?`).
		WithArgs(username, 1).
		WillReturnRows(rows)
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(amount\), 0\) FROM .coin_transactions. WHERE user_id = \? AND type = \? AND amount > 0`).
		WithArgs("u-1", "topup").
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(int64(0)))
}

func expectFindByUsernameNotFound(mock sqlmock.Sqlmock, username string) {
	mock.ExpectQuery(`SELECT \* FROM .users. WHERE username = \? LIMIT \?`).
		WithArgs(username, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
}

func expectCreateUser(mock sqlmock.Sqlmock, username string) {
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `users`").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
}

// tests ------------------------------------------------------------------

func TestLogin_Success(t *testing.T) {
	svc, mock, mr := newSvc(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("demo"), bcrypt.MinCost)
	expectFindByUsername(mock, "demo", string(hash))

	resp, err := svc.Login(context.Background(), "demo", "demo")
	require.NoError(t, err)
	require.NotEmpty(t, resp.Token)
	require.NotEmpty(t, resp.RefreshToken)
	require.Equal(t, "demo", resp.User.Username)

	// refresh token persisted in redis
	require.True(t, mr.Exists("refresh:"+resp.RefreshToken))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLogin_BannedUserReceivesRestrictedSession(t *testing.T) {
	svc, mock, mr := newSvc(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("demo"), bcrypt.MinCost)
	expectFindBannedByUsername(mock, "demo", string(hash))

	resp, err := svc.Login(context.Background(), "demo", "demo")
	require.NoError(t, err)
	require.NotEmpty(t, resp.Token)
	require.NotEmpty(t, resp.RefreshToken)
	require.True(t, resp.User.Banned)
	require.Equal(t, "appeal test", resp.User.BanReason)
	require.True(t, mr.Exists("refresh:"+resp.RefreshToken))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLogin_WrongPassword(t *testing.T) {
	svc, mock, _ := newSvc(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("demo"), bcrypt.MinCost)
	expectFindByUsername(mock, "demo", string(hash))

	_, err := svc.Login(context.Background(), "demo", "wrong")
	require.ErrorIs(t, err, service.ErrInvalidCredentials)
}

func TestLogin_NoSuchUser(t *testing.T) {
	svc, mock, _ := newSvc(t)
	expectFindByUsernameNotFound(mock, "ghost")

	_, err := svc.Login(context.Background(), "ghost", "x")
	require.ErrorIs(t, err, service.ErrInvalidCredentials)
}

func TestLogin_FiveFailuresTriggerCooldown(t *testing.T) {
	svc, mock, mr := newSvc(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("demo"), bcrypt.MinCost)

	for i := 0; i < 4; i++ {
		expectFindByUsername(mock, "demo", string(hash))
		_, err := svc.Login(context.Background(), "demo", "wrong")
		require.ErrorIs(t, err, service.ErrInvalidCredentials)
	}

	expectFindByUsername(mock, "demo", string(hash))
	_, err := svc.Login(context.Background(), "demo", "wrong")
	require.ErrorIs(t, err, service.ErrLoginCooldown)

	_, err = svc.Login(context.Background(), "demo", "demo")
	require.ErrorIs(t, err, service.ErrLoginCooldown)

	mr.FastForward(time.Minute + time.Second)
	expectFindByUsername(mock, "demo", string(hash))
	resp, err := svc.Login(context.Background(), "demo", "demo")
	require.NoError(t, err)
	require.Equal(t, "demo", resp.User.Username)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRegister_Success(t *testing.T) {
	svc, mock, mr := newSvc(t)
	expectFindByUsernameNotFound(mock, "kabun")
	expectCreateUser(mock, "kabun")

	resp, err := svc.Register(context.Background(), " kabun ", "secret", "Kabun Live")
	require.NoError(t, err)
	require.NotEmpty(t, resp.Token)
	require.NotEmpty(t, resp.RefreshToken)
	require.Equal(t, "kabun", resp.User.Username)
	require.Equal(t, "Kabun Live", resp.User.DisplayName)
	require.Equal(t, int64(1200), resp.User.CoinBalance)
	require.True(t, mr.Exists("refresh:"+resp.RefreshToken))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRegister_UsernameTaken(t *testing.T) {
	svc, mock, _ := newSvc(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("demo"), bcrypt.MinCost)
	expectFindByUsername(mock, "demo", string(hash))

	_, err := svc.Register(context.Background(), "demo", "secret", "Demo")
	require.ErrorIs(t, err, service.ErrUsernameTaken)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRegister_InvalidDetails(t *testing.T) {
	svc, _, _ := newSvc(t)

	_, err := svc.Register(context.Background(), "ab", "pw", "")
	require.ErrorIs(t, err, service.ErrInvalidRegister)
}

func TestRefresh_RotatesAndInvalidatesOld(t *testing.T) {
	svc, _, mr := newSvc(t)
	// Pre-seed a refresh token directly.
	old := "old-refresh-token"
	require.NoError(t, mr.Set("refresh:"+old, "u-1"))
	mr.SetTTL("refresh:"+old, 24*time.Hour)

	resp, err := svc.Refresh(context.Background(), old)
	require.NoError(t, err)
	require.NotEmpty(t, resp.Token)
	require.NotEqual(t, old, resp.RefreshToken)

	// Old gone, new present.
	require.False(t, mr.Exists("refresh:"+old))
	require.True(t, mr.Exists("refresh:"+resp.RefreshToken))

	// And the old token can no longer be refreshed.
	_, err = svc.Refresh(context.Background(), old)
	require.ErrorIs(t, err, service.ErrInvalidRefresh)
}

func TestRefresh_InvalidToken(t *testing.T) {
	svc, _, _ := newSvc(t)
	_, err := svc.Refresh(context.Background(), "nope")
	require.ErrorIs(t, err, service.ErrInvalidRefresh)
}

func TestLogout_Revokes(t *testing.T) {
	svc, _, mr := newSvc(t)
	require.NoError(t, mr.Set("refresh:gone", "u-1"))
	require.NoError(t, svc.Logout(context.Background(), "gone"))
	require.False(t, mr.Exists("refresh:gone"))
}

func TestParseAccess_RoundTrip(t *testing.T) {
	svc, mock, _ := newSvc(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("demo"), bcrypt.MinCost)
	expectFindByUsername(mock, "demo", string(hash))

	resp, err := svc.Login(context.Background(), "demo", "demo")
	require.NoError(t, err)

	uid, err := svc.ParseAccess(resp.Token)
	require.NoError(t, err)
	require.Equal(t, "u-1", uid)
}
