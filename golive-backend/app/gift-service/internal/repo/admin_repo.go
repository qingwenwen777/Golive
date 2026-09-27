package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
)

type AdminRepo struct{ db *gorm.DB }

func NewAdminRepo(db *gorm.DB) *AdminRepo { return &AdminRepo{db: db} }

var ErrInvalidGiftPrice = errors.New("invalid gift price")

type AdminGiftStats struct {
	Total        int64 `json:"total"`
	Enabled      int64 `json:"enabled"`
	Disabled     int64 `json:"disabled"`
	CatalogValue int64 `json:"catalogValue"`
}

type AdminGiftPatch struct {
	PriceCoin *int64
	Enabled   *bool
}

type AdminEconomySummary struct {
	GiftCount              int64 `json:"giftCount"`
	EnabledGiftCount       int64 `json:"enabledGiftCount"`
	TodayGiftRevenue       int64 `json:"todayGiftRevenue"`
	TodaySuperChatRevenue  int64 `json:"todaySuperChatRevenue"`
	TodayTopupCoins        int64 `json:"todayTopupCoins"`
	TodaySpendCoins        int64 `json:"todaySpendCoins"`
	TodayBetTurnover       int64 `json:"todayBetTurnover"`
	TotalCoinBalance       int64 `json:"totalCoinBalance"`
	TotalFrozenCoins       int64 `json:"totalFrozenCoins"`
	OpenBetRounds          int64 `json:"openBetRounds"`
	UnsettledBetRounds     int64 `json:"unsettledBetRounds"`
	PendingSettlementCoins int64 `json:"pendingSettlementCoins"`
}

type AdminOrderFilter struct {
	Type   string
	Status string
	Q      string
	Page   int
	Size   int
}

type AdminOrderRecord struct {
	Type       string    `gorm:"column:type" json:"type"`
	OrderID    string    `gorm:"column:order_id" json:"orderId"`
	UserID     string    `gorm:"column:user_id" json:"userId"`
	UserName   string    `gorm:"column:user_name" json:"userName"`
	RoomID     string    `gorm:"column:room_id" json:"roomId,omitempty"`
	RoomTitle  string    `gorm:"column:room_title" json:"roomTitle,omitempty"`
	ItemID     string    `gorm:"column:item_id" json:"itemId"`
	ItemName   string    `gorm:"column:item_name" json:"itemName"`
	Count      int       `gorm:"column:count" json:"count"`
	Amount     int64     `gorm:"column:amount" json:"amount"`
	Status     string    `gorm:"column:status" json:"status"`
	FailReason string    `gorm:"column:fail_reason" json:"failReason,omitempty"`
	Option     string    `gorm:"column:bet_option" json:"option,omitempty"`
	Result     string    `gorm:"column:result" json:"result,omitempty"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"createdAt"`
}

type AdminCoinFilter struct {
	Type string
	Q    string
	Page int
	Size int
}

type AdminCoinStats struct {
	TotalTopupCoins   int64 `json:"totalTopupCoins"`
	TotalSpendCoins   int64 `json:"totalSpendCoins"`
	TotalBalanceCoins int64 `json:"totalBalanceCoins"`
	TotalFrozenCoins  int64 `json:"totalFrozenCoins"`
	TodayTopupCoins   int64 `json:"todayTopupCoins"`
	TodaySpendCoins   int64 `json:"todaySpendCoins"`
	UserCount         int64 `json:"userCount"`
}

type AdminCoinRecord struct {
	ID             string    `gorm:"column:id" json:"id"`
	UserID         string    `gorm:"column:user_id" json:"userId"`
	UserName       string    `gorm:"column:user_name" json:"userName"`
	Type           string    `gorm:"column:type" json:"type"`
	Amount         int64     `gorm:"column:amount" json:"amount"`
	BalanceAfter   int64     `gorm:"column:balance_after" json:"balanceAfter"`
	Title          string    `gorm:"column:title" json:"title"`
	Description    string    `gorm:"column:description" json:"description,omitempty"`
	SourceType     string    `gorm:"column:source_type" json:"sourceType,omitempty"`
	SourceID       string    `gorm:"column:source_id" json:"sourceId,omitempty"`
	RoomID         string    `gorm:"column:room_id" json:"roomId,omitempty"`
	CounterpartyID string    `gorm:"column:counterparty_id" json:"counterpartyId,omitempty"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"createdAt"`
}

