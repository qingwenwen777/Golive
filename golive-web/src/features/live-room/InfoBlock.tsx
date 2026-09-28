import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { toast } from 'sonner';
import { ThumbsUp, ThumbsDown, Share2, Bell, Gift, MoreHorizontal, Flag } from 'lucide-react';
import { Avatar } from '@/components/Avatar';
import { Icons } from '@/components/Icons';
import { ShareDialog } from '@/components/ShareDialog';
import { VerifiedBadge } from '@/components/VerifiedBadge';
import { useLangStore } from '@/stores/useLangStore';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useFollowState, useFollow, useUnfollow, useLikeState, useLike } from '@/api/room';
import {
  useLibraryMembership,
  useRemoveUserLibraryItem,
  useSaveUserLibraryItem,
} from '@/api/library';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ReportDialog, type ReportTargetDraft } from '@/features/reporting/ReportDialog';
import { cn } from '@/lib/cn';
import {
  LIKED_STREAMS_KEY,
  WATCH_LATER_KEY,
  removeFromLibrary,
  saveToLibrary,
} from '@/lib/liveLibrary';
import { streamChannelName, type Stream } from '@/types/stream';
import { formatCount } from '@/lib/format';

export interface InfoBlockProps {
  stream: Stream;
  viewerCount?: number;
  onOpenGifts?: () => void;
  interactionsLocked?: boolean;
  lockedInteractionLabel?: string;
}

function timeAgo(startedAt: string, t: ReturnType<typeof useTranslation>['t']): string {
  const ms = Date.now() - new Date(startedAt).getTime();
  const h = Math.floor(ms / 3600000);
  const m = Math.floor((ms % 3600000) / 60000);
  if (h > 0) return t('liveRoom.timeAgo.hours', { hours: h, minutes: m });
  return t('liveRoom.timeAgo.minutes', { minutes: Math.max(0, m) });
}

