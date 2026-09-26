package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qingwenwen777/golive/app/room-service/internal/model"
	"github.com/qingwenwen777/golive/app/room-service/internal/repo"
	"github.com/qingwenwen777/golive/pkg/contentpolicy"
	"github.com/qingwenwen777/golive/pkg/errcode"
)

type CreateBlockedWordReq struct {
	Word    string `json:"word"`
	Note    string `json:"note"`
	Enabled *bool  `json:"enabled"`
}

type BulkBlockedWordItem struct {
	Word string `json:"word"`
	Note string `json:"note"`
}

type BulkImportBlockedWordsReq struct {
	Items []BulkBlockedWordItem `json:"items"`
}

type BulkImportBlockedWordsResp struct {
	Created int `json:"created"`
	Skipped int `json:"skipped"`
}

type UpdateBlockedWordReq struct {
	Word    *string `json:"word"`
	Note    *string `json:"note"`
	Enabled *bool   `json:"enabled"`
}

type BlockedWordDTO struct {
	ID        string `json:"id"`
	Word      string `json:"word"`
	Note      string `json:"note,omitempty"`
	Enabled   bool   `json:"enabled"`
	CreatedBy string `json:"createdBy,omitempty"`
	UpdatedBy string `json:"updatedBy,omitempty"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type BlockedWordListResp struct {
	Items []BlockedWordDTO `json:"items"`
	Total int64            `json:"total"`
	Page  int              `json:"page"`
	Size  int              `json:"size"`
}

func (s *ModerationService) ListBlockedWords(ctx context.Context, adminID string, page, size int) (*BlockedWordListResp, error) {
	if _, err := s.requireContentModerator(ctx, adminID); err != nil {
		return nil, err
	}
	rows, total, err := s.moderation.ListBlockedWords(ctx, page, size)
	if err != nil {
		return nil, err
	}
	return &BlockedWordListResp{
		Items: blockedWordDTOs(rows),
		Total: total,
		Page:  normalizePage(page),
		Size:  normalizeSize(size),
	}, nil
}

func (s *ModerationService) CreateBlockedWord(ctx context.Context, adminID string, req CreateBlockedWordReq) (*BlockedWordDTO, error) {
	if _, err := s.requireContentModerator(ctx, adminID); err != nil {
		return nil, err
	}
	word := trimRunes(strings.TrimSpace(req.Word), 60)
	normalized := contentpolicy.NormalizeWord(word)
	if normalized == "" {
		return nil, errcode.New(http.StatusBadRequest, "blocked word is required").WithReason("word_required")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	now := s.now()
	row := &model.BlockedWord{
		ID:             uuid.NewString(),
		Word:           word,
		NormalizedWord: normalized,
		Note:           trimRunes(strings.TrimSpace(req.Note), 120),
		Enabled:        enabled,
		CreatedBy:      adminID,
		UpdatedBy:      adminID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.moderation.CreateBlockedWord(ctx, row); err != nil {
		if errors.Is(err, repo.ErrBlockedWordExists) {
			return nil, errcode.New(http.StatusConflict, "blocked word already exists").WithReason("word_exists")
		}
		return nil, err
	}
	_ = s.logAdminAudit(ctx, model.AdminAuditCategoryReview, "blocked_word_create", adminID, "blocked_word", row.ID, row.Word, "", "", row.Note, now)
	dto := blockedWordDTO(*row)
	return &dto, nil
}

func (s *ModerationService) UpdateBlockedWord(ctx context.Context, adminID, id string, req UpdateBlockedWordReq) (*BlockedWordDTO, error) {
	if _, err := s.requireContentModerator(ctx, adminID); err != nil {
		return nil, err
	}
	updates := map[string]any{
		"updated_by": adminID,
		"updated_at": s.now(),
	}
	if req.Word != nil {
		word := trimRunes(strings.TrimSpace(*req.Word), 60)
		if word == "" {
			return nil, errcode.New(http.StatusBadRequest, "blocked word is required").WithReason("word_required")
		}
		updates["word"] = word
		updates["normalized_word"] = contentpolicy.NormalizeWord(word)
	}
	if req.Note != nil {
		updates["note"] = trimRunes(strings.TrimSpace(*req.Note), 120)
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	row, err := s.moderation.UpdateBlockedWord(ctx, strings.TrimSpace(id), updates)
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrBlockedWordExists):
			return nil, errcode.New(http.StatusConflict, "blocked word already exists").WithReason("word_exists")
		case errors.Is(err, repo.ErrBlockedWordNotFound):
			return nil, errcode.New(http.StatusNotFound, "blocked word not found")
		default:
			return nil, err
		}
	}
	_ = s.logAdminAudit(ctx, model.AdminAuditCategoryReview, "blocked_word_update", adminID, "blocked_word", row.ID, row.Word, "", "", row.Note, s.now())
	dto := blockedWordDTO(*row)
	return &dto, nil
}

func (s *ModerationService) DeleteBlockedWord(ctx context.Context, adminID, id string) error {
	if _, err := s.requireContentModerator(ctx, adminID); err != nil {
		return err
	}
	if err := s.moderation.DeleteBlockedWord(ctx, strings.TrimSpace(id)); err != nil {
		if errors.Is(err, repo.ErrBlockedWordNotFound) {
			return errcode.New(http.StatusNotFound, "blocked word not found")
		}
		return err
	}
	_ = s.logAdminAudit(ctx, model.AdminAuditCategoryReview, "blocked_word_delete", adminID, "blocked_word", strings.TrimSpace(id), "", "", "", "", s.now())
	return nil
}

func (s *ModerationService) BulkImportBlockedWords(ctx context.Context, adminID string, req BulkImportBlockedWordsReq) (*BulkImportBlockedWordsResp, error) {
	if _, err := s.requireContentModerator(ctx, adminID); err != nil {
		return nil, err
	}
	if len(req.Items) == 0 {
		return nil, errcode.New(http.StatusBadRequest, "no blocked words").WithReason("empty_import")
	}
	if len(req.Items) > 500 {
		return nil, errcode.New(http.StatusBadRequest, "too many blocked words").WithReason("too_many_items")
	}
	now := s.now()
	created := 0
	skipped := 0
	for _, item := range req.Items {
		word := trimRunes(strings.TrimSpace(item.Word), 60)
		normalized := contentpolicy.NormalizeWord(word)
		if normalized == "" {
			skipped++
			continue
		}
		row := &model.BlockedWord{
			ID:             uuid.NewString(),
			Word:           word,
			NormalizedWord: normalized,
			Note:           trimRunes(strings.TrimSpace(item.Note), 120),
			Enabled:        true,
			CreatedBy:      adminID,
			UpdatedBy:      adminID,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := s.moderation.CreateBlockedWord(ctx, row); err != nil {
			if errors.Is(err, repo.ErrBlockedWordExists) {
				skipped++
				continue
			}
			return nil, err
		}
		created++
	}
	_ = s.logAdminAudit(ctx, model.AdminAuditCategoryReview, "blocked_word_import", adminID, "blocked_word", "", "blocked words", "", "", "", now)
	return &BulkImportBlockedWordsResp{Created: created, Skipped: skipped}, nil
}

func (s *ModerationService) EnsureTextAllowed(ctx context.Context, texts ...string) error {
	if s == nil || s.moderation == nil {
		return nil
	}
	hit, err := s.moderation.BlockedWordHit(ctx, texts...)
	if err != nil {
		return err
	}
	if hit != "" {
		return errcode.New(http.StatusBadRequest, "content contains blocked word").WithReason("blocked_word")
	}
	return nil
}

func blockedWordDTOs(rows []model.BlockedWord) []BlockedWordDTO {
	out := make([]BlockedWordDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, blockedWordDTO(row))
	}
	return out
}

func blockedWordDTO(row model.BlockedWord) BlockedWordDTO {
	return BlockedWordDTO{
		ID:        row.ID,
		Word:      row.Word,
		Note:      row.Note,
		Enabled:   row.Enabled,
		CreatedBy: row.CreatedBy,
		UpdatedBy: row.UpdatedBy,
		CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: row.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
