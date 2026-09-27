package repo

import (
	"context"
	"strings"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/pkg/contentpolicy"
	"gorm.io/gorm"
)

func (r *ModerationRepo) ListBlockedWords(ctx context.Context, page, size int) ([]model.BlockedWord, int64, error) {
	page, size = normalizeModerationPage(page, size)
	q := r.db.WithContext(ctx).Model(&model.BlockedWord{})
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.BlockedWord
	err := q.Order("enabled DESC, updated_at DESC, created_at DESC").
		Scopes(pageWindow(page, size)).
		Limit(size).
		Find(&rows).Error
	return rows, total, err
}

func (r *ModerationRepo) ActiveBlockedWords(ctx context.Context) ([]model.BlockedWord, error) {
	var rows []model.BlockedWord
	err := r.db.WithContext(ctx).Model(&model.BlockedWord{}).
		Where("enabled = ?", true).
		Order("updated_at DESC, created_at DESC").
		Find(&rows).Error
	return rows, err
}

func (r *ModerationRepo) CreateBlockedWord(ctx context.Context, word *model.BlockedWord) error {
	if err := r.db.WithContext(ctx).Create(word).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ErrBlockedWordExists
		}
		return err
	}
	return r.SyncBlockedWords(ctx)
}

func (r *ModerationRepo) UpdateBlockedWord(ctx context.Context, id string, updates map[string]any) (*model.BlockedWord, error) {
	var row model.BlockedWord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.BlockedWord{}).Where("id = ?", id).Updates(updates)
		if res.Error != nil {
			if strings.Contains(strings.ToLower(res.Error.Error()), "duplicate") || strings.Contains(strings.ToLower(res.Error.Error()), "unique") {
				return ErrBlockedWordExists
			}
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrBlockedWordNotFound
		}
		return tx.Where("id = ?", id).Take(&row).Error
	})
	if err != nil {
		return nil, err
	}
	return &row, r.SyncBlockedWords(ctx)
}

func (r *ModerationRepo) DeleteBlockedWord(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Delete(&model.BlockedWord{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrBlockedWordNotFound
	}
	return r.SyncBlockedWords(ctx)
}

func (r *ModerationRepo) BlockedWordHit(ctx context.Context, texts ...string) (string, error) {
	if len(texts) == 0 {
		return "", nil
	}
	rows, err := r.ActiveBlockedWords(ctx)
	if err != nil {
		return "", err
	}
	words := make([]string, 0, len(rows))
	for _, row := range rows {
		words = append(words, row.NormalizedWord)
	}
	for _, text := range texts {
		if hit := contentpolicy.Hit(text, words); hit != "" {
			return hit, nil
		}
	}
	return "", nil
}

func (r *ModerationRepo) SyncBlockedWords(ctx context.Context) error {
	if r.rdb == nil {
		return nil
	}
	rows, err := r.ActiveBlockedWords(ctx)
	if err != nil {
		return err
	}
	pipe := r.rdb.TxPipeline()
	pipe.Del(ctx, contentpolicy.RedisBlockedWordsKey)
	if len(rows) > 0 {
		members := make([]any, 0, len(rows))
		for _, row := range rows {
			if word := contentpolicy.NormalizeWord(row.NormalizedWord); word != "" {
				members = append(members, word)
			}
		}
		if len(members) > 0 {
			pipe.SAdd(ctx, contentpolicy.RedisBlockedWordsKey, members...)
		}
	}
	_, err = pipe.Exec(ctx)
	return err
}