type AdminBetFilter struct {
	Status string
	Q      string
	Page   int
	Size   int
}

type AdminBetStats struct {
	Total       int64 `json:"total"`
	Open        int64 `json:"open"`
	Closed      int64 `json:"closed"`
	Settled     int64 `json:"settled"`
	Cancelled   int64 `json:"cancelled"`
	LockedCoins int64 `json:"lockedCoins"`
}

type AdminBetRoundRecord struct {
	ID            string     `gorm:"column:id" json:"id"`
	RoomID        string     `gorm:"column:room_id" json:"roomId"`
	RoomTitle     string     `gorm:"column:room_title" json:"roomTitle,omitempty"`
	OwnerID       string     `gorm:"column:owner_id" json:"ownerId"`
	OwnerName     string     `gorm:"column:owner_name" json:"ownerName"`
	Question      string     `gorm:"column:question" json:"question"`
	Amount        int64      `gorm:"column:amount" json:"amount"`
	Status        string     `gorm:"column:status" json:"status"`
	WinningOption string     `gorm:"column:winning_option" json:"winningOption,omitempty"`
	CloseAt       time.Time  `gorm:"column:close_at" json:"closeAt"`
	SettledAt     *time.Time `gorm:"column:settled_at" json:"settledAt,omitempty"`
	CreatedAt     time.Time  `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt     time.Time  `gorm:"column:updated_at" json:"updatedAt"`
	WagerCount    int64      `gorm:"column:wager_count" json:"wagerCount"`
	TotalPool     int64      `gorm:"column:total_pool" json:"totalPool"`
	WinCount      int64      `gorm:"column:win_count" json:"winCount"`
	LoseCount     int64      `gorm:"column:lose_count" json:"loseCount"`
	WinPool       int64      `gorm:"column:win_pool" json:"winPool"`
	LosePool      int64      `gorm:"column:lose_pool" json:"losePool"`
}

type AdminRevenueReportRow struct {
	PeriodStart    string `json:"periodStart"`
	PeriodEnd      string `json:"periodEnd"`
	TopupCoins     int64  `json:"topupCoins"`
	GiftCoins      int64  `json:"giftCoins"`
	SuperChatCoins int64  `json:"superChatCoins"`
	RevenueCoins   int64  `json:"revenueCoins"`
	BetWagerCoins  int64  `json:"betWagerCoins"`
	BetPayoutCoins int64  `json:"betPayoutCoins"`
	BetRefundCoins int64  `json:"betRefundCoins"`
	NetBetCoins    int64  `json:"netBetCoins"`
}

func (r *AdminRepo) IsAdmin(ctx context.Context, userID string) (bool, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false, nil
	}
	var role string
	err := r.db.WithContext(ctx).Raw(`
SELECT COALESCE(role, '')
FROM users
WHERE id = ?
`, userID).Row().Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(role) == "admin", nil
}

// IsBanned applies room-service's ban rule: user-service records a ban in
// both users.banned and user_moderation_states.banned, and either counts.
func (r *AdminRepo) IsBanned(ctx context.Context, userID string) (bool, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false, nil
	}
	var banned bool
	err := r.db.WithContext(ctx).Raw(`
SELECT COALESCE(banned, false)
FROM users
WHERE id = ?
`, userID).Row().Scan(&banned)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if banned {
		return true, nil
	}
	var states int64
	err = r.db.WithContext(ctx).Raw(`
SELECT COUNT(*)
FROM user_moderation_states
WHERE user_id = ? AND banned = ?
`, userID, true).Row().Scan(&states)
	// user-service creates the table; until it has, nobody is banned in it.
	if err != nil && !isMissingTable(err) {
		return false, err
	}
	return states > 0, nil
}

func (r *AdminRepo) EconomySummary(ctx context.Context) (AdminEconomySummary, error) {
	var out AdminEconomySummary
	today := startOfDay(time.Now().UTC())
	if err := r.db.WithContext(ctx).Model(&model.Gift{}).Count(&out.GiftCount).Error; err != nil {
		return out, err
	}
	if err := r.db.WithContext(ctx).Model(&model.Gift{}).Where("enabled = ?", true).Count(&out.EnabledGiftCount).Error; err != nil {
		return out, err
	}
	if err := scanInt64(r.db.WithContext(ctx).Model(&model.GiftOrder{}).
		Select("COALESCE(SUM(total_coin), 0)").
		Where("status = ? AND created_at >= ?", model.StatusSuccess, today), &out.TodayGiftRevenue); err != nil {
		return out, err
	}
	if err := scanInt64(r.db.WithContext(ctx).Model(&model.SuperChatOrder{}).
		Select("COALESCE(SUM(amount), 0)").
		Where("status = ? AND created_at >= ?", model.StatusSuccess, today), &out.TodaySuperChatRevenue); err != nil {
		return out, err
	}
	if err := scanInt64(r.db.WithContext(ctx).Model(&model.CoinTransaction{}).
		Select("COALESCE(SUM(amount), 0)").
		Where("type = ? AND amount > 0 AND created_at >= ?", model.CoinTxTopup, today), &out.TodayTopupCoins); err != nil {
		return out, err
	}
	if err := scanInt64(r.db.WithContext(ctx).Model(&model.CoinTransaction{}).
		Select("COALESCE(SUM(CASE WHEN amount < 0 THEN -amount ELSE 0 END), 0)").
		Where("type IN ? AND created_at >= ?", spendCoinTypes(), today), &out.TodaySpendCoins); err != nil {
		return out, err
	}
	if err := scanInt64(r.db.WithContext(ctx).Model(&model.CoinTransaction{}).
		Select("COALESCE(SUM(CASE WHEN amount < 0 THEN -amount ELSE 0 END), 0)").
		Where("type = ? AND created_at >= ?", model.CoinTxBetWager, today), &out.TodayBetTurnover); err != nil {
		return out, err
	}
	if err := r.db.WithContext(ctx).Raw(`
SELECT
  COALESCE(SUM(coin_balance), 0),
  COALESCE(SUM(frozen_coins), 0)
FROM users
`).Row().Scan(&out.TotalCoinBalance, &out.TotalFrozenCoins); err != nil {
		return out, err
	}
	if err := r.db.WithContext(ctx).Model(&model.BetRound{}).
		Where("status = ?", model.BetRoundOpen).
		Count(&out.OpenBetRounds).Error; err != nil {
		return out, err
	}
	if err := r.db.WithContext(ctx).Model(&model.BetRound{}).
		Where("status IN ?", []string{model.BetRoundOpen, model.BetRoundClosed}).
		Count(&out.UnsettledBetRounds).Error; err != nil {
		return out, err
	}
	if err := scanInt64(r.db.WithContext(ctx).Model(&model.BetWager{}).
		Select("COALESCE(SUM(amount), 0)").
		Where("status = ?", model.StatusLocked), &out.PendingSettlementCoins); err != nil {
		return out, err
	}
	return out, nil
}

func (r *AdminRepo) ListGifts(ctx context.Context) ([]model.Gift, AdminGiftStats, error) {
	var items []model.Gift
	var stats AdminGiftStats
	if err := r.db.WithContext(ctx).
		Order("enabled DESC, category ASC, price_coin ASC, id ASC").
		Find(&items).Error; err != nil {
		return nil, stats, err
	}
	stats.Total = int64(len(items))
	for _, gift := range items {
		if gift.Enabled {
			stats.Enabled++
			stats.CatalogValue += gift.PriceCoin
		} else {
			stats.Disabled++
		}
	}
	return items, stats, nil
}

func (r *AdminRepo) UpdateGift(ctx context.Context, id string, patch AdminGiftPatch) (*model.Gift, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrGiftNotFound
	}
	var existing model.Gift
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrGiftNotFound
	}
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if patch.PriceCoin != nil {
		price := *patch.PriceCoin
		if price <= 0 || price > 1_000_000_000 {
			return nil, ErrInvalidGiftPrice
		}
		updates["price_coin"] = price
	}
	if patch.Enabled != nil {
		updates["enabled"] = *patch.Enabled
	}
	if len(updates) > 0 {
		if err := r.db.WithContext(ctx).Model(&model.Gift{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	var out model.Gift
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&out).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

var ErrSuperChatNotFound = errors.New("super chat not found")

// ModerateSuperChat hides a super chat from public history by setting
// moderated_at. It never touches status, amount or the coin ledger: a paid
// super chat stays "success", so the payer is not refunded and the
// creator's income and revenue reports are unchanged. Refunds are a
// separate policy decision. Repeat calls keep the first moderation.
func (r *AdminRepo) ModerateSuperChat(ctx context.Context, orderID, operatorID string, now time.Time) (*model.SuperChatOrder, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, ErrSuperChatNotFound
	}
	err := r.db.WithContext(ctx).Model(&model.SuperChatOrder{}).
		Where("order_id = ? AND moderated_at IS NULL", orderID).
		Updates(map[string]any{
			"moderated_at": now,
			"moderated_by": strings.TrimSpace(operatorID),
		}).Error
	if err != nil {
		return nil, err
	}
	var out model.SuperChatOrder
	err = r.db.WithContext(ctx).Where("order_id = ?", orderID).Take(&out).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSuperChatNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *AdminRepo) ListOrders(ctx context.Context, f AdminOrderFilter) ([]AdminOrderRecord, int64, int, int, error) {
	page, size := normalizeAdminPage(f.Page, f.Size)
	parts := adminOrderParts(f.Type)
	base := "FROM (" + strings.Join(parts, " UNION ALL ") + ") AS x"
	where, args := adminOrderWhere(f)
	var total int64
	if err := r.db.WithContext(ctx).Raw("SELECT COUNT(*) "+base+where, args...).Scan(&total).Error; err != nil {
		return nil, 0, page, size, err
	}
	items := make([]AdminOrderRecord, 0)
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, size, (page-1)*size)
	if err := r.db.WithContext(ctx).Raw("SELECT * "+base+where+" ORDER BY created_at DESC LIMIT ? OFFSET ?", queryArgs...).Scan(&items).Error; err != nil {
		return nil, 0, page, size, err
	}
	return items, total, page, size, nil
}

func (r *AdminRepo) CoinLedger(ctx context.Context, f AdminCoinFilter) ([]AdminCoinRecord, int64, int, int, AdminCoinStats, error) {
	page, size := normalizeAdminPage(f.Page, f.Size)
	stats, err := r.coinStats(ctx)
	if err != nil {
		return nil, 0, page, size, stats, err
	}
	q := r.db.WithContext(ctx).Table("coin_transactions ct").
		Joins("LEFT JOIN users u ON u.id = ct.user_id")
	if typ := strings.TrimSpace(f.Type); typ != "" && typ != "all" {
		q = q.Where("ct.type = ?", typ)
	}
	if search := strings.TrimSpace(f.Q); search != "" {
		like := "%" + search + "%"
		q = q.Where(`
ct.id LIKE ? OR ct.user_id LIKE ? OR COALESCE(u.display_name, '') LIKE ? OR COALESCE(u.username, '') LIKE ? OR
ct.title LIKE ? OR ct.description LIKE ? OR ct.source_id LIKE ?
`, like, like, like, like, like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, page, size, stats, err
	}
	items := make([]AdminCoinRecord, 0)
	if err := q.Select(`
ct.id,
ct.user_id,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), ct.user_id) AS user_name,
ct.type,
ct.amount,
ct.balance_after,
ct.title,
ct.description,
ct.source_type,
ct.source_id,
ct.room_id,
ct.counterparty_id,
ct.created_at
`).Order("ct.created_at DESC, ct.id DESC").
		Limit(size).
		Offset((page - 1) * size).
		Scan(&items).Error; err != nil {
		return nil, 0, page, size, stats, err
	}
	return items, total, page, size, stats, nil
}

func (r *AdminRepo) ListBetRounds(ctx context.Context, f AdminBetFilter) ([]AdminBetRoundRecord, int64, int, int, AdminBetStats, error) {
	page, size := normalizeAdminPage(f.Page, f.Size)
	stats, err := r.betStats(ctx)
	if err != nil {
		return nil, 0, page, size, stats, err
	}
	q := r.adminBetRoundQuery(ctx, f)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, page, size, stats, err
	}
	items := make([]AdminBetRoundRecord, 0)
	if err := q.Select(adminBetRoundSelectSQL()).
		Order("br.created_at DESC, br.id DESC").
		Limit(size).
		Offset((page - 1) * size).
		Scan(&items).Error; err != nil {
		return nil, 0, page, size, stats, err
	}
	return items, total, page, size, stats, nil
}

func (r *AdminRepo) BetRoundDetail(ctx context.Context, roundID string) (*AdminBetRoundRecord, error) {
	var out AdminBetRoundRecord
	err := r.adminBetRoundQuery(ctx, AdminBetFilter{}).
		Where("br.id = ?", roundID).
		Select(adminBetRoundSelectSQL()).
		Take(&out).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrBetRoundNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *AdminRepo) RevenueReport(ctx context.Context, period string, limit int) ([]AdminRevenueReportRow, error) {
	period = normalizePeriod(period)
	starts := reportPeriodStarts(period, limit)
	rows := make([]AdminRevenueReportRow, len(starts))
	byKey := make(map[string]*AdminRevenueReportRow, len(starts))
	for i, start := range starts {
		end := addPeriod(start, period, 1)
		rows[i] = AdminRevenueReportRow{
			PeriodStart: start.Format(time.RFC3339),
			PeriodEnd:   end.Format(time.RFC3339),
		}
		byKey[start.Format(time.RFC3339)] = &rows[i]
	}
	if len(starts) == 0 {
		return rows, nil
	}
	from := starts[0]
	if err := r.addCoinReportRows(ctx, byKey, period, from); err != nil {
		return nil, err
	}
	if err := r.addGiftReportRows(ctx, byKey, period, from); err != nil {
		return nil, err
	}
	if err := r.addSuperChatReportRows(ctx, byKey, period, from); err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].RevenueCoins = rows[i].GiftCoins + rows[i].SuperChatCoins
		rows[i].NetBetCoins = rows[i].BetWagerCoins - rows[i].BetPayoutCoins - rows[i].BetRefundCoins
	}
	return rows, nil
}

func normalizeAdminPage(page, size int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

func adminOrderParts(orderType string) []string {
	orderType = strings.TrimSpace(orderType)
	if orderType == "" || orderType == "all" {
		orderType = "all"
	}
	parts := make([]string, 0, 3)
	if orderType == "all" || orderType == "gift" {
		parts = append(parts, `
