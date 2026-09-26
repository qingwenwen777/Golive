package service

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
)

// ioCounter counts SQL statements (GORM callbacks) and Redis round trips (a
// pipeline counts once) so list endpoints can be checked for per-room I/O.
type ioCounter struct {
	sql   atomic.Int64
	redis atomic.Int64
}

func (c *ioCounter) reset() {
	c.sql.Store(0)
	c.redis.Store(0)
}

func (c *ioCounter) registerGORM(t testing.TB, db *gorm.DB) {
	t.Helper()
	count := func(*gorm.DB) { c.sql.Add(1) }
	cb := db.Callback()
	require.NoError(t, cb.Query().After("gorm:query").Register("test:count_query", count))
	require.NoError(t, cb.Row().After("gorm:row").Register("test:count_row", count))
	require.NoError(t, cb.Raw().After("gorm:raw").Register("test:count_raw", count))
	require.NoError(t, cb.Create().After("gorm:create").Register("test:count_create", count))
	require.NoError(t, cb.Update().After("gorm:update").Register("test:count_update", count))
	require.NoError(t, cb.Delete().After("gorm:delete").Register("test:count_delete", count))
}

func (c *ioCounter) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (c *ioCounter) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		c.redis.Add(1)
		return next(ctx, cmd)
	}
}

func (c *ioCounter) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		c.redis.Add(1)
		return next(ctx, cmds)
	}
}

type listPerfFixture struct {
	db       *gorm.DB
	rooms    *repo.RoomRepo
	messages *repo.MessageRepo
	social   *repo.SocialRepo
	svc      *RoomService
	search   *SearchService
	counter  *ioCounter
	now      time.Time
	viewerID string
	owners   []string
}

const listPerfRooms = 60

func perfOwnerID(i int) string {
	return fmt.Sprintf("00000000-0000-0000-0000-%012d", i)
}

