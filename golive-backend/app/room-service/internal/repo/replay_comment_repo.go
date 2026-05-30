package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
)

var ErrReplayCommentNotFound = errors.New("replay comment not found")

type ReplayCommentRepo struct {
	db *gorm.DB
}

func NewReplayCommentRepo(db *gorm.DB) *ReplayCommentRepo { return &ReplayCommentRepo{db: db} }

func (r *ReplayCommentRepo) AutoMigrate() error {
	return r.db.AutoMigrate(
		&model.ReplayComment{},
		&model.ReplayCommentLike{},
	)
}

type ReplayCommentRow struct {
	ID          string
	RoomID      string
	UserID      string
	ParentID    string
	RootID      string
	Depth       int
	Content     string
	LikeCount   int64
	ReplyCount  int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Username    string
	DisplayName string
	AuthorName  string
	Avatar      string
	Verified    bool
}

func (r *ReplayCommentRepo) UserProfile(ctx context.Context, userID string) (PostUserProfile, error) {
	var row PostUserProfile
	err := r.db.WithContext(ctx).
		Table("users AS u").
		Select(`
u.id,
COALESCE(u.username, '') AS username,
COALESCE(u.display_name, '') AS display_name,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), u.id) AS name,
COALESCE(u.avatar, '') AS avatar,
u.verified
`).
		Where("u.id = ?", userID).
		Take(&row).Error
	return row, err
}

func (r *ReplayCommentRepo) Create(ctx context.Context, comment *model.ReplayComment) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(comment).Error; err != nil {
			return err
		}
		if comment.ParentID != "" {
			return tx.Model(&model.ReplayComment{}).
				Where("id = ?", comment.ParentID).
				Update("reply_count", gorm.Expr("reply_count + ?", 1)).Error
		}
		return nil
	})
}

func (r *ReplayCommentRepo) Get(ctx context.Context, roomID, commentID string) (*model.ReplayComment, error) {
	var comment model.ReplayComment
	err := r.db.WithContext(ctx).
		Where("room_id = ? AND id = ?", roomID, commentID).
		Take(&comment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrReplayCommentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

func (r *ReplayCommentRepo) List(ctx context.Context, roomID string, limit int) ([]ReplayCommentRow, int64, error) {
	if limit < 1 {
		limit = 200
	}
	if limit > 300 {
		limit = 300
	}
	q := r.db.WithContext(ctx).
		Table("replay_comments AS c").
		Where("c.room_id = ? AND c.deleted_at IS NULL", roomID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []ReplayCommentRow
	err := q.Select(`
c.id,
c.room_id,
c.user_id,
COALESCE(c.parent_id, '') AS parent_id,
COALESCE(c.root_id, '') AS root_id,
c.depth,
c.content,
c.like_count,
c.reply_count,
c.created_at,
c.updated_at,
COALESCE(u.username, '') AS username,
COALESCE(u.display_name, '') AS display_name,
COALESCE(NULLIF(u.display_name, ''), NULLIF(u.username, ''), c.user_id) AS author_name,
COALESCE(u.avatar, '') AS avatar,
COALESCE(u.verified, false) AS verified
`).
		Joins("LEFT JOIN users AS u ON u.id = c.user_id").
		Joins("LEFT JOIN replay_comments AS root ON root.id = COALESCE(NULLIF(c.root_id, ''), c.id)").
		Order("root.created_at DESC, c.depth ASC, c.created_at ASC").
		Limit(limit).
		Scan(&rows).Error
	return rows, total, err
}

func (r *ReplayCommentRepo) Count(ctx context.Context, roomID string) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).
		Model(&model.ReplayComment{}).
		Where("room_id = ? AND deleted_at IS NULL", roomID).
		Count(&total).Error
	return total, err
}

func (r *ReplayCommentRepo) DeleteTree(ctx context.Context, roomID, commentID string) (int64, error) {
	var deleted int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var comments []model.ReplayComment
		if err := tx.Where("room_id = ?", roomID).Find(&comments).Error; err != nil {
			return err
		}
		byID := make(map[string]model.ReplayComment, len(comments))
		children := make(map[string][]string, len(comments))
		for _, comment := range comments {
			byID[comment.ID] = comment
			if comment.ParentID != "" {
				children[comment.ParentID] = append(children[comment.ParentID], comment.ID)
			}
		}
		target, ok := byID[commentID]
		if !ok {
			return ErrReplayCommentNotFound
		}

		ids := make([]string, 0, 3)
		var walk func(string)
		walk = func(id string) {
			ids = append(ids, id)
			for _, childID := range children[id] {
				walk(childID)
			}
		}
		walk(commentID)
		if len(ids) == 0 {
			return nil
		}
		if err := tx.Where("comment_id IN ?", ids).Delete(&model.ReplayCommentLike{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id IN ?", ids).Delete(&model.ReplayComment{}).Error; err != nil {
			return err
		}
		if target.ParentID != "" {
			deletedSet := make(map[string]bool, len(ids))
			for _, id := range ids {
				deletedSet[id] = true
			}
			if !deletedSet[target.ParentID] {
				if err := tx.Model(&model.ReplayComment{}).
					Where("id = ?", target.ParentID).
					Update("reply_count", gorm.Expr("CASE WHEN reply_count > 0 THEN reply_count - 1 ELSE 0 END")).Error; err != nil {
					return err
				}
			}
		}
		deleted = int64(len(ids))
		return nil
	})
	return deleted, err
}

func (r *ReplayCommentRepo) Like(ctx context.Context, commentID, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.ReplayCommentLike{
			CommentID: commentID,
			UserID:    userID,
			CreatedAt: time.Now(),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected > 0 {
			if err := tx.Model(&model.ReplayComment{}).
				Where("id = ?", commentID).
				Update("like_count", gorm.Expr("like_count + ?", 1)).Error; err != nil {
				return err
			}
		}
		return selectReplayCommentLikeCount(tx, commentID, &count)
	})
	return count, err
}

func (r *ReplayCommentRepo) Unlike(ctx context.Context, commentID, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("comment_id = ? AND user_id = ?", commentID, userID).Delete(&model.ReplayCommentLike{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected > 0 {
			if err := tx.Model(&model.ReplayComment{}).
				Where("id = ?", commentID).
				Update("like_count", gorm.Expr("CASE WHEN like_count > 0 THEN like_count - 1 ELSE 0 END")).Error; err != nil {
				return err
			}
		}
		return selectReplayCommentLikeCount(tx, commentID, &count)
	})
	return count, err
}

func (r *ReplayCommentRepo) LikedIDs(ctx context.Context, userID string, commentIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(commentIDs))
	if userID == "" || len(commentIDs) == 0 {
		return out, nil
	}
	var likes []model.ReplayCommentLike
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND comment_id IN ?", userID, commentIDs).
		Find(&likes).Error; err != nil {
		return nil, err
	}
	for _, like := range likes {
		out[like.CommentID] = true
	}
	return out, nil
}

func selectReplayCommentLikeCount(tx *gorm.DB, commentID string, out *int64) error {
	var comment model.ReplayComment
	err := tx.Select("like_count").Where("id = ?", commentID).Take(&comment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrReplayCommentNotFound
	}
	if err != nil {
		return err
	}
	*out = comment.LikeCount
	return nil
}
