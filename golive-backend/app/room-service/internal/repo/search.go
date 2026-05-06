package repo

import (
	"context"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"gorm.io/gorm"
)

const maxSearchRunes = 80

type SearchPhrase struct {
	Raw     string
	Tokens  []string
	Compact string
}

type CreatorSearchRow struct {
	ID          string
	Username    string
	DisplayName string
	Avatar      string
	Cover       string
	Verified    bool
	UpdatedAt   time.Time
	ChannelID   string
	Channel     string
	LiveRoomID  string
	LastLiveAt  *time.Time
	LastTitle   string
}

func roomsFullTextMatch(alias string) string {
	return "MATCH(" + alias + ".title, " + alias + ".title_ja, " + alias + ".description, " + alias + ".category, " + alias + ".category_ja, " + alias + ".channel, " + alias + ".channel_id) AGAINST (? IN BOOLEAN MODE)"
}

func usersFullTextMatch(alias string) string {
	return "MATCH(" + alias + ".username, " + alias + ".display_name, " + alias + ".id) AGAINST (? IN BOOLEAN MODE)"
}

func appointmentsFullTextMatch(alias string) string {
	return "MATCH(" + alias + ".title, " + alias + ".description) AGAINST (? IN BOOLEAN MODE)"
}

func postsFullTextMatch(alias string) string {
	return "MATCH(" + alias + ".content, " + alias + ".channel_id) AGAINST (? IN BOOLEAN MODE)"
}

func NewSearchPhrase(raw string) SearchPhrase {
	cleaned := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	cleaned = trimRunes(cleaned, maxSearchRunes)
	tokens := make([]string, 0, 4)
	for _, token := range strings.FieldsFunc(cleaned, isTokenSeparator) {
		token = strings.ToLower(strings.TrimSpace(token))
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	if len(tokens) == 0 && cleaned != "" {
		tokens = []string{strings.ToLower(cleaned)}
	}
	return SearchPhrase{
		Raw:     cleaned,
		Tokens:  tokens,
		Compact: compactSearchText(cleaned),
	}
}

func (p SearchPhrase) Empty() bool {
	return strings.TrimSpace(p.Raw) == ""
}

func (r *RoomRepo) SearchCreators(ctx context.Context, phrase SearchPhrase, limit int) ([]CreatorSearchRow, error) {
	if phrase.Empty() {
		return []CreatorSearchRow{}, nil
	}
	limit = normalizeSearchLimit(limit)
	if query, ok := mysqlBooleanSearchQuery(phrase); ok && r.db.Dialector.Name() == "mysql" {
		where := "(" + usersFullTextMatch("u") + " OR EXISTS (SELECT 1 FROM rooms cr WHERE cr.owner_id = u.id AND cr.owner_id <> '' AND " + roomsFullTextMatch("cr") + "))"
		var rows []CreatorSearchRow
		err := r.creatorSearchBaseQuery(ctx).
			Where(where, query, query).
			Order("u.updated_at DESC").
			Limit(limit).
			Scan(&rows).Error
		if err == nil {
			return rows, nil
		}
		if !shouldFallbackFromFullText(err) {
			return nil, err
		}
	}
	userWhere, userArgs := fuzzyWhere([]string{"u.username", "u.display_name", "u.id"}, phrase)
	roomWhere, roomArgs := fuzzyWhere([]string{"cr.channel", "cr.channel_id", "cr.title", "cr.title_ja", "cr.category", "cr.category_ja"}, phrase)
	where := "(" + userWhere + " OR EXISTS (SELECT 1 FROM rooms cr WHERE cr.owner_id = u.id AND cr.owner_id <> '' AND " + roomWhere + "))"
	args := append([]any{}, userArgs...)
	args = append(args, roomArgs...)

	var rows []CreatorSearchRow
	err := r.creatorSearchBaseQuery(ctx).
		Where(where, args...).
		Order("u.updated_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if isMissingTable(err) {
		return []CreatorSearchRow{}, nil
	}
	return rows, err
}

func (r *RoomRepo) creatorSearchBaseQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("users AS u").
		Select(`
			u.id,
			COALESCE(u.username, '') AS username,
			COALESCE(u.display_name, '') AS display_name,
			COALESCE(u.avatar, '') AS avatar,
			COALESCE(u.cover, '') AS cover,
			u.verified,
			u.updated_at,
			COALESCE((
				SELECT r.channel_id FROM rooms r
				WHERE r.owner_id = u.id AND r.owner_id <> ''
				ORDER BY r.updated_at DESC, r.started_at DESC
				LIMIT 1
			), '') AS channel_id,
			COALESCE((
				SELECT r.channel FROM rooms r
				WHERE r.owner_id = u.id AND r.owner_id <> ''
				ORDER BY r.updated_at DESC, r.started_at DESC
				LIMIT 1
			), '') AS channel,
			COALESCE((
				SELECT r.id FROM rooms r
				WHERE r.owner_id = u.id AND r.status = ?
				ORDER BY r.started_at DESC, r.created_at DESC
				LIMIT 1
			), '') AS live_room_id,
			(
				SELECT r.started_at FROM rooms r
				WHERE r.owner_id = u.id AND r.status IN ?
				ORDER BY r.started_at DESC, r.created_at DESC
				LIMIT 1
			) AS last_live_at,
			COALESCE((
				SELECT r.title FROM rooms r
				WHERE r.owner_id = u.id AND r.status IN ?
				ORDER BY r.started_at DESC, r.created_at DESC
				LIMIT 1
			), '') AS last_title
		`, model.StatusLive, []string{model.StatusLive, model.StatusEnded}, []string{model.StatusLive, model.StatusEnded}).
		Where("u.live_permission_status = ?", "approved")
}