// newListPerfFixture seeds listPerfRooms live rooms and as many public replays,
// each with its own owner, gifts, super chats, danmus, followers and Redis
// viewer metrics, plus one block between the viewer and the first owner.
func newListPerfFixture(t testing.TB) *listPerfFixture {
	t.Helper()
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "perf.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	rooms := repo.NewRoomRepo(db)
	require.NoError(t, rooms.AutoMigrate())
	messages := repo.NewMessageRepo(db)
	require.NoError(t, messages.AutoMigrate())
	posts := repo.NewPostRepo(db)
	require.NoError(t, posts.AutoMigrate())
	appointments := repo.NewAppointmentRepo(db)
	require.NoError(t, appointments.AutoMigrate())
	require.NoError(t, db.Exec(`
CREATE TABLE users (
	id TEXT PRIMARY KEY,
	username TEXT,
	display_name TEXT,
	avatar TEXT,
	cover TEXT,
	verified BOOLEAN DEFAULT 0,
	live_permission_status TEXT DEFAULT 'approved',
	updated_at DATETIME
);
CREATE TABLE gift_orders (room_id TEXT, user_id TEXT, total_coin INTEGER, status TEXT, created_at DATETIME);
CREATE TABLE super_chat_orders (room_id TEXT, user_id TEXT, amount INTEGER, status TEXT, created_at DATETIME);
CREATE TABLE fan_badges (user_id TEXT, creator_id TEXT, total_contribution INTEGER);
`).Error)
	for i := 0; i < 8; i++ {
		require.NoError(t, db.Exec(fmt.Sprintf("CREATE TABLE danmus_%d (room_id TEXT)", i)).Error)
	}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	social := repo.NewSocialRepo(rdb)
	live := repo.NewLiveRepo(rdb)

	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	viewerID := perfOwnerID(9999)
	owners := make([]string, 0, listPerfRooms)
	for i := 0; i < listPerfRooms; i++ {
		ownerID := perfOwnerID(i)
		owners = append(owners, ownerID)
		require.NoError(t, db.Exec(
			"INSERT INTO users (id, username, display_name, avatar, verified, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
			ownerID, fmt.Sprintf("creator%d", i), fmt.Sprintf("Creator %d", i), "/a.png", i%3 == 0, now,
		).Error)
		liveID := fmt.Sprintf("live-%02d", i)
		require.NoError(t, rooms.Upsert(ctx, &model.Room{
			ID:           liveID,
			Title:        fmt.Sprintf("Live show %d", i),
			Channel:      "Creator",
			ChannelID:    "ch-" + ownerID,
			Category:     "Gaming",
			OwnerID:      ownerID,
			Status:       model.StatusLive,
			StartedAt:    now.Add(-time.Duration(i+1) * time.Minute),
			Viewers:      int64(10 + i),
			PeakViewers:  int64(10 + i),
			ReplayStatus: model.ReplayStatusNone,
		}))
		endedAt := now.Add(-time.Duration(i+1) * time.Hour / 4)
		uploadedAt := endedAt.Add(time.Minute)
		replayID := fmt.Sprintf("replay-%02d", i)
		require.NoError(t, rooms.Upsert(ctx, &model.Room{
			ID:                   replayID,
			Title:                fmt.Sprintf("Replay show %d", i),
			Channel:              "Creator",
			ChannelID:            "ch-" + ownerID,
			Category:             "Gaming",
			OwnerID:              ownerID,
			Status:               model.StatusEnded,
			StartedAt:            endedAt.Add(-time.Hour),
			EndedAt:              &endedAt,
			PeakViewers:          int64(20 + i),
			ReplayStatus:         model.ReplayStatusReady,
			ReplayVisibility:     model.PostVisibilityPublic,
			ReplayBunnyVideoID:   replayID + "-video",
			ReplayBunnyLibraryID: "lib",
			ReplayUploadedAt:     &uploadedAt,
		}))
		for _, roomID := range []string{liveID, replayID} {
			for g := 0; g < 5; g++ {
				require.NoError(t, db.Exec(
					"INSERT INTO gift_orders (room_id, user_id, total_coin, status, created_at) VALUES (?, ?, ?, 'success', ?)",
					roomID, fmt.Sprintf("fan-%d", g), 10*(g+i+1), now.Add(-time.Minute),
				).Error)
			}
			require.NoError(t, db.Exec(
				"INSERT INTO super_chat_orders (room_id, user_id, amount, status, created_at) VALUES (?, 'fan-sc', ?, 'success', ?)",
				roomID, 100+i, now.Add(-time.Minute),
			).Error)
			require.NoError(t, db.Exec(fmt.Sprintf("INSERT INTO danmus_%d (room_id) VALUES (?)", i%8), roomID).Error)
		}
		require.NoError(t, rdb.HSet(ctx, "roommetrics:"+liveID, "viewers", 50+i, "peak", 80+i).Err())
		require.NoError(t, social.Follow(ctx, fmt.Sprintf("follower-%d", i), "ch-"+ownerID))
	}
	require.NoError(t, messages.UpsertBlock(ctx, viewerID, owners[0], "user", "", now))

	svc := NewRoomService(rooms, "", social)
	svc.now = func() time.Time { return now }
	svc.SetLiveRepo(live)
	svc.SetReplayService(NewReplayService(rooms, social, ReplayConfig{BunnyLibraryID: "lib"}))
	messageSvc := NewMessageService(messages, rooms, social)
	svc.SetBlockChecker(messageSvc)
	socialSvc := NewSocialService(social, rooms)
	socialSvc.SetBlockChecker(messageSvc)
	postSvc := NewPostService(posts, rooms, social, allowAllLivePermission{})
	postSvc.SetBlockChecker(messageSvc)
	apptSvc := NewAppointmentService(appointments, rooms, social, nil)
	apptSvc.SetBlockChecker(messageSvc)

	counter := &ioCounter{}
	counter.registerGORM(t, db)
	rdb.AddHook(counter)
	return &listPerfFixture{
		db:       db,
		rooms:    rooms,
		messages: messages,
		social:   social,
		svc:      svc,
		search:   NewSearchService(svc, socialSvc, postSvc, apptSvc),
		counter:  counter,
		now:      now,
		viewerID: viewerID,
		owners:   owners,
	}
}