SELECT
  'gift' AS type,
  go.order_id,
  go.user_id,
  COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), go.user_id) AS user_name,
  COALESCE(go.room_id, '') AS room_id,
  COALESCE(r.title, '') AS room_title,
  go.gift_id AS item_id,
  COALESCE(NULLIF(g.name, ''), go.gift_id) AS item_name,
  go.count AS count,
  go.total_coin AS amount,
  go.status,
  go.fail_reason,
  '' AS bet_option,
  '' AS result,
  go.created_at
FROM gift_orders go
LEFT JOIN users u ON u.id = go.user_id
LEFT JOIN rooms r ON r.id = go.room_id
LEFT JOIN gifts g ON g.id = go.gift_id`)
	}
	if orderType == "all" || orderType == "super_chat" {
		parts = append(parts, `
SELECT
  'super_chat' AS type,
  so.order_id,
  so.user_id,
  COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), so.user_id) AS user_name,
  COALESCE(so.room_id, '') AS room_id,
  COALESCE(r.title, '') AS room_title,
  'super_chat' AS item_id,
  'SuperChat' AS item_name,
  1 AS count,
  so.amount AS amount,
  so.status,
  so.fail_reason,
  '' AS bet_option,
  '' AS result,
  so.created_at
FROM super_chat_orders so
LEFT JOIN users u ON u.id = so.user_id
LEFT JOIN rooms r ON r.id = so.room_id`)
	}
	if orderType == "all" || orderType == "bet" {
		parts = append(parts, `
