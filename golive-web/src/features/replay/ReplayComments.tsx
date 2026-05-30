import { useMemo, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { ChevronDown, ChevronUp, Flag, ListFilter, ThumbsDown, ThumbsUp, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import {
  useCreateReplayComment,
  useDeleteReplayComment,
  useReplayComments,
  useToggleReplayCommentLike,
  type ReplayComment,
} from '@/api/replayComments';
import { Avatar } from '@/components/Avatar';
import { VerifiedBadge } from '@/components/VerifiedBadge';
import { ReportDialog, type ReportTargetDraft } from '@/features/reporting/ReportDialog';
import { cn } from '@/lib/cn';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';

const MAX_COMMENT_LENGTH = 500;

type SortMode = 'top' | 'newest';

export function ReplayComments({
  roomId,
  ownerId,
  channelId,
}: {
  roomId: string;
  ownerId?: string;
  channelId?: string;
}) {
  const { t } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const currentUser = useAuthStore((s) => s.user);
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const comments = useReplayComments(roomId);
  const createComment = useCreateReplayComment(roomId);

  const [draft, setDraft] = useState('');
  const [focused, setFocused] = useState(false);
  const [replyTarget, setReplyTarget] = useState<ReplayComment | null>(null);
  const [sortMode, setSortMode] = useState<SortMode>('top');
  const [sortOpen, setSortOpen] = useState(false);
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});
  const [reportTarget, setReportTarget] = useState<ReportTargetDraft | null>(null);

  const items = comments.data?.items;
  const total = comments.data?.total ?? 0;
  const canComment = comments.data?.canComment ?? false;
  const commentTree = useMemo(() => buildCommentTree(items ?? [], sortMode), [items, sortMode]);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const content = draft.trim();
    if (!content) return;
    if (!isAuthed) {
      openLogin();
      return;
    }
    createComment.mutate(
      { content, parentId: replyTarget?.id },
      {
        onSuccess: () => {
          setDraft('');
          setReplyTarget(null);
          setFocused(false);
          toast.success(t('replayComments.sent', { defaultValue: '评论已发布。' }));
        },
        onError: (err) =>
          toast.error(err.message || t('replayComments.failed', { defaultValue: '评论发布失败。' })),
      },
    );
  };

  const cancel = () => {
    setDraft('');
    setReplyTarget(null);
    setFocused(false);
  };

  const startReply = (comment: ReplayComment) => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    setReplyTarget(comment);
    setDraft((current) => current);
    setFocused(true);
    requestAnimationFrame(() => {
      document.getElementById('gl-replay-comment-input')?.focus();
    });
  };

  const toggleReplies = (commentId: string) => {
    setCollapsed((current) => ({ ...current, [commentId]: !current[commentId] }));
  };

  const openReport = (target: ReportTargetDraft) => {
    if (!isAuthed) {
      openLogin(() => setReportTarget(target));
      return;
    }
    setReportTarget(target);
  };

  const reportComment = (comment: ReplayComment) => {
    openReport({
      targetType: 'post_comment',
      targetId: comment.id,
      targetUrl:
        typeof window !== 'undefined'
          ? `${window.location.href}#replay-comment-${comment.id}`
          : undefined,
      channelId,
      targetOwnerId: ownerId,
      targetUserId: comment.userId,
      targetUserName: comment.author.name,
      targetTitle: comment.author.name,
      targetText: comment.content,
    });
  };

  return (
    <section className="gl-replay-comments" aria-label={t('replayComments.title', { defaultValue: '评论' })}>
      <div className="gl-replay-comments-head">
        <h2>
          {t('replayComments.count', {
            count: total,
            formattedCount: total.toLocaleString(),
            defaultValue: '{{formattedCount}} 条评论',
          })}
        </h2>
        <div className="gl-replay-comments-sort">
          <button
            type="button"
            className="gl-replay-comments-sort-btn"
            aria-haspopup="menu"
            aria-expanded={sortOpen}
            onClick={() => setSortOpen((open) => !open)}
          >
            <ListFilter size={18} />
            {t('replayComments.sort', { defaultValue: '排序方式' })}
          </button>
          {sortOpen && (
            <>
              <div className="gl-replay-comments-sort-backdrop" onClick={() => setSortOpen(false)} />
              <div className="gl-replay-comments-sort-menu" role="menu">
                <button
                  type="button"
                  role="menuitemradio"
                  aria-checked={sortMode === 'top'}
                  className={sortMode === 'top' ? 'is-active' : undefined}
                  onClick={() => {
                    setSortMode('top');
                    setSortOpen(false);
                  }}
                >
                  {t('replayComments.sortTop', { defaultValue: '热门评论' })}
                </button>
                <button
                  type="button"
                  role="menuitemradio"
                  aria-checked={sortMode === 'newest'}
                  className={sortMode === 'newest' ? 'is-active' : undefined}
                  onClick={() => {
                    setSortMode('newest');
                    setSortOpen(false);
                  }}
                >
                  {t('replayComments.sortNewest', { defaultValue: '最新评论' })}
                </button>
              </div>
            </>
          )}
        </div>
      </div>

      <form className="gl-replay-comment-composer" onSubmit={submit}>
        <Avatar name={currentUser?.displayName || currentUser?.username || 'Guest'} src={currentUser?.avatar} size={40} />
        <div className="gl-replay-comment-composer-main">
          {replyTarget && (
            <div className="gl-replay-comment-reply-chip">
              <span>
                {t('replayComments.replyingTo', {
                  name: replyTarget.author.name,
                  defaultValue: '回复 {{name}}',
                })}
              </span>
              <button type="button" onClick={() => setReplyTarget(null)}>
                {t('replayComments.cancel', { defaultValue: '取消' })}
              </button>
            </div>
          )}
          <input
            id="gl-replay-comment-input"
            type="text"
            className="gl-replay-comment-input"
            maxLength={MAX_COMMENT_LENGTH}
            value={draft}
            autoComplete="off"
            autoCorrect="off"
            spellCheck={false}
            placeholder={t('replayComments.placeholder', { defaultValue: '添加评论...' })}
            onFocus={() => {
              if (!isAuthed) {
                openLogin();
                return;
              }
              setFocused(true);
            }}
            onChange={(event) => setDraft(event.target.value)}
          />
          {(focused || draft) && (
            <div className="gl-replay-comment-composer-actions">
              <button type="button" className="gl-replay-comment-btn-ghost" onClick={cancel}>
                {t('replayComments.cancel', { defaultValue: '取消' })}
              </button>
              <button
                type="submit"
                className="gl-replay-comment-btn-primary"
                disabled={createComment.isPending || !draft.trim()}
              >
                {t('replayComments.submit', { defaultValue: '评论' })}
              </button>
            </div>
          )}
        </div>
      </form>

      {comments.isPending ? (
        <div className="gl-replay-comment-status">
          {t('replayComments.loading', { defaultValue: '评论加载中...' })}
        </div>
      ) : commentTree.length ? (
        <div className="gl-replay-comment-list">
          {commentTree.map((comment) => (
            <ReplayCommentRow
              key={comment.id}
              comment={comment}
              roomId={roomId}
              ownerId={ownerId}
              canComment={canComment}
              collapsed={Boolean(collapsed[comment.id])}
              onToggleReplies={toggleReplies}
              onReply={startReply}
              onReport={reportComment}
            />
          ))}
        </div>
      ) : (
        <div className="gl-replay-comment-status">
          {t('replayComments.empty', { defaultValue: '还没有评论，快来抢沙发。' })}
        </div>
      )}

      <ReportDialog
        open={Boolean(reportTarget)}
        target={reportTarget}
        onOpenChange={(open) => {
          if (!open) setReportTarget(null);
        }}
      />
    </section>
  );
}