func (r *RoomRepo) SearchLiveRooms(ctx context.Context, phrase SearchPhrase, limit int) ([]model.Room, error) {
	if phrase.Empty() {
		return []model.Room{}, nil
	}
	limit = normalizeSearchLimit(limit)
	if query, ok := mysqlBooleanSearchQuery(phrase); ok && r.db.Dialector.Name() == "mysql" {
		where := "(" + roomsFullTextMatch("rooms") + " OR " + usersFullTextMatch("u") + ")"
		var rooms []model.Room
		err := r.liveRoomSearchBaseQuery(ctx).
			Where(where, query, query).
			Order("rooms.started_at DESC, rooms.viewers DESC").
			Limit(limit).
			Find(&rooms).Error
		if err == nil {
			return rooms, nil
		}
		if !shouldFallbackFromFullText(err) {
			return nil, err
		}
	}
	where, args := fuzzyWhere([]string{
		"rooms.title",
		"rooms.title_ja",
		"rooms.description",
		"rooms.category",
		"rooms.category_ja",
		"rooms.channel",
		"rooms.channel_id",
		"u.username",
		"u.display_name",
	}, phrase)
	var rooms []model.Room
	err := r.liveRoomSearchBaseQuery(ctx).
		Where(where, args...).
		Order("rooms.started_at DESC, rooms.viewers DESC").
		Limit(limit).
		Find(&rooms).Error
	if isMissingTable(err) {
		return []model.Room{}, nil
	}
	return rooms, err
}

func (r *RoomRepo) liveRoomSearchBaseQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&model.Room{}).
		Select("rooms.*").
		Joins("LEFT JOIN users u ON u.id = rooms.owner_id").
		Where("rooms.status = ? AND rooms.owner_id <> ?", model.StatusLive, "")
}

func (r *RoomRepo) SearchReplayRooms(ctx context.Context, phrase SearchPhrase, limit int) ([]model.Room, error) {
	if phrase.Empty() {
		return []model.Room{}, nil
	}
	limit = normalizeSearchLimit(limit)
	if query, ok := mysqlBooleanSearchQuery(phrase); ok && r.db.Dialector.Name() == "mysql" {
		where := "(" + roomsFullTextMatch("rooms") + " OR " + usersFullTextMatch("u") + ")"
		var rooms []model.Room
		err := r.replayRoomSearchBaseQuery(ctx).
			Where(where, query, query).
			Order("COALESCE(rooms.replay_uploaded_at, rooms.ended_at, rooms.updated_at) DESC").
			Limit(limit).
			Find(&rooms).Error
		if err == nil {
			return rooms, nil
		}
		if !shouldFallbackFromFullText(err) {
			return nil, err
		}
	}
	where, args := fuzzyWhere([]string{
		"rooms.title",
		"rooms.title_ja",
		"rooms.description",
		"rooms.category",
		"rooms.category_ja",
		"rooms.channel",
		"rooms.channel_id",
		"u.username",
		"u.display_name",
	}, phrase)
	var rooms []model.Room
	err := r.replayRoomSearchBaseQuery(ctx).
		Where(where, args...).
		Order("COALESCE(rooms.replay_uploaded_at, rooms.ended_at, rooms.updated_at) DESC").
		Limit(limit).
		Find(&rooms).Error
	if isMissingTable(err) {
		return []model.Room{}, nil
	}
	return rooms, err
}

