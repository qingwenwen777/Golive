import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import {
  CheckCircle2,
  Globe2,
  Heart,
  LockKeyhole,
  MessageCircle,
  Reply,
  Send,
  Trash2,
  Users,
} from 'lucide-react';
import { toast } from 'sonner';
import {
  useCreatePostComment,
  useDeletePost,
  useDeletePostComment,
  usePostComments,
  useToggleCommentLike,
  useTogglePostLike,
  type ChannelPost,
  type PostComment,
} from '@/api/posts';
import { Avatar } from '@/components/Avatar';
import { LoadableImage } from '@/components/LoadableImage';
import { cn } from '@/lib/cn';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useIsAuthed } from '@/stores/useAuthStore';

export function PostCard({ post, context = 'channel' }: { post: ChannelPost; context?: 'channel' | 'studio' }) {
  const { t, i18n } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const [commentsOpen, setCommentsOpen] = useState(false);
  const [commentText, setCommentText] = useState('');
  const [replyTarget, setReplyTarget] = useState<PostComment | null>(null);
  const comments = usePostComments(post.id, commentsOpen);
  const createComment = useCreatePostComment(post.id);
  const deletePost = useDeletePost();
  const toggleLike = useTogglePostLike(post.id);
  const meta = visibilityMeta(post, t);
  const CommentMetaIcon = post.commentsEnabled && post.commentMode === 'followers' ? Users : MessageCircle;

  const submitComment = (event: FormEvent) => {
    event.preventDefault();
    const content = commentText.trim();
    if (!content) return;
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (!post.canComment) {
      toast.info(t('posts.comments.restricted', { defaultValue: '当前帖子限制评论。' }));
      return;
    }
    createComment.mutate(
      {
        content,
        parentId: replyTarget?.id,
      },
      {
        onSuccess: () => {
          setCommentText('');
          setReplyTarget(null);
          setCommentsOpen(true);
          toast.success(t('posts.comments.sent', { defaultValue: '评论已发布。' }));
        },
        onError: (err) => toast.error(err.message || t('posts.comments.failed', { defaultValue: '评论发布失败。' })),
      },
    );
  };

  const handleLike = () => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    toggleLike.mutate(post.liked ? 'unlike' : 'like');
  };

  const handleDelete = () => {
    if (!window.confirm(t('posts.confirmDelete', { defaultValue: '确定删除这条帖子吗？' }))) return;
    deletePost.mutate(post.id, {
      onSuccess: () => toast.success(t('posts.deleted', { defaultValue: '帖子已删除。' })),
      onError: (err) => toast.error(err.message || t('posts.deleteFailed', { defaultValue: '无法删除帖子。' })),
    });
  };

  const openReply = (comment: PostComment) => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (!post.canComment) return;
    setCommentsOpen(true);
    setReplyTarget(comment);
  };

  return (
    <article className={cn('gl-post-card', context === 'studio' && 'is-studio')}>
      <header className="gl-post-head">
        <Avatar name={post.author.name} src={post.author.avatar} size={42} />
        <div className="gl-post-author">
          <strong>
            {post.author.name}
            {post.author.verified && <CheckCircle2 size={15} />}
          </strong>
          <span>{formatPostDate(post.createdAt, i18n.language)}</span>
        </div>
        <div className="gl-post-badges">
          <span title={meta.title}>
            <meta.Icon size={14} />
            {meta.label}
          </span>
          <span>
            <CommentMetaIcon size={14} />
            {post.commentsEnabled
              ? post.commentMode === 'followers'
                ? t('posts.followerComments', { defaultValue: '粉丝评论' })
                : t('posts.openComments', { defaultValue: '可评论' })
              : t('posts.closedComments', { defaultValue: '评论关闭' })}
          </span>
        </div>
        {post.canDelete && (
          <button
            type="button"
            className="gl-post-icon-button is-danger"
            aria-label={t('posts.delete', { defaultValue: '删除帖子' })}
            disabled={deletePost.isPending}
            onClick={handleDelete}
          >
            <Trash2 size={16} />
          </button>
        )}
      </header>

      {post.content.trim() && <p className="gl-post-content">{post.content}</p>}

      {post.images.length > 0 && (
        <div className={cn('gl-post-images', post.images.length === 1 && 'is-single', post.images.length > 2 && 'is-collage')}>
          {post.images.map((image) => (
            <button type="button" className="gl-post-image" key={image} onClick={() => window.open(image, '_blank', 'noopener,noreferrer')}>
              <LoadableImage src={image} alt="" />
            </button>
          ))}
        </div>
      )}

      <footer className="gl-post-actions">
        <button type="button" className={cn(post.liked && 'is-active')} disabled={toggleLike.isPending} onClick={handleLike}>
          <Heart size={17} fill={post.liked ? 'currentColor' : 'none'} />
          {post.likeCount.toLocaleString()}
        </button>
        <button type="button" className={commentsOpen ? 'is-active' : undefined} onClick={() => setCommentsOpen((open) => !open)}>
          <MessageCircle size={17} />
          {post.commentCount.toLocaleString()}
        </button>
      </footer>

      {commentsOpen && (
        <section className="gl-post-comments">
          {comments.isPending ? (
            <div className="gl-post-comment-empty">{t('posts.comments.loading', { defaultValue: '评论加载中...' })}</div>
          ) : comments.data?.items.length ? (
            <div className="gl-post-comment-list">
              {comments.data.items.map((comment) => (
                <PostCommentRow
                  key={comment.id}
                  comment={comment}
                  postId={post.id}
                  canComment={post.canComment}
                  onReply={openReply}
                />
              ))}
            </div>
          ) : (
            <div className="gl-post-comment-empty">{t('posts.comments.empty', { defaultValue: '还没有评论。' })}</div>
          )}

          {post.commentsEnabled ? (
            post.canComment ? (
              <form className="gl-post-comment-form" onSubmit={submitComment}>
                {replyTarget && (
                  <div className="gl-post-reply-target">
                    <span>{t('posts.comments.replyingTo', { name: replyTarget.author.name, defaultValue: '回复 {{name}}' })}</span>
                    <button type="button" onClick={() => setReplyTarget(null)}>
                      {t('posts.comments.cancelReply', { defaultValue: '取消' })}
                    </button>
                  </div>
                )}
                <div className="gl-post-comment-input">
                  <textarea
                    rows={2}
                    maxLength={500}
                    value={commentText}
                    placeholder={t('posts.comments.placeholder', { defaultValue: '写一条评论...' })}
                    onChange={(event) => setCommentText(event.target.value)}
                  />
                  <button type="submit" disabled={createComment.isPending || !commentText.trim()} aria-label={t('posts.comments.send', { defaultValue: '发送评论' })}>
                    <Send size={16} />
                  </button>
                </div>
              </form>
            ) : (
              <div className="gl-post-comment-empty">
                {isAuthed
                  ? t('posts.comments.followersOnly', { defaultValue: '仅粉丝可以评论。' })
                  : t('posts.comments.signIn', { defaultValue: '登录后参与评论。' })}
              </div>
            )
          ) : (
            <div className="gl-post-comment-empty">{t('posts.comments.closed', { defaultValue: '评论区已关闭。' })}</div>
          )}
        </section>
      )}
    </article>
  );
}

