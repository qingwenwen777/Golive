import { useState } from 'react';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { ThumbsUp, ThumbsDown, Share2, Bookmark, Bell, Gift } from 'lucide-react';
import { Avatar } from '@/components/Avatar';
import { Icons } from '@/components/Icons';
import { useLangStore } from '@/stores/useLangStore';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useFollowState, useFollow, useUnfollow, useLikeState, useLike } from '@/api/room';
import { cn } from '@/lib/cn';
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

function timeAgo(startedAt: string, lang: 'en' | 'ja'): string {
  const ms = Date.now() - new Date(startedAt).getTime();
  const h = Math.floor(ms / 3600000);
  const m = Math.floor((ms % 3600000) / 60000);
  if (lang === 'ja') {
    if (h > 0) return `${h}時間${m}分前`;
    return `${m}分前`;
  }
  if (h > 0) return `${h}h ${m}m ago`;
  return `${m}m ago`;
}

export function InfoBlock({ stream, viewerCount, onOpenGifts }: InfoBlockProps) {
  const { t } = useTranslation('pages');
  const lang = useLangStore((s) => s.lang);
  const isAuthed = useIsAuthed();
  const currentUser = useAuthStore((s) => s.user);
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const [expanded, setExpanded] = useState(false);

  const channelId = stream.channelId || stream.channel;
  const streamId = stream.id;
  const channelName = streamChannelName(stream, currentUser);

  const followState = useFollowState(channelId, !!channelId);
  const likeState = useLikeState(streamId, isAuthed);
  const follow = useFollow(channelId);
  const unfollow = useUnfollow(channelId);
  const like = useLike(streamId);
  const [saved, setSaved] = useState(() => isInLibrary(WATCH_LATER_KEY, stream.id));
  const [localLiked, setLocalLiked] = useState(() => isInLibrary(LIKED_STREAMS_KEY, stream.id));

  useEffect(() => {
    setSaved(isInLibrary(WATCH_LATER_KEY, stream.id));
    setLocalLiked(isInLibrary(LIKED_STREAMS_KEY, stream.id));
  }, [stream.id]);

  const subscribed = followState.data?.following ?? false;
  const subscriberCount =
    followState.data?.subscriberCount ?? stream.subscriberCount ?? 0;
  const displaySubscriberCount = followState.isPending
    ? (stream.subscriberCount ?? subscriberCount)
    : subscriberCount;
  const likeInfo = likeState.data;
  const baseLikes = Math.floor(stream.viewers * 0.3);
  const likes = likeInfo?.likes ?? baseLikes;
  const liked = likeInfo?.liked ?? localLiked;
  const disliked = likeInfo?.disliked ?? false;

  const handleSubscribe = () => {
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
      toast.success('Added to liked live rooms.');
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
      toast.success('Removed from Watch later.');
      return;
    }
    saveToLibrary(WATCH_LATER_KEY, stream);
    setSaved(true);
    toast.success('Saved to Watch later.');
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

  const description = stream.description?.trim() ?? '';

  return (
    <div>
      <h1 className="gl-title">{title}</h1>

      <div className="gl-info-row">
        <div className="gl-info-chan">
          <Avatar name={channelName} src={stream.avatar} size={40} />
          <div className="gl-info-chan-text">
            <div className="gl-info-chan-name">
              <span className="truncate">{channelName}</span>
              {stream.verified && <Icons.BadgeCheck size={14} className="text-text-secondary" />}
            </div>
            <div className="gl-info-chan-subs">
              {t('liveRoom.subscribers', { count: displaySubscriberCount })}
            </div>
          </div>
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
          <button className="gl-pg-solo" aria-label="Gift" onClick={handleGiftClick}>
            <Gift size={18} />
            <span>Gift</span>
          </button>
          <button className="gl-pg-solo" aria-label={t('liveRoom.share')}>
            <Share2 size={18} />
            <span>{t('liveRoom.share')}</span>
          </button>
          <button
            className={cn('gl-pg-solo', saved && 'is-on')}
            aria-label={t('liveRoom.save')}
            aria-pressed={saved}
            onClick={handleSave}
          >
            <Bookmark size={18} />
            <span>{saved ? 'Saved' : t('liveRoom.save')}</span>
          </button>
        </div>
      </div>

      <div className="gl-desc">
        <div className="gl-desc-meta">
          <span>{t('liveRoom.watching', { count: viewerCount ?? stream.viewers })}</span>
          <span>·</span>
          <span>
            {lang === 'ja'
              ? `${timeAgo(stream.startedAt, 'ja')}配信開始`
              : `started ${timeAgo(stream.startedAt, 'en')}`}
          </span>
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
    </div>
  );
}