function ReplayCommentRow({
  comment,
  roomId,
  ownerId,
  canComment,
  collapsed,
  onToggleReplies,
  onReply,
  onReport,
}: {
  comment: ReplayCommentNode;
  roomId: string;
  ownerId?: string;
  canComment: boolean;
  collapsed: boolean;
  onToggleReplies: (commentId: string) => void;
  onReply: (comment: ReplayComment) => void;
  onReport: (comment: ReplayComment) => void;
}) {
  const { t, i18n } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const toggleLike = useToggleReplayCommentLike(roomId, comment.id);
  const deleteComment = useDeleteReplayComment(roomId);
  const [menuOpen, setMenuOpen] = useState(false);
  const nestedCount = countNested(comment.children);
  const isOwnerComment = Boolean(ownerId) && comment.userId === ownerId;

  const handleLike = () => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    toggleLike.mutate(comment.liked ? 'unlike' : 'like');
  };

  const handleDelete = () => {
    if (!window.confirm(t('replayComments.confirmDelete', { defaultValue: '确定删除这条评论吗？' })))
      return;
    deleteComment.mutate(comment.id, {
      onSuccess: () => toast.success(t('replayComments.deleted', { defaultValue: '评论已删除。' })),
      onError: (err) =>
        toast.error(
          err.message || t('replayComments.deleteFailed', { defaultValue: '无法删除评论。' }),
        ),
    });
  };

  return (
    <div className="gl-replay-comment" id={`replay-comment-${comment.id}`}>
      <Avatar name={comment.author.name} src={comment.author.avatar} size={comment.depth > 0 ? 30 : 40} />
      <div className="gl-replay-comment-body">
        <div className="gl-replay-comment-meta">
          <span className={cn('gl-replay-comment-author', isOwnerComment && 'is-owner')}>
            @{comment.author.username || comment.author.name}
          </span>
          {comment.author.verified && <VerifiedBadge size={13} />}
          <span className="gl-replay-comment-time">
            {formatRelativeTime(comment.createdAt, i18n.language, t)}
          </span>
        </div>
        <p className="gl-replay-comment-text">{comment.content}</p>
        <div className="gl-replay-comment-actions">
          <button
            type="button"
            className={cn('gl-replay-comment-like', comment.liked && 'is-active')}
            disabled={toggleLike.isPending}
            onClick={handleLike}
            aria-label={t('replayComments.like', { defaultValue: '赞' })}
          >
            <ThumbsUp size={16} fill={comment.liked ? 'currentColor' : 'none'} />
            {comment.likeCount > 0 && <span>{comment.likeCount.toLocaleString()}</span>}
          </button>
          <button
            type="button"
            className="gl-replay-comment-dislike"
            aria-label={t('replayComments.dislike', { defaultValue: '踩' })}
            disabled
          >
            <ThumbsDown size={16} />
          </button>
          {canComment && comment.depth < 2 && (
            <button type="button" className="gl-replay-comment-reply" onClick={() => onReply(comment)}>
              {t('replayComments.reply', { defaultValue: '回复' })}
            </button>
          )}
          <span className="gl-replay-comment-more-wrap">
            <button
              type="button"
              className="gl-replay-comment-more"
              aria-label={t('report.moreActions')}
              onClick={() => setMenuOpen((open) => !open)}
            >
              <span />
              <span />
              <span />
            </button>
            {menuOpen && (
              <>
                <div className="gl-replay-comment-menu-backdrop" onClick={() => setMenuOpen(false)} />
                <div className="gl-replay-comment-menu" role="menu">
                  {comment.canDelete && (
                    <button
                      type="button"
                      className="is-danger"
                      disabled={deleteComment.isPending}
                      onClick={() => {
                        setMenuOpen(false);
                        handleDelete();
                      }}
                    >
                      <Trash2 size={14} />
                      {t('replayComments.delete', { defaultValue: '删除' })}
                    </button>
                  )}
                  <button
                    type="button"
                    onClick={() => {
                      setMenuOpen(false);
                      onReport(comment);
                    }}
                  >
                    <Flag size={14} />
                    {t('report.action')}
                  </button>
                </div>
              </>
            )}
          </span>
        </div>

        {comment.children.length > 0 && (
          <button
            type="button"
            className="gl-replay-comment-toggle"
            onClick={() => onToggleReplies(comment.id)}
          >
            {collapsed ? <ChevronDown size={18} /> : <ChevronUp size={18} />}
            {t('replayComments.replyCount', {
              count: nestedCount,
              formattedCount: nestedCount.toLocaleString(),
              defaultValue: '{{formattedCount}} 条回复',
            })}
          </button>
        )}

        {comment.children.length > 0 && !collapsed && (
          <div className="gl-replay-comment-children">
            {comment.children.map((child) => (
              <ReplayCommentRow
                key={child.id}
                comment={child}
                roomId={roomId}
                ownerId={ownerId}
                canComment={canComment}
                collapsed={false}
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

interface ReplayCommentNode extends ReplayComment {
  children: ReplayCommentNode[];
}

function buildCommentTree(items: ReplayComment[], sortMode: SortMode): ReplayCommentNode[] {
  const nodes = new Map<string, ReplayCommentNode>();
  for (const item of items) {
    nodes.set(item.id, { ...item, children: [] });
  }
  const roots: ReplayCommentNode[] = [];
  for (const item of items) {
    const node = nodes.get(item.id);
    if (!node) continue;
    const parent = item.parentId ? nodes.get(item.parentId) : undefined;
    if (parent) parent.children.push(node);
    else roots.push(node);
  }
  const rootSorter =
    sortMode === 'top'
      ? (a: ReplayCommentNode, b: ReplayCommentNode) =>
          b.likeCount - a.likeCount || compareDateDesc(a.createdAt, b.createdAt)
      : (a: ReplayCommentNode, b: ReplayCommentNode) => compareDateDesc(a.createdAt, b.createdAt);
  roots.sort(rootSorter);
  const sortChildren = (node: ReplayCommentNode) => {
    node.children.sort((a, b) => compareDateAsc(a.createdAt, b.createdAt));
    node.children.forEach(sortChildren);
  };
  roots.forEach(sortChildren);
  return roots;
}

function countNested(items: ReplayCommentNode[]): number {
  return items.reduce((sum, item) => sum + 1 + countNested(item.children), 0);
}

function compareDateAsc(a: string, b: string): number {
  return new Date(a).getTime() - new Date(b).getTime();
}

function compareDateDesc(a: string, b: string): number {
  return new Date(b).getTime() - new Date(a).getTime();
}

const RELATIVE_UNITS: Array<{ unit: Intl.RelativeTimeFormatUnit; seconds: number }> = [
  { unit: 'year', seconds: 31536000 },
  { unit: 'month', seconds: 2592000 },
  { unit: 'week', seconds: 604800 },
  { unit: 'day', seconds: 86400 },
  { unit: 'hour', seconds: 3600 },
  { unit: 'minute', seconds: 60 },
];

function formatRelativeTime(
  value: string,
  locale: string,
  t: ReturnType<typeof useTranslation>['t'],
): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  const diffSeconds = Math.round((date.getTime() - Date.now()) / 1000);
  const abs = Math.abs(diffSeconds);
  if (abs < 60) return t('replayComments.justNow', { defaultValue: '刚刚' });
  try {
    const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
    for (const { unit, seconds } of RELATIVE_UNITS) {
      if (abs >= seconds) {
        return rtf.format(Math.round(diffSeconds / seconds), unit);
      }
    }
  } catch {
    /* fall through to absolute date */
  }
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(date);
}