export function InfoBlock({
  stream,
  viewerCount,
  onOpenGifts,
  interactionsLocked,
  lockedInteractionLabel,
}: InfoBlockProps) {
  const { t } = useTranslation('pages');
  const lang = useLangStore((s) => s.lang);
  const isAuthed = useIsAuthed();
  const currentUser = useAuthStore((s) => s.user);
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const [expanded, setExpanded] = useState(false);
  const [reportTarget, setReportTarget] = useState<ReportTargetDraft | null>(null);
  const [likeBurstKey, setLikeBurstKey] = useState(0);
  const [shareOpen, setShareOpen] = useState(false);

  const channelId = stream.channelId || stream.channel;
  const streamId = stream.id;
  const channelName = streamChannelName(stream, currentUser);
  const isOwnChannel = Boolean(
    currentUser?.id && (stream.ownerId === currentUser.id || channelId === `ch-${currentUser.id}`),
  );

  const followState = useFollowState(channelId, !!channelId);
  const likeState = useLikeState(streamId, isAuthed);
  const follow = useFollow(channelId);
  const unfollow = useUnfollow(channelId);
  const like = useLike(streamId);
  const watchLaterMembership = useLibraryMembership(WATCH_LATER_KEY, stream.id, isAuthed);
  const likedMembership = useLibraryMembership(LIKED_STREAMS_KEY, stream.id, isAuthed);
  const saveWatchLater = useSaveUserLibraryItem(WATCH_LATER_KEY);
  const removeWatchLater = useRemoveUserLibraryItem(WATCH_LATER_KEY);

  const subscribed = followState.data?.following ?? false;
  const subscriberCount = followState.data?.subscriberCount ?? stream.subscriberCount ?? 0;
  const displaySubscriberCount = followState.isPending
    ? (stream.subscriberCount ?? subscriberCount)
    : subscriberCount;
  const likeInfo = likeState.data;
  const likes = likeInfo?.likes;
  const liked = likeInfo?.liked ?? likedMembership.isMember;
  const disliked = likeInfo?.disliked ?? false;
  const saved = watchLaterMembership.isMember;

  const handleSubscribe = () => {
    if (isOwnChannel) return;
    if (!isAuthed) {
      openLogin(() => follow.mutate());
      return;
    }
    if (subscribed) unfollow.mutate();
    else follow.mutate();
  };

  const handleLike = () => {
    const nextLiked = !liked;
    if (!isAuthed) {
      if (nextLiked) {
        saveToLibrary(LIKED_STREAMS_KEY, stream);
        likedMembership.setLocalMember(true);
        setLikeBurstKey((value) => value + 1);
        toast.success(t('liveRoom.addedLiked', { defaultValue: 'Added to liked live rooms.' }));
      } else {
        removeFromLibrary(LIKED_STREAMS_KEY, stream.id);
        likedMembership.setLocalMember(false);
      }
      openLogin();
      return;
    }
    if (nextLiked) {
      setLikeBurstKey((value) => value + 1);
      toast.success(t('liveRoom.addedLiked', { defaultValue: 'Added to liked live rooms.' }));
    }
    like.mutate(liked ? 'unlike' : 'like');
  };

  const handleDislike = () => {
    if (!isAuthed) {
      removeFromLibrary(LIKED_STREAMS_KEY, stream.id);
      likedMembership.setLocalMember(false);
      openLogin();
      return;
    }
    like.mutate(disliked ? 'undislike' : 'dislike');
  };

  const handleSave = () => {
    if (saved) {
      if (isAuthed) removeWatchLater.mutate(stream.id);
      else {
        removeFromLibrary(WATCH_LATER_KEY, stream.id);
        watchLaterMembership.setLocalMember(false);
      }
      toast.success(t('liveRoom.removedWatchLater', { defaultValue: 'Removed from Watch later.' }));
      return;
    }
    if (isAuthed) saveWatchLater.mutate({ stream });
    else {
      saveToLibrary(WATCH_LATER_KEY, stream);
      watchLaterMembership.setLocalMember(true);
    }
    toast.success(t('liveRoom.savedWatchLater', { defaultValue: 'Saved to Watch later.' }));
  };

  const handleGiftClick = () => {
    if (interactionsLocked) {
      toast.info(
        lockedInteractionLabel ??
          t('liveRoom.fanClubExclusive.giftLocked', {
            defaultValue: 'Join the fan club to send gifts in this room.',
          }),
      );
      return;
    }
    if (!isAuthed) {
      openLogin(() => onOpenGifts?.());
      return;
    }
    onOpenGifts?.();
  };

  const title = lang === 'ja' ? (stream.titleJa ?? stream.title) : stream.title;
  const category = lang === 'ja' ? (stream.categoryJa ?? stream.category) : stream.category;
  const channelPath = `/channel/${encodeURIComponent(channelId || stream.ownerId || channelName)}`;
  const description = stream.description?.trim() ?? '';

  const openReport = () => {
    const target: ReportTargetDraft = {
      targetType: 'room',
      targetId: stream.id,
      targetUrl: window.location.href,
      roomId: stream.id,
      channelId,
      targetOwnerId: stream.ownerId,
      targetOwnerName: channelName,
      targetTitle: title,
      targetText: description,
    };
    if (!isAuthed) {
      openLogin(() => setReportTarget(target));
      return;
    }
    setReportTarget(target);
  };

  return (
    <div>
      <h1 className="gl-title">{title}</h1>

      <div className="gl-info-row">
        <div className="gl-info-chan">
          <Link className="gl-info-chan-link" to={channelPath} aria-label={channelName}>
            <Avatar name={channelName} src={stream.avatar} size={40} />
            <div className="gl-info-chan-text">
              <div className="gl-info-chan-name">
                <span className="truncate">{channelName}</span>
                {stream.verified && <VerifiedBadge size={14} />}
              </div>
              <div className="gl-info-chan-subs">
                {t('liveRoom.subscribers', { count: displaySubscriberCount })}
              </div>
            </div>
          </Link>
          {!isOwnChannel && (
            <button
              className={cn('gl-sub-btn', subscribed && 'is-on')}
              onClick={handleSubscribe}
              disabled={follow.isPending || unfollow.isPending}
              aria-pressed={subscribed}
            >
              {subscribed ? (
                <>
                  <Bell size={16} />
                  <span>{t('liveRoom.subscribed')}</span>
                </>
              ) : (
                <span>{t('liveRoom.subscribe')}</span>
              )}
            </button>
          )}
        </div>

        <div className="gl-info-actions">
          <div className="gl-pill-group">
            <button
              className={cn('gl-pg-left', liked && 'is-on')}
              aria-label={t('liveRoom.like')}
              aria-pressed={liked}
              onClick={handleLike}
            >
              <ThumbsUp size={18} />
              {likes !== undefined && <span>{formatCount(likes)}</span>}
              {likeBurstKey > 0 && <LikeBurst key={likeBurstKey} />}
            </button>
            <div className="gl-pg-div" />
            <button
              className={cn('gl-pg-right', disliked && 'is-on')}
              aria-label={t('liveRoom.dislike')}
              aria-pressed={disliked}
              onClick={handleDislike}
            >
              <ThumbsDown size={18} />
            </button>
          </div>
          <button
            className={cn('gl-pg-solo', interactionsLocked && 'is-disabled')}
            aria-label={t('liveRoom.gift', { defaultValue: 'Gift' })}
            aria-disabled={interactionsLocked}
            onClick={handleGiftClick}
          >
            <Gift size={18} />
            <span>{t('liveRoom.gift', { defaultValue: 'Gift' })}</span>
          </button>
          <button
            className="gl-pg-solo"
            aria-label={t('liveRoom.share')}
            onClick={() => setShareOpen(true)}
          >
            <Share2 size={18} />
            <span>{t('liveRoom.share')}</span>
          </button>
          <button
            className={cn('gl-pg-solo', saved && 'is-on')}
            aria-label={t('liveRoom.save')}
            aria-pressed={saved}
            onClick={handleSave}
          >
            <Icons.WatchLater size={18} />
            <span>{saved ? t('liveRoom.saved') : t('liveRoom.save')}</span>
          </button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button className="gl-pg-solo gl-pg-more" aria-label={t('report.moreActions')}>
                <MoreHorizontal size={18} />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-44">
              <DropdownMenuItem className="gl-menu-danger" onSelect={openReport}>
                <Flag size={15} />
                {t('report.action')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <div className="gl-desc">
        <div className="gl-desc-meta">
          <span>{t('liveRoom.watching', { count: viewerCount ?? stream.viewers })}</span>
          <span>·</span>
          <span>{t('liveRoom.startedAt', { time: timeAgo(stream.startedAt, t) })}</span>
          <span>·</span>
          <span className="gl-desc-tag">#{category.replace(/\s+/g, '')}</span>
        </div>
        {description && (
          <>
            <div className={cn('gl-desc-body', !expanded && 'is-clamped')}>{description}</div>
            <button className="gl-desc-toggle" onClick={() => setExpanded((v) => !v)}>
              {expanded ? t('liveRoom.showLess') : t('liveRoom.showMore')}
            </button>
          </>
        )}
      </div>
      <ReportDialog
        open={Boolean(reportTarget)}
        target={reportTarget}
        onOpenChange={(open) => {
          if (!open) setReportTarget(null);
        }}
      />
      <ShareDialog
        open={shareOpen}
        onOpenChange={setShareOpen}
        title={title}
        url={window.location.href}
        description={description}
        previewImage={stream.cover}
        previewKicker={t('shareDialog.liveKicker', { defaultValue: 'Live room' })}
        previewMeta={`${channelName}${t('shareDialog.metaSeparator', { defaultValue: ' · ' })}${t(
          'liveRoom.watching',
          { count: viewerCount ?? stream.viewers },
        )}`}
      />
    </div>
  );
}

function LikeBurst() {
  return (
    <span className="gl-like-burst" aria-hidden="true">
      <i />
      <i />
      <i />
      <i />
      <i />
      <b />
    </span>
  );
}
