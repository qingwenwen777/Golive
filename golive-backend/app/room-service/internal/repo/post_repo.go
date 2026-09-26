package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qingwenwen777/golive/app/room-service/internal/model"
)

var (
	ErrPostNotFound    = errors.New("post not found")
	ErrCommentNotFound = errors.New("comment not found")
)

type PostRepo struct {
	db *gorm.DB
}

func NewPostRepo(db *gorm.DB) *PostRepo { return &PostRepo{db: db} }

func (r *PostRepo) AutoMigrate() error {
	if err := r.db.AutoMigrate(
		&model.ChannelPost{},
		&model.PostComment{},
		&model.PostLike{},
		&model.PostCommentLike{},
	); err != nil {
		return err
	}
	specs := append(sharedSearchFullTextIndexes(), fullTextIndexSpec{
		Table:   "channel_posts",
		Name:    "ft_channel_posts_search",
		Columns: []string{"content", "channel_id"},
	})
	return ensureMySQLFullTextIndexes(r.db, specs...)
}

type PostUserProfile struct {
	ID          string
	Username    string
	DisplayName string
	Name        string
	Avatar      string
	Verified    bool
}

type PostCommentRow struct {
	ID          string
	PostID      string
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

func (r *PostRepo) UserProfile(ctx context.Context, userID string) (PostUserProfile, error) {
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

func (r *PostRepo) CreatePost(ctx context.Context, post *model.ChannelPost) error {
	commentsEnabled := post.CommentsEnabled
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(post).Error; err != nil {
			return err
		}
		post.CommentsEnabled = commentsEnabled
		return tx.Model(&model.ChannelPost{}).
			Where("id = ?", post.ID).
			UpdateColumn("comments_enabled", commentsEnabled).Error
	})
}

func (r *PostRepo) GetPost(ctx context.Context, postID string) (*model.ChannelPost, error) {
	var post model.ChannelPost
	err := r.db.WithContext(ctx).Where("id = ?", postID).Take(&post).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPostNotFound
	}
	if err != nil {
		return nil, err
	}
	return &post, nil
}

func (r *PostRepo) ListByOwner(ctx context.Context, ownerID string, page, size int) ([]model.ChannelPost, int64, error) {
	page, size = normalizePostPage(page, size)
	tx := r.db.WithContext(ctx).Model(&model.ChannelPost{}).Where("owner_id = ?", ownerID)
	return listPosts(tx, page, size)
}

func (r *PostRepo) ListVisibleByOwner(ctx context.Context, ownerID string, visibilities []string, page, size int) ([]model.ChannelPost, int64, error) {
	page, size = normalizePostPage(page, size)
	tx := r.db.WithContext(ctx).Model(&model.ChannelPost{}).Where("owner_id = ?", ownerID)
	if len(visibilities) > 0 {
		tx = tx.Where("visibility IN ?", visibilities)
	}
	return listPosts(tx, page, size)
}

func (r *PostRepo) ListLatestVisibleByOwners(ctx context.Context, ownerIDs, visibilities []string, limit int) ([]model.ChannelPost, int64, error) {
	if len(ownerIDs) == 0 {
		return []model.ChannelPost{}, 0, nil
	}
	if len(visibilities) == 0 {
		visibilities = []string{model.PostVisibilityPublic}
	}
	if limit < 1 {
		limit = 8
	}
	if limit > 50 {
		limit = 50
	}
	var total int64
	if err := r.db.WithContext(ctx).
		Model(&model.ChannelPost{}).
		Where("owner_id IN ? AND visibility IN ?", ownerIDs, visibilities).
		Distinct("owner_id").
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var posts []model.ChannelPost
	err := r.db.WithContext(ctx).
		Raw(`
SELECT id, owner_id, channel_id, content, images_json, visibility, comments_enabled, comment_mode, like_count, comment_count, created_at, updated_at, deleted_at
FROM (
	SELECT channel_posts.*,
		ROW_NUMBER() OVER (PARTITION BY owner_id ORDER BY created_at DESC, id DESC) AS rn
	FROM channel_posts
	WHERE deleted_at IS NULL AND owner_id IN ? AND visibility IN ?
) AS ranked_posts
WHERE rn = 1
ORDER BY created_at DESC, id DESC
LIMIT ?`, ownerIDs, visibilities, limit).
		Scan(&posts).Error
	return posts, total, err
}