func (r *RoomRepo) replayRoomSearchBaseQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Model(&model.Room{}).
		Select("rooms.*").
		Joins("LEFT JOIN users u ON u.id = rooms.owner_id").
		Where("rooms.status = ? AND rooms.replay_status = ? AND rooms.replay_bunny_video_id <> ?", model.StatusEnded, model.ReplayStatusReady, "").
		Where("rooms.replay_visibility IN ?", []string{model.PostVisibilityPublic, model.PostVisibilityFollowers})
}

func (r *AppointmentRepo) SearchPublicUpcoming(ctx context.Context, phrase SearchPhrase, now time.Time, limit int) ([]model.LiveAppointment, error) {
	if phrase.Empty() {
		return []model.LiveAppointment{}, nil
	}
	limit = normalizeSearchLimit(limit)
	if query, ok := mysqlBooleanSearchQuery(phrase); ok && r.db.Dialector.Name() == "mysql" {
		where := "(" + appointmentsFullTextMatch("live_appointments") + " OR " + roomsFullTextMatch("rooms") + " OR " + usersFullTextMatch("u") + ")"
		var items []model.LiveAppointment
		err := publicAppointmentQuery(r.db.WithContext(ctx), now).
			Joins("JOIN rooms ON rooms.id = live_appointments.room_id").
			Joins("LEFT JOIN users u ON u.id = live_appointments.owner_id").
			Where(where, query, query, query).
			Order("live_appointments.scheduled_at ASC, live_appointments.created_at ASC").
			Limit(limit).
			Find(&items).Error
		if err == nil {
			return items, nil
		}
		if !shouldFallbackFromFullText(err) {
			return nil, err
		}
	}
	where, args := fuzzyWhere([]string{
		"live_appointments.title",
		"live_appointments.description",
		"rooms.title",
		"rooms.description",
		"rooms.category",
		"rooms.category_ja",
		"rooms.channel",
		"rooms.channel_id",
		"u.username",
		"u.display_name",
	}, phrase)
	var items []model.LiveAppointment
	err := publicAppointmentQuery(r.db.WithContext(ctx), now).
		Joins("JOIN rooms ON rooms.id = live_appointments.room_id").
		Joins("LEFT JOIN users u ON u.id = live_appointments.owner_id").
		Where(where, args...).
		Order("live_appointments.scheduled_at ASC, live_appointments.created_at ASC").
		Limit(limit).
		Find(&items).Error
	if isMissingTable(err) {
		return []model.LiveAppointment{}, nil
	}
	return items, err
}

func (r *PostRepo) SearchVisible(ctx context.Context, phrase SearchPhrase, limit int) ([]model.ChannelPost, error) {
	if phrase.Empty() {
		return []model.ChannelPost{}, nil
	}
	limit = normalizeSearchLimit(limit)
	if query, ok := mysqlBooleanSearchQuery(phrase); ok && r.db.Dialector.Name() == "mysql" {
		where := "(" + postsFullTextMatch("channel_posts") + " OR " + usersFullTextMatch("u") + ")"
		var posts []model.ChannelPost
		err := r.db.WithContext(ctx).
			Model(&model.ChannelPost{}).
			Select("channel_posts.*").
			Joins("LEFT JOIN users u ON u.id = channel_posts.owner_id").
			Where("channel_posts.visibility IN ?", []string{model.PostVisibilityPublic, model.PostVisibilityFollowers}).
			Where(where, query, query).
			Order("channel_posts.created_at DESC").
			Limit(limit).
			Find(&posts).Error
		if err == nil {
			return posts, nil
		}
		if !shouldFallbackFromFullText(err) {
			return nil, err
		}
	}
	where, args := fuzzyWhere([]string{
		"channel_posts.content",
		"channel_posts.channel_id",
		"u.username",
		"u.display_name",
	}, phrase)
	var posts []model.ChannelPost
	err := r.db.WithContext(ctx).
		Model(&model.ChannelPost{}).
		Select("channel_posts.*").
		Joins("LEFT JOIN users u ON u.id = channel_posts.owner_id").
		Where("channel_posts.visibility IN ?", []string{model.PostVisibilityPublic, model.PostVisibilityFollowers}).
		Where(where, args...).
		Order("channel_posts.created_at DESC").
		Limit(limit).
		Find(&posts).Error
	if isMissingTable(err) {
		return []model.ChannelPost{}, nil
	}
	return posts, err
}

