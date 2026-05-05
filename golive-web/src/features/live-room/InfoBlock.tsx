import { useState } from 'react';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { toast } from 'sonner';
import {
  ThumbsUp,
  ThumbsDown,
  Share2,
  Bell,
  Gift,
  MoreHorizontal,
  Flag,
} from 'lucide-react';
import { Avatar } from '@/components/Avatar';
import { UserLevelBadge } from '@/components/UserLevelBadge';
import { Icons } from '@/components/Icons';
import { VerifiedBadge } from '@/components/VerifiedBadge';
import { useLangStore } from '@/stores/useLangStore';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useFollowState, useFollow, useUnfollow, useLikeState, useLike } from '@/api/room';
import { usePublicUser } from '@/api/auth';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ReportDialog, type ReportTargetDraft } from '@/features/reporting/ReportDialog';
import { cn } from '@/lib/cn';
import { copyText } from '@/lib/clipboard';
import {
  LIKED_STREAMS_KEY,
  WATCH_LATER_KEY,
  isInLibrary,
  removeFromLibrary,
  saveToLibrary,
} from '@/lib/liveLibrary';
import { streamChannelName, type Stream } from '@/types/stream';

export interface InfoBlockProps {
  stream: Stream;
  viewerCount?: number;
  onOpenGifts?: () => void;
}

function timeAgo(startedAt: string, t: ReturnType<typeof useTranslation>['t']): string {
  const ms = Date.now() - new Date(startedAt).getTime();
  const h = Math.floor(ms / 3600000);
  const m = Math.floor((ms % 3600000) / 60000);
  if (h > 0) return t('liveRoom.timeAgo.hours', { hours: h, minutes: m });
  return t('liveRoom.timeAgo.minutes', { minutes: Math.max(0, m) });
}

export function InfoBlock({ stream, viewerCount, onOpenGifts }: InfoBlockProps) {
  const { t } = useTranslation('pages');
  const lang = useLangStore((s) => s.lang);
  const isAuthed = useIsAuthed();
  const currentUser = useAuthStore((s) => s.user);
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const [expanded, setExpanded] = useState(false);
  const [reportTarget, setReportTarget] = useState<ReportTargetDraft | null>(null);

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
  const [saved, setSaved] = useState(() => isInLibrary(WATCH_LATER_KEY, stream.id));
  const [localLiked, setLocalLiked] = useState(() => isInLibrary(LIKED_STREAMS_KEY, stream.id));
  const ownerProfile = usePublicUser(stream.ownerId ?? '');
  const ownerLevelInfo = isOwnChannel ? currentUser?.levelInfo : ownerProfile.data?.levelInfo;

  useEffect(() => {
    setSaved(isInLibrary(WATCH_LATER_KEY, stream.id));
    setLocalLiked(isInLibrary(LIKED_STREAMS_KEY, stream.id));
  }, [stream.id]);

  const subscribed = followState.data?.following ?? false;
  const subscriberCount = followState.data?.subscriberCount ?? stream.subscriberCount ?? 0;
  const displaySubscriberCount = followState.isPending
    ? (stream.subscriberCount ?? subscriberCount)
    : subscriberCount;
  const likeInfo = likeState.data;
  const baseLikes = Math.floor(stream.viewers * 0.3);
  const likes = likeInfo?.likes ?? baseLikes;
  const liked = likeInfo?.liked ?? localLiked;
  const disliked = likeInfo?.disliked ?? false;

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
    if (nextLiked) {
      saveToLibrary(LIKED_STREAMS_KEY, stream);
      toast.success(t('liveRoom.addedLiked', { defaultValue: 'Added to liked live rooms.' }));
    } else {
      removeFromLibrary(LIKED_STREAMS_KEY, stream.id);
    }
    setLocalLiked(nextLiked);

    if (!isAuthed) {
      openLogin();
      return;
    }
    like.mutate(liked ? 'unlike' : 'like');
  };

  const handleDislike = () => {
    removeFromLibrary(LIKED_STREAMS_KEY, stream.id);
    setLocalLiked(false);
    if (!isAuthed) {
      openLogin();
      return;
    }
    like.mutate(disliked ? 'undislike' : 'dislike');
  };

  const handleSave = () => {
    if (saved) {
      removeFromLibrary(WATCH_LATER_KEY, stream.id);
      setSaved(false);
      toast.success(t('liveRoom.removedWatchLater', { defaultValue: 'Removed from Watch later.' }));
      return;
    }
    saveToLibrary(WATCH_LATER_KEY, stream);
    setSaved(true);
    toast.success(t('liveRoom.savedWatchLater', { defaultValue: 'Saved to Watch later.' }));
  };

  const handleGiftClick = () => {
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

  const handleShare = async () => {
    try {
      const method = await copyText(window.location.href, t('liveRoom.copyTarget'));
      if (method === 'manual') {
        toast.info(
          t('liveRoom.copyManual', { defaultValue: 'Live room link opened for manual copy.' }),
        );
      } else {
        toast.success(t('liveRoom.copySuccess', { defaultValue: 'Live room link copied.' }));
      }
    } catch {
      toast.error(t('liveRoom.copyFailed', { defaultValue: 'Could not copy the live room link.' }));
    }
  };

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
                <UserLevelBadge levelInfo={ownerLevelInfo} size="compact" />
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
              <span>{likes.toLocaleString()}</span>
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
            className="gl-pg-solo"
            aria-label={t('liveRoom.gift', { defaultValue: 'Gift' })}
            onClick={handleGiftClick}
          >
            <Gift size={18} />
            <span>{t('liveRoom.gift', { defaultValue: 'Gift' })}</span>
          </button>
          <button className="gl-pg-solo" aria-label={t('liveRoom.share')} onClick={handleShare}>
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
    </div>
  );
}