type allowAllLivePermission struct{}

func (allowAllLivePermission) HasApprovedLivePermission(context.Context, string) (bool, error) {
	return true, nil
}

// measure runs fn and returns the SQL statements and Redis round trips it made.
func (f *listPerfFixture) measure(t testing.TB, fn func() error) (int64, int64) {
	t.Helper()
	f.counter.reset()
	require.NoError(t, fn())
	return f.counter.sql.Load(), f.counter.redis.Load()
}

// resetCaches empties the recommendation, hot-replay and suggestion pools so
// the next call measures a cold load.
func (f *listPerfFixture) resetCaches() {
	f.svc.liveRecommendations = newTTLCache[*liveRecommendationPool](liveRecommendationCacheTTL, roomPoolCacheEntries)
	f.svc.hotReplays = newTTLCache[*hotReplayPool](hotReplayCacheTTL, roomPoolCacheEntries)
	f.search.suggestions = newTTLCache[*suggestionPool](suggestCacheTTL, suggestCacheEntries)
}

// TestListEndpointsQueryCounts pins the I/O of the public list endpoints for
// a page of listPerfRooms rooms. Before batching, each room cost its own
// block, profile, metrics and follower lookups (sql/redis per call: List
// 121/118, RecommendedLive anonymous 83/73 and viewer 144/74, HotReplays
// anonymous 10/61 and viewer 70/60, SearchReplays 25/24); the bounds below
// don't grow with the number of rooms.
func TestListEndpointsQueryCounts(t *testing.T) {
	ctx := context.Background()
	f := newListPerfFixture(t)
	cases := []struct {
		name            string
		cold            bool
		maxSQL, maxRedi int64
		fn              func() error
	}{
		{"List viewer size=60", false, 4, 2, func() error {
			resp, err := f.svc.List(ctx, f.viewerID, "", 1, listPerfRooms)
			if err == nil && len(resp.Items) != listPerfRooms-1 {
				err = fmt.Errorf("list returned %d items", len(resp.Items))
			}
			return err
		}},
		// count + list + profiles + revenue + 8 danmu shards; followers and
		// metrics pipelines.
		{"RecommendedLive anonymous cold", true, 12, 2, func() error {
			_, err := f.svc.RecommendedLive(ctx, "", "", 12)
			return err
		}},
		{"RecommendedLive anonymous warm", false, 0, 0, func() error {
			_, err := f.svc.RecommendedLive(ctx, "", "", 12)
			return err
		}},
		{"RecommendedLive viewer cold", true, 15, 3, func() error {
			_, err := f.svc.RecommendedLive(ctx, f.viewerID, "", 12)
			return err
		}},
		// watch and gift preferences, blocks; following set.
		{"RecommendedLive viewer warm", false, 3, 1, func() error {
			_, err := f.svc.RecommendedLive(ctx, f.viewerID, "", 12)
			return err
		}},
		// candidates + revenue + 8 danmu shards; likes and followers.
		{"HotReplays anonymous cold", true, 10, 2, func() error {
			_, err := f.svc.HotReplays(ctx, "", "", 3, 12)
			return err
		}},
		{"HotReplays viewer cold", true, 11, 2, func() error {
			_, err := f.svc.HotReplays(ctx, f.viewerID, "", 3, 12)
			return err
		}},
		{"HotReplays anonymous warm", false, 0, 0, func() error {
			_, err := f.svc.HotReplays(ctx, "", "", 3, 12)
			return err
		}},
		{"HotReplays viewer warm", false, 1, 0, func() error {
			_, err := f.svc.HotReplays(ctx, f.viewerID, "", 3, 12)
			return err
		}},
		{"SearchReplays viewer", false, 2, 2, func() error {
			_, err := f.svc.SearchReplays(ctx, f.viewerID, "replay show", 24)
			return err
		}},
		// creators, live rooms, replay rooms, owner profiles; previously the
		// full five-way Search (14/60).
		{"Suggest viewer cold", true, 4, 1, func() error {
			_, err := f.search.Suggest(ctx, f.viewerID, "show", 12)
			return err
		}},
		{"Suggest viewer warm", false, 0, 0, func() error {
			_, err := f.search.Suggest(ctx, f.viewerID, "show", 12)
			return err
		}},
	}
	var report strings.Builder
	for _, tc := range cases {
		if tc.cold {
			f.resetCaches()
		}
		sqlCount, redisCount := f.measure(t, tc.fn)
		fmt.Fprintf(&report, "\n  %-32s sql=%-3d redis=%d", tc.name, sqlCount, redisCount)
		require.LessOrEqual(t, sqlCount, tc.maxSQL, tc.name)
		require.LessOrEqual(t, redisCount, tc.maxRedi, tc.name)
	}
	t.Log(report.String())
}

