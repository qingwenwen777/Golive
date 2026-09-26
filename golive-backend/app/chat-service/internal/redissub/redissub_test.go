package redissub

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/chat-service/internal/repo"
)

// Message ids used to be the client's clientId, so a client could pick the
// id of a message already on screen (or not yet persisted) and rewrite it.
func TestHandle_PersistsOnlyServerGeneratedIDs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "chat.db")), &gorm.Config{})
	require.NoError(t, err)
	danmus := repo.NewDanmuRepo(db, 2)
	require.NoError(t, danmus.AutoMigrate())
	s := New(nil, danmus)
	ctx := context.Background()

	id := uuid.NewString()
	s.handle(ctx, &redis.Message{Channel: "room:R1", Payload: `{"type":"chat","id":"` + id + `","userId":"u1","user":"Luna","text":"hi","ts":1}`})
	s.handle(ctx, &redis.Message{Channel: "room:R1", Payload: `{"type":"chat","id":"chat-123-abc","userId":"u1","user":"Luna","text":"spoof","ts":2}`})

	rows, err := danmus.History(ctx, "R1", 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, id, rows[0].ID)
}