func (r *PostRepo) UpdatePostVisibility(ctx context.Context, ownerID, postID, visibility string) (*model.ChannelPost, error) {
	var post model.ChannelPost
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND owner_id = ?", postID, ownerID).Take(&post).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPostNotFound
			}
			return err
		}
		if err := tx.Model(&post).Update("visibility", visibility).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", postID).Take(&post).Error
	})
	if err != nil {
		return nil, err
	}
	return &post, nil
}

func listPosts(tx *gorm.DB, page, size int) ([]model.ChannelPost, int64, error) {
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var posts []model.ChannelPost
	err := tx.Order("created_at DESC").
		Scopes(pageWindow(page, size)).
		Limit(size).
		Find(&posts).Error
	return posts, total, err
}

func (r *PostRepo) DeletePost(ctx context.Context, ownerID, postID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var post model.ChannelPost
		err := tx.Where("id = ? AND owner_id = ?", postID, ownerID).Take(&post).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPostNotFound
		}
		if err != nil {
			return err
		}

		var commentIDs []string
		if err := tx.Model(&model.PostComment{}).Where("post_id = ?", postID).Pluck("id", &commentIDs).Error; err != nil {
			return err
		}
		if len(commentIDs) > 0 {
			if err := tx.Where("comment_id IN ?", commentIDs).Delete(&model.PostCommentLike{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", commentIDs).Delete(&model.PostComment{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("post_id = ?", postID).Delete(&model.PostLike{}).Error; err != nil {
			return err
		}
		return tx.Delete(&post).Error
	})
}

func (r *PostRepo) CreateComment(ctx context.Context, comment *model.PostComment) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(comment).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.ChannelPost{}).
			Where("id = ?", comment.PostID).
			Update("comment_count", gorm.Expr("comment_count + ?", 1)).Error; err != nil {
			return err
		}
		if comment.ParentID != "" {
			return tx.Model(&model.PostComment{}).
				Where("id = ?", comment.ParentID).
				Update("reply_count", gorm.Expr("reply_count + ?", 1)).Error
		}
		return nil
	})
}