func fuzzyWhere(fields []string, phrase SearchPhrase) (string, []any) {
	if phrase.Empty() || len(fields) == 0 {
		return "1 = 0", nil
	}
	args := make([]any, 0, len(fields)*len(phrase.Tokens)+len(fields))
	tokenGroups := make([]string, 0, len(phrase.Tokens))
	for _, token := range phrase.Tokens {
		pattern := likePattern(token)
		fieldClauses := make([]string, 0, len(fields))
		for _, field := range fields {
			fieldClauses = append(fieldClauses, "LOWER(COALESCE("+field+", '')) LIKE ? ESCAPE '!'")
			args = append(args, pattern)
		}
		tokenGroups = append(tokenGroups, "("+strings.Join(fieldClauses, " OR ")+")")
	}
	clauses := make([]string, 0, 2)
	if len(tokenGroups) > 0 {
		clauses = append(clauses, "("+strings.Join(tokenGroups, " AND ")+")")
	}
	if phrase.Compact != "" {
		pattern := likePattern(phrase.Compact)
		fieldClauses := make([]string, 0, len(fields))
		for _, field := range fields {
			fieldClauses = append(fieldClauses, compactSQL(field)+" LIKE ? ESCAPE '!'")
			args = append(args, pattern)
		}
		clauses = append(clauses, "("+strings.Join(fieldClauses, " OR ")+")")
	}
	if len(clauses) == 0 {
		return "1 = 0", nil
	}
	return "(" + strings.Join(clauses, " OR ") + ")", args
}

func mysqlBooleanSearchQuery(phrase SearchPhrase) (string, bool) {
	if phrase.Empty() || len(phrase.Tokens) == 0 {
		return "", false
	}
	terms := make([]string, 0, len(phrase.Tokens))
	for _, token := range phrase.Tokens {
		parts, ok := asciiFullTextParts(token)
		if !ok || len(parts) == 0 {
			return "", false
		}
		for _, part := range parts {
			if len(part) < 3 {
				return "", false
			}
			terms = append(terms, "+"+part+"*")
		}
	}
	if len(terms) == 0 {
		return "", false
	}
	return strings.Join(terms, " "), true
}

func asciiFullTextParts(value string) ([]string, bool) {
	parts := make([]string, 0, 2)
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if r > unicode.MaxASCII {
			return nil, false
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		if b.Len() > 0 {
			parts = append(parts, b.String())
			b.Reset()
		}
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts, true
}

func shouldFallbackFromFullText(err error) bool {
	if err == nil {
		return false
	}
	if isMissingTable(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "fulltext") ||
		strings.Contains(msg, "match") && strings.Contains(msg, "against")
}

func compactSQL(field string) string {
	return "REPLACE(REPLACE(REPLACE(REPLACE(LOWER(COALESCE(" + field + ", '')), ' ', ''), '_', ''), '-', ''), '.', '')"
}

func likePattern(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "!", "!!")
	value = strings.ReplaceAll(value, "%", "!%")
	value = strings.ReplaceAll(value, "_", "!_")
	return "%" + value + "%"
}

func compactSearchText(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range strings.ToLower(value) {
		if isCompactSeparator(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isTokenSeparator(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune(",，;；/|+*?？!！:：", r)
}

func isCompactSeparator(r rune) bool {
	return unicode.IsSpace(r) || r == '_' || r == '-' || r == '.' || r == '·' || r == '/' || r == '\\'
}

func trimRunes(value string, max int) string {
	if max <= 0 || utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max])
}

func normalizeSearchLimit(limit int) int {
	if limit < 1 {
		return 10
	}
	if limit > 100 {
		return 100
	}
	return limit
}