SELECT
  'bet' AS type,
  bw.id AS order_id,
  bw.user_id,
  COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), bw.user_id) AS user_name,
  COALESCE(bw.room_id, '') AS room_id,
  COALESCE(r.title, '') AS room_title,
  bw.round_id AS item_id,
  COALESCE(NULLIF(br.question, ''), bw.round_id) AS item_name,
  1 AS count,
  bw.amount AS amount,
  bw.status,
  '' AS fail_reason,
  bw.`+"`option`"+` AS bet_option,
  br.winning_option AS result,
  bw.created_at
FROM bet_wagers bw
LEFT JOIN users u ON u.id = bw.user_id
LEFT JOIN rooms r ON r.id = bw.room_id
LEFT JOIN bet_rounds br ON br.id = bw.round_id`)
	}
	if len(parts) == 0 {
		return adminOrderParts("all")
	}
	return parts
}

func adminOrderWhere(f AdminOrderFilter) (string, []any) {
	wheres := make([]string, 0, 2)
	args := make([]any, 0)
	if status := strings.TrimSpace(f.Status); status != "" && status != "all" {
		wheres = append(wheres, "status = ?")
		args = append(args, status)
	}
	if search := strings.TrimSpace(f.Q); search != "" {
		like := "%" + search + "%"
		wheres = append(wheres, `(order_id LIKE ? OR user_id LIKE ? OR user_name LIKE ? OR room_id LIKE ? OR room_title LIKE ? OR item_id LIKE ? OR item_name LIKE ?)`)
		args = append(args, like, like, like, like, like, like, like)
	}
	if len(wheres) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(wheres, " AND "), args
}

func (r *AdminRepo) coinStats(ctx context.Context) (AdminCoinStats, error) {
	var stats AdminCoinStats
	today := startOfDay(time.Now().UTC())
	if err := r.db.WithContext(ctx).Raw(`
