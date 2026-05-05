import { useMemo, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import {
  ChevronDown,
  ChevronRight,
  CheckCircle2,
  Flag,
  Globe2,
  Heart,
  LockKeyhole,
  MessageCircle,
  MoreVertical,
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
  useUpdatePostVisibility,
  type ChannelPost,
  type PostComment,
  type PostVisibility,
} from '@/api/posts';
import { Avatar } from '@/components/Avatar';
import { LoadableImage } from '@/components/LoadableImage';
import { ReportDialog, type ReportTargetDraft } from '@/features/reporting/ReportDialog';
import { cn } from '@/lib/cn';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useIsAuthed } from '@/stores/useAuthStore';

export function PostCard({
  post,
  context = 'channel',
}: {
  post: ChannelPost;
  context?: 'channel' | 'studio';
}) {
  const { t, i18n } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const [commentsOpen, setCommentsOpen] = useState(false);
  const [commentText, setCommentText] = useState('');
  const [replyTarget, setReplyTarget] = useState<PostComment | null>(null);
  const [postMenuOpen, setPostMenuOpen] = useState(false);
  const [reportTarget, setReportTarget] = useState<ReportTargetDraft | null>(null);
  const [collapsedReplies, setCollapsedReplies] = useState<Record<string, boolean>>({});
  const comments = usePostComments(post.id, commentsOpen);
  const createComment = useCreatePostComment(post.id);
  const deletePost = useDeletePost();
  const toggleLike = useTogglePostLike(post.id);
  const updateVisibility = useUpdatePostVisibility();
  const meta = visibilityMeta(post, t);
  const CommentMetaIcon =
    post.commentsEnabled && post.commentMode === 'followers' ? Users : MessageCircle;
  const hasPostBody = Boolean(post.content.trim() || post.images.length > 0);
  const commentTree = useMemo(
    () => buildCommentTree(comments.data?.items ?? []),
    [comments.data?.items],
  );

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
        onError: (err) =>
          toast.error(
            err.message || t('posts.comments.failed', { defaultValue: '评论发布失败。' }),
          ),
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
      onError: (err) =>
        toast.error(err.message || t('posts.deleteFailed', { defaultValue: '无法删除帖子。' })),
    });
  };

  const changeVisibility = (visibility: PostVisibility) => {
    if (visibility === post.visibility || updateVisibility.isPending) return;
    updateVisibility.mutate(
      { postId: post.id, visibility },
      {
        onSuccess: () =>
          toast.success(t('posts.visibility.updated', { defaultValue: '可见范围已更新。' })),
        onError: (err) =>
          toast.error(
            err.message ||
              t('posts.visibility.updateFailed', { defaultValue: '可见范围更新失败。' }),
          ),
      },
    );
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

  const toggleReplies = (commentId: string) => {
    setCollapsedReplies((current) => ({ ...current, [commentId]: !current[commentId] }));
  };

  const openReport = (target: ReportTargetDraft) => {
    if (!isAuthed) {
      openLogin(() => setReportTarget(target));
      return;
    }
    setReportTarget(target);
  };

  const reportPost = () => {
    setPostMenuOpen(false);
    openReport({
      targetType: 'post',
      targetId: post.id,
      targetUrl:
        typeof window !== 'undefined' ? `${window.location.href}#post-${post.id}` : undefined,
      channelId: post.channelId,
      targetOwnerId: post.ownerId,
      targetOwnerName: post.author.name,
      targetUserId: post.ownerId,
      targetUserName: post.author.name,
      targetTitle: post.author.name,
      targetText: post.content,
    });
  };

  const reportComment = (comment: PostComment) => {
    openReport({
      targetType: 'post_comment',
      targetId: comment.id,
      targetUrl:
        typeof window !== 'undefined' ? `${window.location.href}#comment-${comment.id}` : undefined,
      channelId: post.channelId,
      targetOwnerId: post.ownerId,
      targetOwnerName: post.author.name,
      targetUserId: comment.userId,
      targetUserName: comment.author.name,
      targetTitle: post.author.name,
      targetText: comment.content,
    });
  };

  return (
    <article
      className={cn('gl-post-card', context === 'studio' && 'is-studio')}
      id={`post-${post.id}`}
    >
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
        <div className="gl-post-menu-wrap">
          <button
            type="button"
            className="gl-post-icon-button"
            aria-label={t('report.moreActions')}
            onClick={() => setPostMenuOpen((open) => !open)}
          >
            <MoreVertical size={17} />
          </button>
          {postMenuOpen && (
            <div className="gl-post-menu">
              <button type="button" onClick={reportPost}>
                <Flag size={14} />
                {t('report.action')}
              </button>
            </div>
          )}
        </div>
      </header>

      {post.canDelete && (
        <div className="gl-post-owner-tools">
          <span>{t('posts.visibility.editLabel', { defaultValue: '可见范围' })}</span>
          <div
            className="gl-post-visibility-edit"
            aria-label={t('posts.editor.visibility', { defaultValue: '可见范围' })}
          >
            {postVisibilityOptions(t).map((option) => (
              <button
                key={option.value}
                type="button"
                className={post.visibility === option.value ? 'is-active' : undefined}
                disabled={updateVisibility.isPending}
                onClick={() => changeVisibility(option.value)}
              >
                <option.Icon size={14} />
                {option.label}
              </button>
            ))}
          </div>
        </div>
      )}

      {hasPostBody && (
        <div className="gl-post-body">
          {post.content.trim() && <p className="gl-post-content">{post.content}</p>}

          {post.images.length > 0 && (
            <div
              className={cn(
                'gl-post-images',
                post.images.length === 1 && 'is-single',
                post.images.length > 2 && 'is-collage',
              )}
            >
              {post.images.map((image) => (
                <button
                  type="button"
                  className="gl-post-image"
                  key={image}
                  onClick={() => window.open(image, '_blank', 'noopener,noreferrer')}
                >
                  <LoadableImage src={image} alt="" />
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      <footer className="gl-post-actions">
        <button
          type="button"
          className={cn(post.liked && 'is-active')}
          disabled={toggleLike.isPending}
          onClick={handleLike}
        >
          <Heart size={17} fill={post.liked ? 'currentColor' : 'none'} />
          {post.likeCount.toLocaleString()}
        </button>
        <button
          type="button"
          className={commentsOpen ? 'is-active' : undefined}
          onClick={() => setCommentsOpen((open) => !open)}
        >
          <MessageCircle size={17} />
          {post.commentCount.toLocaleString()}
        </button>
      </footer>

      {commentsOpen && (
        <section className="gl-post-comments">
          {comments.isPending ? (
            <div className="gl-post-comment-empty">
              {t('posts.comments.loading', { defaultValue: '评论加载中...' })}
            </div>
          ) : commentTree.length ? (
            <div className="gl-post-comment-list">
              {commentTree.map((comment) => (
                <PostCommentRow
                  key={comment.id}
                  comment={comment}
                  postId={post.id}
                  canComment={post.canComment}
                  isCollapsed={(commentId) => Boolean(collapsedReplies[commentId])}
                  onToggleReplies={toggleReplies}
                  onReply={openReply}
                  onReport={reportComment}
                />
              ))}
            </div>
          ) : (
            <div className="gl-post-comment-empty">
              {t('posts.comments.empty', { defaultValue: '还没有评论。' })}
            </div>
          )}

          {post.commentsEnabled ? (
            post.canComment ? (
              <form className="gl-post-comment-form" onSubmit={submitComment}>
                {replyTarget && (
                  <div className="gl-post-reply-target">
                    <span>
                      {t('posts.comments.replyingTo', {
                        name: replyTarget.author.name,
                        defaultValue: '回复 {{name}}',
                      })}
                    </span>
                    <button type="button" onClick={() => setReplyTarget(null)}>
                      {t('posts.comments.cancelReply', { defaultValue: '取消' })}
                    </button>
                  </div>
                )}
                <div className="gl-post-comment-input">
                  <textarea
                    rows={1}
                    maxLength={500}
                    value={commentText}
                    placeholder={t('posts.comments.placeholder', { defaultValue: '写一条评论...' })}
                    onChange={(event) => setCommentText(event.target.value)}
                  />
                  <button
                    type="submit"
                    disabled={createComment.isPending || !commentText.trim()}
                    aria-label={t('posts.comments.send', { defaultValue: '发送评论' })}
                  >
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
            <div className="gl-post-comment-empty">
              {t('posts.comments.closed', { defaultValue: '评论区已关闭。' })}
            </div>
          )}
        </section>
      )}
      <ReportDialog
        open={Boolean(reportTarget)}
        target={reportTarget}
        onOpenChange={(open) => {
          if (!open) setReportTarget(null);
        }}
      />
    </article>
  );
}

function PostCommentRow({
  comment,
  postId,
  canComment,
  isCollapsed,
  onToggleReplies,
  onReply,
  onReport,
}: {
  comment: PostCommentNode;
  postId: string;
  canComment: boolean;
  isCollapsed: (commentId: string) => boolean;
  onToggleReplies: (commentId: string) => void;
  onReply: (comment: PostComment) => void;
  onReport: (comment: PostComment) => void;
}) {
  const { t, i18n } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const toggleLike = useToggleCommentLike(postId, comment.id);
  const deleteComment = useDeletePostComment(postId);
  const nestedCount = countNestedComments(comment.children);
  const collapsed = isCollapsed(comment.id);
  const [menuOpen, setMenuOpen] = useState(false);

  const handleLike = () => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    toggleLike.mutate(comment.liked ? 'unlike' : 'like');
  };

  const handleDelete = () => {
    if (
      !window.confirm(t('posts.comments.confirmDelete', { defaultValue: '确定删除这条评论吗？' }))
    )
      return;
    deleteComment.mutate(comment.id, {
      onSuccess: () => toast.success(t('posts.comments.deleted', { defaultValue: '评论已删除。' })),
      onError: (err) =>
        toast.error(
          err.message || t('posts.comments.deleteFailed', { defaultValue: '无法删除评论。' }),
        ),
    });
  };

  return (
    <div
      className={cn('gl-post-comment', `is-depth-${Math.min(comment.depth, 2)}`)}
      id={`comment-${comment.id}`}
    >
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
          <button
            type="button"
            className={comment.liked ? 'is-active' : undefined}
            disabled={toggleLike.isPending}
            onClick={handleLike}
          >
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
            <button
              type="button"
              className="is-danger"
              disabled={deleteComment.isPending}
              onClick={handleDelete}
            >
              <Trash2 size={14} />
              {t('posts.comments.delete', { defaultValue: '删除' })}
            </button>
          )}
          <span className="gl-post-comment-more">
            <button
              type="button"
              aria-label={t('report.moreActions')}
              onClick={() => setMenuOpen((open) => !open)}
            >
              <MoreVertical size={14} />
            </button>
            {menuOpen && (
              <span className="gl-post-comment-menu">
                <button
                  type="button"
                  onClick={() => {
                    setMenuOpen(false);
                    onReport(comment);
                  }}
                >
                  <Flag size={13} />
                  {t('report.action')}
                </button>
              </span>
            )}
          </span>
          {comment.children.length > 0 && (
            <button
              type="button"
              className="gl-post-comment-collapse"
              onClick={() => onToggleReplies(comment.id)}
            >
              {collapsed ? <ChevronRight size={14} /> : <ChevronDown size={14} />}
              {collapsed
                ? t('posts.comments.expandReplies', {
                    count: nestedCount,
                    defaultValue: '展开 {{count}} 条回复',
                  })
                : t('posts.comments.collapseReplies', {
                    count: nestedCount,
                    defaultValue: '收起 {{count}} 条回复',
                  })}
            </button>
          )}
        </div>
        {comment.children.length > 0 && !collapsed && (
          <div className="gl-post-comment-children">
            {comment.children.map((child) => (
              <PostCommentRow
                key={child.id}
                comment={child}
                postId={postId}
                canComment={canComment}
                isCollapsed={isCollapsed}
                onToggleReplies={onToggleReplies}
                onReply={onReply}
                onReport={onReport}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

interface PostCommentNode extends PostComment {
  children: PostCommentNode[];
}

function buildCommentTree(items: PostComment[]): PostCommentNode[] {
  const nodes = new Map<string, PostCommentNode>();
  for (const item of items) {
    nodes.set(item.id, { ...item, children: [] });
  }
  const roots: PostCommentNode[] = [];
  for (const item of items) {
    const node = nodes.get(item.id);
    if (!node) continue;
    const parent = item.parentId ? nodes.get(item.parentId) : undefined;
    if (parent) parent.children.push(node);
    else roots.push(node);
  }
  roots.sort((a, b) => compareDateDesc(a.createdAt, b.createdAt));
  const sortChildren = (node: PostCommentNode) => {
    node.children.sort((a, b) => compareDateAsc(a.createdAt, b.createdAt));
    node.children.forEach(sortChildren);
  };
  roots.forEach(sortChildren);
  return roots;
}

function countNestedComments(items: PostCommentNode[]): number {
  return items.reduce((sum, item) => sum + 1 + countNestedComments(item.children), 0);
}

function compareDateAsc(a: string, b: string): number {
  return new Date(a).getTime() - new Date(b).getTime();
}

function compareDateDesc(a: string, b: string): number {
  return new Date(b).getTime() - new Date(a).getTime();
}

function postVisibilityOptions(t: ReturnType<typeof useTranslation>['t']): Array<{
  value: PostVisibility;
  label: string;
  Icon: typeof Globe2;
}> {
  return [
    {
      value: 'public',
      label: t('posts.visibility.public', { defaultValue: '公开' }),
      Icon: Globe2,
    },
    {
      value: 'followers',
      label: t('posts.visibility.followers', { defaultValue: '粉丝' }),
      Icon: Users,
    },
    {
      value: 'private',
      label: t('posts.visibility.private', { defaultValue: '仅自己' }),
      Icon: LockKeyhole,
    },
  ];
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