func TestListAppliesBatchedLookups(t *testing.T) {
	ctx := context.Background()
	f := newListPerfFixture(t)

	resp, err := f.svc.List(ctx, f.viewerID, "", 1, listPerfRooms)
	require.NoError(t, err)
	require.Len(t, resp.Items, listPerfRooms-1, "the blocked owner's room is filtered out")
	for _, item := range resp.Items {
		var i int
		_, err := fmt.Sscanf(item.ID, "live-%d", &i)
		require.NoError(t, err)
		require.NotEqual(t, 0, i, "owner 0 is blocked by the viewer")
		require.Equal(t, fmt.Sprintf("Creator %d", i), item.Channel, item.ID)
		require.Equal(t, i%3 == 0, item.Verified, item.ID)
		require.Equal(t, "/a.png", item.Avatar, item.ID)
		require.Equal(t, int64(50+i), item.Viewers, item.ID)
		require.Equal(t, int64(80+i), item.PeakViewers, item.ID)
		require.Equal(t, int64(1), item.SubscriberCount, item.ID)
	}

	anonymous, err := f.svc.List(ctx, "", "", 1, listPerfRooms)
	require.NoError(t, err)
	require.Len(t, anonymous.Items, listPerfRooms)
}

func TestStreamsFromRoomsMatchesSingleRoomStreams(t *testing.T) {
	ctx := context.Background()
	f := newListPerfFixture(t)
	for _, id := range []string{"live-05", "live-06"} {
		room, err := f.rooms.GetByID(ctx, id)
		require.NoError(t, err)
		room.FanClubOnly = true
		room.StreamKey = "key-" + id
		require.NoError(t, f.rooms.Upsert(ctx, room))
	}
	require.NoError(t, f.db.Exec(
		"INSERT INTO fan_badges (user_id, creator_id, total_contribution) VALUES (?, ?, 100)",
		f.viewerID, f.owners[5],
	).Error)

	var rooms []model.Room
	for _, id := range []string{"live-05", "live-06", "live-07", "replay-08"} {
		room, err := f.rooms.GetByID(ctx, id)
		require.NoError(t, err)
		rooms = append(rooms, *room)
	}
	// An owner without a users row keeps the stored channel name.
	rooms = append(rooms, model.Room{ID: "orphan", OwnerID: "no-such-user", Channel: "Stored", ChannelID: "ch-orphan", Status: model.StatusLive})

	batched, err := f.svc.streamsFromRooms(ctx, rooms, f.now, f.viewerID)
	require.NoError(t, err)
	require.Len(t, batched, len(rooms))
	for i := range rooms {
		single := f.svc.streamFromRoom(ctx, &rooms[i], f.now, f.viewerID)
		require.NoError(t, f.svc.addSubscriberCount(ctx, &single))
		require.Equal(t, single, batched[i], rooms[i].ID)
	}
	require.True(t, batched[0].FanClubMember)
	require.NotEmpty(t, batched[0].PlaybackURL)
	require.False(t, batched[1].FanClubMember)
	require.Empty(t, batched[1].PlaybackURL, "fan-club-only playback is hidden from non-members")
	require.Equal(t, "Stored", batched[4].Channel)
}