SELECT
  COALESCE(SUM(coin_balance), 0),
  COALESCE(SUM(frozen_coins), 0),
  COUNT(*)
FROM users
`).Row().Scan(&stats.TotalBalanceCoins, &stats.TotalFrozenCoins, &stats.UserCount); err != nil {
		return stats, err
	}
	if err := scanInt64(r.db.WithContext(ctx).Model(&model.CoinTransaction{}).
		Select("COALESCE(SUM(amount), 0)").
		Where("type = ? AND amount > 0", model.CoinTxTopup), &stats.TotalTopupCoins); err != nil {
		return stats, err
	}
	if err := scanInt64(r.db.WithContext(ctx).Model(&model.CoinTransaction{}).
		Select("COALESCE(SUM(CASE WHEN amount < 0 THEN -amount ELSE 0 END), 0)").
		Where("type IN ?", spendCoinTypes()), &stats.TotalSpendCoins); err != nil {
		return stats, err
	}
	if err := scanInt64(r.db.WithContext(ctx).Model(&model.CoinTransaction{}).
		Select("COALESCE(SUM(amount), 0)").
		Where("type = ? AND amount > 0 AND created_at >= ?", model.CoinTxTopup, today), &stats.TodayTopupCoins); err != nil {
		return stats, err
	}
	if err := scanInt64(r.db.WithContext(ctx).Model(&model.CoinTransaction{}).
		Select("COALESCE(SUM(CASE WHEN amount < 0 THEN -amount ELSE 0 END), 0)").
		Where("type IN ? AND created_at >= ?", spendCoinTypes(), today), &stats.TodaySpendCoins); err != nil {
		return stats, err
	}
	return stats, nil
}

func (r *AdminRepo) betStats(ctx context.Context) (AdminBetStats, error) {
	var stats AdminBetStats
	count := func(status string) (int64, error) {
		var n int64
		q := r.db.WithContext(ctx).Model(&model.BetRound{})
		if status != "" {
			q = q.Where("status = ?", status)
		}
		return n, q.Count(&n).Error
	}
	var err error
	if stats.Total, err = count(""); err != nil {
		return stats, err
	}
	if stats.Open, err = count(model.BetRoundOpen); err != nil {
		return stats, err
	}
	if stats.Closed, err = count(model.BetRoundClosed); err != nil {
		return stats, err
	}
	if stats.Settled, err = count(model.BetRoundSettled); err != nil {
		return stats, err
	}
	if stats.Cancelled, err = count(model.BetRoundCancelled); err != nil {
		return stats, err
	}
	if err := scanInt64(r.db.WithContext(ctx).Model(&model.BetWager{}).
		Select("COALESCE(SUM(amount), 0)").
		Where("status = ?", model.StatusLocked), &stats.LockedCoins); err != nil {
		return stats, err
	}
	return stats, nil
}

func (r *AdminRepo) adminBetRoundQuery(ctx context.Context, f AdminBetFilter) *gorm.DB {
	q := r.db.WithContext(ctx).Table("bet_rounds br").
		Joins("LEFT JOIN rooms ro ON ro.id = br.room_id").
		Joins("LEFT JOIN users u ON u.id = br.owner_id").
		Joins("LEFT JOIN (\n" +
			"SELECT\n" +
			"  round_id,\n" +
			"  COUNT(*) AS wager_count,\n" +
			"  COALESCE(SUM(amount), 0) AS total_pool,\n" +
			"  COALESCE(SUM(CASE WHEN `option` = 'win' THEN 1 ELSE 0 END), 0) AS win_count,\n" +
			"  COALESCE(SUM(CASE WHEN `option` = 'lose' THEN 1 ELSE 0 END), 0) AS lose_count,\n" +
			"  COALESCE(SUM(CASE WHEN `option` = 'win' THEN amount ELSE 0 END), 0) AS win_pool,\n" +
			"  COALESCE(SUM(CASE WHEN `option` = 'lose' THEN amount ELSE 0 END), 0) AS lose_pool\n" +
			"FROM bet_wagers\n" +
			"GROUP BY round_id\n" +
			") bw ON bw.round_id = br.id")
	if status := strings.TrimSpace(f.Status); status != "" && status != "all" {
		q = q.Where("br.status = ?", status)
	}
	if search := strings.TrimSpace(f.Q); search != "" {
		like := "%" + search + "%"
		q = q.Where(`
