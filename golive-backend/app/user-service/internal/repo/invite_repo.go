package repo

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/qingwenwen777/golive/app/user-service/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *UserRepo) CreateInviteCode(ctx context.Context, createdBy string) (*model.InviteCode, error) {
	for i := 0; i < 8; i++ {
		code, err := randomInviteCode(10)
		if err != nil {
			return nil, err
		}
		var existing model.InviteCode
		err = r.db.WithContext(ctx).Where("code = ?", code).Take(&existing).Error
		if err == nil {
			continue
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		invite := &model.InviteCode{
			ID:        newID(),
			Code:      code,
			CreatedBy: createdBy,
		}
		if err := r.db.WithContext(ctx).Create(invite).Error; err != nil {
			return nil, err
		}
		return invite, nil
	}
	return nil, errors.New("could not generate unique invite code")
}

type InviteCodeView struct {
	ID              string     `json:"id"`
	Code            string     `json:"code"`
	CreatedBy       string     `json:"createdBy"`
	Used            bool       `json:"used"`
	UsedBy          string     `json:"usedBy,omitempty"`
	UsedUsername    string     `json:"usedUsername,omitempty"`
	UsedDisplayName string     `json:"usedDisplayName,omitempty"`
	UsedEmail       string     `json:"usedEmail,omitempty"`
	UsedAt          *time.Time `json:"usedAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
}

type InviteCodeStats struct {
	Available int64 `json:"available"`
	Used      int64 `json:"used"`
	Total     int64 `json:"total"`
}

func (r *UserRepo) ListInviteCodes(ctx context.Context) ([]InviteCodeView, error) {
	rows := make([]InviteCodeView, 0)
	err := r.db.WithContext(ctx).
		Table("invite_codes AS ic").
		Select(`ic.id, ic.code, ic.created_by, ic.used_by,
			users.username AS used_username, users.display_name AS used_display_name, users.email AS used_email,
			ic.used_at, ic.created_at`).
		Joins("LEFT JOIN users ON users.id = ic.used_by").
		Order("ic.created_at DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Used = rows[i].UsedAt != nil || rows[i].UsedBy != ""
	}
	return rows, nil
}

func (r *UserRepo) ListInviteCodesPage(ctx context.Context, page, size int) ([]InviteCodeView, int64, InviteCodeStats, error) {
	page, size = normalizeAdminPage(page, size)
	base := r.db.WithContext(ctx).
		Table("invite_codes AS ic").
		Joins("LEFT JOIN users ON users.id = ic.used_by")
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, InviteCodeStats{}, err
	}
	rows := make([]InviteCodeView, 0)
	err := base.
		Select(`ic.id, ic.code, ic.created_by, ic.used_by,
			users.username AS used_username, users.display_name AS used_display_name, users.email AS used_email,
			ic.used_at, ic.created_at`).
		Order("ic.created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, InviteCodeStats{}, err
	}
	for i := range rows {
		rows[i].Used = rows[i].UsedAt != nil || rows[i].UsedBy != ""
	}
	stats, err := r.InviteCodeStats(ctx)
	if err != nil {
		return nil, 0, InviteCodeStats{}, err
	}
	return rows, total, stats, nil
}

func (r *UserRepo) InviteCodeStats(ctx context.Context) (InviteCodeStats, error) {
	var stats InviteCodeStats
	err := r.db.WithContext(ctx).
		Table("invite_codes").
		Select(`COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN used_by <> '' OR used_at IS NOT NULL THEN 1 ELSE 0 END), 0) AS used,
			COALESCE(SUM(CASE WHEN used_by = '' AND used_at IS NULL THEN 1 ELSE 0 END), 0) AS available`).
		Scan(&stats).Error
	return stats, err
}

func (r *UserRepo) DeleteUnusedInviteCode(ctx context.Context, id string) (*model.InviteCode, error) {
	var invite model.InviteCode
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).
			Take(&invite).Error; err != nil {
			return err
		}
		if invite.UsedBy != "" || invite.UsedAt != nil {
			return ErrInviteUsed
		}
		return tx.Delete(&invite).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInviteNotFound
	}
	if err != nil {
		return nil, err
	}
	return &invite, nil
}

func randomInviteCode(length int) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i, b := range buf {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf), nil
}