function PostCommentRow({
  comment,
  postId,
  canComment,
  onReply,
}: {
  comment: PostComment;
  postId: string;
  canComment: boolean;
  onReply: (comment: PostComment) => void;
}) {
  const { t, i18n } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const toggleLike = useToggleCommentLike(postId, comment.id);
  const deleteComment = useDeletePostComment(postId);

  const handleLike = () => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    toggleLike.mutate(comment.liked ? 'unlike' : 'like');
  };

  const handleDelete = () => {
    if (!window.confirm(t('posts.comments.confirmDelete', { defaultValue: '确定删除这条评论吗？' }))) return;
    deleteComment.mutate(comment.id, {
      onSuccess: () => toast.success(t('posts.comments.deleted', { defaultValue: '评论已删除。' })),
      onError: (err) => toast.error(err.message || t('posts.comments.deleteFailed', { defaultValue: '无法删除评论。' })),
    });
  };

  return (
    <div className={cn('gl-post-comment', `is-depth-${Math.min(comment.depth, 2)}`)}>
      <Avatar name={comment.author.name} src={comment.author.avatar} size={30} />
      <div className="gl-post-comment-main">
        <div className="gl-post-comment-bubble">
          <div className="gl-post-comment-author">
            <strong>{comment.author.name}</strong>
            <span>{formatPostDate(comment.createdAt, i18n.language)}</span>
          </div>
          <p>{comment.content}</p>
        </div>
        <div className="gl-post-comment-actions">
          <button type="button" className={comment.liked ? 'is-active' : undefined} disabled={toggleLike.isPending} onClick={handleLike}>
            <Heart size={14} fill={comment.liked ? 'currentColor' : 'none'} />
            {comment.likeCount.toLocaleString()}
          </button>
          {canComment && comment.depth < 2 && (
            <button type="button" onClick={() => onReply(comment)}>
              <Reply size={14} />
              {t('posts.comments.reply', { defaultValue: '回复' })}
            </button>
          )}
          {comment.canDelete && (
            <button type="button" className="is-danger" disabled={deleteComment.isPending} onClick={handleDelete}>
              <Trash2 size={14} />
              {t('posts.comments.delete', { defaultValue: '删除' })}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

function visibilityMeta(post: ChannelPost, t: ReturnType<typeof useTranslation>['t']) {
  if (post.visibility === 'private') {
    return {
      Icon: LockKeyhole,
      label: t('posts.visibility.private', { defaultValue: '仅自己' }),
      title: t('posts.visibility.privateTitle', { defaultValue: '仅自己可见' }),
    };
  }
  if (post.visibility === 'followers') {
    return {
      Icon: Users,
      label: t('posts.visibility.followers', { defaultValue: '粉丝' }),
      title: t('posts.visibility.followersTitle', { defaultValue: '仅粉丝可见' }),
    };
  }
  return {
    Icon: Globe2,
    label: t('posts.visibility.public', { defaultValue: '公开' }),
    title: t('posts.visibility.publicTitle', { defaultValue: '所有人可见' }),
  };
}

function formatPostDate(value: string, locale: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return new Intl.DateTimeFormat(locale, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}