br.id LIKE ? OR br.room_id LIKE ? OR br.question LIKE ? OR COALESCE(ro.title, '') LIKE ? OR
br.owner_id LIKE ? OR COALESCE(u.display_name, '') LIKE ? OR COALESCE(u.username, '') LIKE ?
`, like, like, like, like, like, like, like)
	}
	return q
}

func adminBetRoundSelectSQL() string {
	return `
br.id,
br.room_id,
COALESCE(ro.title, '') AS room_title,
br.owner_id,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), br.owner_id) AS owner_name,
br.question,
br.amount,
br.status,
br.winning_option,
br.close_at,
br.settled_at,
br.created_at,
br.updated_at,
COALESCE(bw.wager_count, 0) AS wager_count,
COALESCE(bw.total_pool, 0) AS total_pool,
COALESCE(bw.win_count, 0) AS win_count,
COALESCE(bw.lose_count, 0) AS lose_count,
COALESCE(bw.win_pool, 0) AS win_pool,
COALESCE(bw.lose_pool, 0) AS lose_pool`
}

type reportAmountRow struct {
	CreatedAt time.Time `gorm:"column:created_at"`
	Amount    int64     `gorm:"column:amount"`
	Type      string    `gorm:"column:type"`
}

func (r *AdminRepo) addCoinReportRows(ctx context.Context, byKey map[string]*AdminRevenueReportRow, period string, from time.Time) error {
	var rows []reportAmountRow
	err := r.db.WithContext(ctx).Model(&model.CoinTransaction{}).
		Select("created_at, amount, type").
		Where("created_at >= ? AND type IN ?", from, []string{
			model.CoinTxTopup,
			model.CoinTxBetWager,
			model.CoinTxBetPayout,
			model.CoinTxBetRefund,
		}).
		Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, item := range rows {
		row := byKey[bucketKey(item.CreatedAt, period)]
		if row == nil {
			continue
		}
		switch item.Type {
		case model.CoinTxTopup:
			if item.Amount > 0 {
				row.TopupCoins += item.Amount
			}
		case model.CoinTxBetWager:
			if item.Amount < 0 {
				row.BetWagerCoins += -item.Amount
			}
		case model.CoinTxBetPayout:
			if item.Amount > 0 {
				row.BetPayoutCoins += item.Amount
			}
		case model.CoinTxBetRefund:
			if item.Amount > 0 {
				row.BetRefundCoins += item.Amount
			}
		}
	}
	return nil
}

func (r *AdminRepo) addGiftReportRows(ctx context.Context, byKey map[string]*AdminRevenueReportRow, period string, from time.Time) error {
	var rows []reportAmountRow
	err := r.db.WithContext(ctx).Model(&model.GiftOrder{}).
		Select("created_at, total_coin AS amount").
		Where("created_at >= ? AND status = ?", from, model.StatusSuccess).
		Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, item := range rows {
		if row := byKey[bucketKey(item.CreatedAt, period)]; row != nil {
			row.GiftCoins += item.Amount
		}
	}
	return nil
}

func (r *AdminRepo) addSuperChatReportRows(ctx context.Context, byKey map[string]*AdminRevenueReportRow, period string, from time.Time) error {
	var rows []reportAmountRow
	err := r.db.WithContext(ctx).Model(&model.SuperChatOrder{}).
		Select("created_at, amount").
		Where("created_at >= ? AND status = ?", from, model.StatusSuccess).
		Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, item := range rows {
		if row := byKey[bucketKey(item.CreatedAt, period)]; row != nil {
			row.SuperChatCoins += item.Amount
		}
	}
	return nil
}

func scanInt64(q *gorm.DB, dest *int64) error {
	return q.Row().Scan(dest)
}

func spendCoinTypes() []string {
	return []string{
		model.CoinTxGiftSpend,
		model.CoinTxSuperChatSpend,
		model.CoinTxBetWager,
	}
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func normalizePeriod(period string) string {
	switch strings.TrimSpace(period) {
	case "week", "month":
		return strings.TrimSpace(period)
	default:
		return "day"
	}
}

func reportPeriodStarts(period string, limit int) []time.Time {
	if limit <= 0 {
		switch period {
		case "week":
			limit = 8
		case "month":
			limit = 6
		default:
			limit = 14
		}
	}
	if limit > 60 {
		limit = 60
	}
	now := periodStart(time.Now().UTC(), period)
	starts := make([]time.Time, limit)
	first := addPeriod(now, period, -(limit - 1))
	for i := 0; i < limit; i++ {
		starts[i] = addPeriod(first, period, i)
	}
	return starts
}

func bucketKey(t time.Time, period string) string {
	return periodStart(t.UTC(), period).Format(time.RFC3339)
}

func periodStart(t time.Time, period string) time.Time {
	y, m, d := t.Date()
	switch period {
	case "month":
		return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	case "week":
		day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		weekday := int(day.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return day.AddDate(0, 0, -(weekday - 1))
	default:
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
}

func addPeriod(t time.Time, period string, n int) time.Time {
	switch period {
	case "month":
		return t.AddDate(0, n, 0)
	case "week":
		return t.AddDate(0, 0, n*7)
	default:
		return t.AddDate(0, 0, n)
	}
}