func TestRecommendedLiveAppliesViewerBlocksToCachedPool(t *testing.T) {
	ctx := context.Background()
	f := newListPerfFixture(t)
	room, err := f.rooms.GetByID(ctx, "live-00")
	require.NoError(t, err)
	room.Category = "Chess"
	require.NoError(t, f.rooms.Upsert(ctx, room))

	ids := func(resp *ListResp) []string {
		out := []string{}
		for _, item := range resp.Items {
			out = append(out, item.ID)
		}
		return out
	}
	anonymous, err := f.svc.RecommendedLive(ctx, "", "Chess", 12)
	require.NoError(t, err)
	require.Equal(t, []string{"live-00"}, ids(anonymous))

	blocked, err := f.svc.RecommendedLive(ctx, f.viewerID, "Chess", 12)
	require.NoError(t, err)
	require.Empty(t, blocked.Items, "blocks apply per viewer on top of the cached pool")

	require.NoError(t, f.messages.DeleteBlock(ctx, f.viewerID, f.owners[0]))
	unblocked, err := f.svc.RecommendedLive(ctx, f.viewerID, "Chess", 12)
	require.NoError(t, err)
	require.Equal(t, []string{"live-00"}, ids(unblocked), "unblocking takes effect without waiting for the cache")

	// New rooms join the pool once the cached entry expires.
	room, err = f.rooms.GetByID(ctx, "live-01")
	require.NoError(t, err)
	room.Category = "Chess"
	require.NoError(t, f.rooms.Upsert(ctx, room))
	cached, err := f.svc.RecommendedLive(ctx, "", "Chess", 12)
	require.NoError(t, err)
	require.Equal(t, []string{"live-00"}, ids(cached))
	f.svc.liveRecommendations.now = func() time.Time { return time.Now().Add(liveRecommendationCacheTTL) }
	refreshed, err := f.svc.RecommendedLive(ctx, "", "Chess", 12)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"live-00", "live-01"}, ids(refreshed))
}

func TestHotReplaysAppliesViewerVisibilityToCachedPool(t *testing.T) {
	ctx := context.Background()
	f := newListPerfFixture(t)
	for id, visibility := range map[string]string{
		"replay-00": model.PostVisibilityPublic,
		"replay-02": model.PostVisibilityFollowers,
	} {
		room, err := f.rooms.GetByID(ctx, id)
		require.NoError(t, err)
		room.Category = "Chess"
		room.ReplayVisibility = visibility
		require.NoError(t, f.rooms.Upsert(ctx, room))
	}
	ids := func(resp *HotReplayResp) []string {
		out := []string{}
		for _, item := range resp.Items {
			out = append(out, item.ID)
		}
		return out
	}

	anonymous, err := f.svc.HotReplays(ctx, "", "Chess", 3, 12)
	require.NoError(t, err)
	require.Equal(t, []string{"replay-00"}, ids(anonymous))
	require.Equal(t, int64(1), anonymous.Items[0].SubscriberCount)
	require.Equal(t, int64(1), anonymous.Items[0].CommentCount)

	viewer, err := f.svc.HotReplays(ctx, f.viewerID, "Chess", 3, 12)
	require.NoError(t, err)
	require.Empty(t, viewer.Items, "owner 0 is blocked and the viewer doesn't follow owner 2")

	require.NoError(t, f.social.Follow(ctx, f.viewerID, "ch-"+f.owners[2]))
	following, err := f.svc.HotReplays(ctx, f.viewerID, "Chess", 3, 12)
	require.NoError(t, err)
	require.Equal(t, []string{"replay-02"}, ids(following))
}