func (r *PostRepo) GetComment(ctx context.Context, postID, commentID string) (*model.PostComment, error) {
	var comment model.PostComment
	err := r.db.WithContext(ctx).
		Where("post_id = ? AND id = ?", postID, commentID).
		Take(&comment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCommentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

func (r *PostRepo) ListComments(ctx context.Context, postID string, limit int) ([]PostCommentRow, int64, error) {
	if limit < 1 {
		limit = 200
	}
	if limit > 300 {
		limit = 300
	}
	q := r.db.WithContext(ctx).
		Table("post_comments AS c").
		Where("c.post_id = ? AND c.deleted_at IS NULL", postID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []PostCommentRow
	err := q.Select(`
c.id,
c.post_id,
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
		Joins("LEFT JOIN post_comments AS root ON root.id = COALESCE(NULLIF(c.root_id, ''), c.id)").
		Order("root.created_at DESC, c.depth ASC, c.created_at ASC").
		Limit(limit).
		Scan(&rows).Error
	return rows, total, err
}

func (r *PostRepo) DeleteCommentTree(ctx context.Context, postID, commentID string) (int64, error) {
	var deleted int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var comments []model.PostComment
		if err := tx.Where("post_id = ?", postID).Find(&comments).Error; err != nil {
			return err
		}
		byID := make(map[string]model.PostComment, len(comments))
		children := make(map[string][]string, len(comments))
		for _, comment := range comments {
			byID[comment.ID] = comment
			if comment.ParentID != "" {
				children[comment.ParentID] = append(children[comment.ParentID], comment.ID)
			}
		}
		target, ok := byID[commentID]
		if !ok {
			return ErrCommentNotFound
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
		if err := tx.Where("comment_id IN ?", ids).Delete(&model.PostCommentLike{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id IN ?", ids).Delete(&model.PostComment{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.ChannelPost{}).
			Where("id = ?", postID).
			Update("comment_count", gorm.Expr("CASE WHEN comment_count >= ? THEN comment_count - ? ELSE 0 END", len(ids), len(ids))).Error; err != nil {
			return err
		}
		if target.ParentID != "" {
			deletedSet := make(map[string]bool, len(ids))
			for _, id := range ids {
				deletedSet[id] = true
			}
			if !deletedSet[target.ParentID] {
				if err := tx.Model(&model.PostComment{}).
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

func (r *PostRepo) LikePost(ctx context.Context, postID, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.PostLike{
			PostID:    postID,
			UserID:    userID,
			CreatedAt: time.Now(),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected > 0 {
			if err := tx.Model(&model.ChannelPost{}).
				Where("id = ?", postID).
				Update("like_count", gorm.Expr("like_count + ?", 1)).Error; err != nil {
				return err
			}
		}
		return selectPostLikeCount(tx, postID, &count)
	})
	return count, err
}

func (r *PostRepo) UnlikePost(ctx context.Context, postID, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("post_id = ? AND user_id = ?", postID, userID).Delete(&model.PostLike{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected > 0 {
			if err := tx.Model(&model.ChannelPost{}).
				Where("id = ?", postID).
				Update("like_count", gorm.Expr("CASE WHEN like_count > 0 THEN like_count - 1 ELSE 0 END")).Error; err != nil {
				return err
			}
		}
		return selectPostLikeCount(tx, postID, &count)
	})
	return count, err
}

func (r *PostRepo) LikeComment(ctx context.Context, commentID, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.PostCommentLike{
			CommentID: commentID,
			UserID:    userID,
			CreatedAt: time.Now(),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected > 0 {
			if err := tx.Model(&model.PostComment{}).
				Where("id = ?", commentID).
				Update("like_count", gorm.Expr("like_count + ?", 1)).Error; err != nil {
				return err
			}
		}
		return selectCommentLikeCount(tx, commentID, &count)
	})
	return count, err
}

func (r *PostRepo) UnlikeComment(ctx context.Context, commentID, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("comment_id = ? AND user_id = ?", commentID, userID).Delete(&model.PostCommentLike{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected > 0 {
			if err := tx.Model(&model.PostComment{}).
				Where("id = ?", commentID).
				Update("like_count", gorm.Expr("CASE WHEN like_count > 0 THEN like_count - 1 ELSE 0 END")).Error; err != nil {
				return err
			}
		}
		return selectCommentLikeCount(tx, commentID, &count)
	})
	return count, err
}

func (r *PostRepo) PostLikedIDs(ctx context.Context, userID string, postIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(postIDs))
	if userID == "" || len(postIDs) == 0 {
		return out, nil
	}
	var likes []model.PostLike
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND post_id IN ?", userID, postIDs).
		Find(&likes).Error; err != nil {
		return nil, err
	}
	for _, like := range likes {
		out[like.PostID] = true
	}
	return out, nil
}

func (r *PostRepo) CommentLikedIDs(ctx context.Context, userID string, commentIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(commentIDs))
	if userID == "" || len(commentIDs) == 0 {
		return out, nil
	}
	var likes []model.PostCommentLike
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

func selectPostLikeCount(tx *gorm.DB, postID string, out *int64) error {
	var post model.ChannelPost
	err := tx.Select("like_count").Where("id = ?", postID).Take(&post).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrPostNotFound
	}
	if err != nil {
		return err
	}
	*out = post.LikeCount
	return nil
}

func selectCommentLikeCount(tx *gorm.DB, commentID string, out *int64) error {
	var comment model.PostComment
	err := tx.Select("like_count").Where("id = ?", commentID).Take(&comment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrCommentNotFound
	}
	if err != nil {
		return err
	}
	*out = comment.LikeCount
	return nil
}

func normalizePostPage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}
	if size > 50 {
		size = 50
	}
	return page, size
}
