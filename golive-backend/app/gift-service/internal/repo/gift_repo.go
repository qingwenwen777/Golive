package repo

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qingwenwen777/golive/app/gift-service/internal/model"
)

var ErrGiftNotFound = errors.New("gift not found")

type GiftRepo struct{ db *gorm.DB }

func NewGiftRepo(db *gorm.DB) *GiftRepo { return &GiftRepo{db: db} }

func (r *GiftRepo) AutoMigrate() error { return r.db.AutoMigrate(&model.Gift{}) }

func (r *GiftRepo) List(ctx context.Context) ([]model.Gift, error) {
	var out []model.Gift
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Order("price_coin ASC").Find(&out).Error
	return out, err
}

func (r *GiftRepo) Get(ctx context.Context, id string) (*model.Gift, error) {
	var g model.Gift
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrGiftNotFound
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// Upsert is used by the seeder.
func (r *GiftRepo) Upsert(ctx context.Context, g *model.Gift) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name",
			"name_ja",
			"icon",
			"category",
			"animation",
			"tier",
			"unlock_level",
		}),
	}).Create(g).Error
}
