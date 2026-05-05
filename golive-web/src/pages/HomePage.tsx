import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useSearchParams } from 'react-router-dom';
import { Bell, CheckCircle2, CloudOff, Inbox, Radio, RefreshCw, UserPlus } from 'lucide-react';
import { CategoryChips } from '@/components/CategoryChips';
import { AppointmentViewerCard } from '@/components/AppointmentViewerCard';
import { LiveCard } from '@/components/LiveCard';
import { LoadableImage } from '@/components/LoadableImage';
import { LiveCardSkeleton } from '@/components/Skeleton';
import {
  useFollow,
  useHotReplays,
  useInfiniteRooms,
  useRecommendedCreators,
  useRecommendedRooms,
  useUnfollow,
  useUpcomingAppointments,
  type HotReplayItem,
  type RecommendedCreator,
} from '@/api/room';
import { Avatar } from '@/components/Avatar';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useIsAuthed } from '@/stores/useAuthStore';
import { cn } from '@/lib/cn';
import { useCoverHoverStyle } from '@/hooks/useCoverHoverStyle';
import { streamChannelName, type Stream } from '@/types/stream';

const UPCOMING_APPOINTMENT_LIMIT = 3;
const RECOMMENDED_LIVE_LIMIT = 12;
const LIVE_PAGE_SIZE = 12;

export default function HomePage() {
  const { t } = useTranslation('pages');
  const [searchParams] = useSearchParams();
  const [activeCat, setActiveCat] = useState<string>('All');
  const searchQuery = searchParams.get('q')?.trim() ?? '';

  const categoryParam = activeCat === 'All' ? undefined : activeCat;
  const recommended = useRecommendedRooms({
    category: categoryParam,
    size: RECOMMENDED_LIVE_LIMIT,
  });
  const recommendedItems = useMemo(
    () => filterStreamsBySearch(recommended.data?.items ?? [], searchQuery),
    [recommended.data?.items, searchQuery],
  );

  return (
    <>
      <div className="gl-chips-wrap">
        <CategoryChips active={activeCat} onPick={setActiveCat} />
      </div>
      <div className="gl-page">
        <div className="gl-home-head">
          <div className="gl-home-h2">
            {searchQuery
              ? t('home.searchResults', {
                  query: searchQuery,
                  defaultValue: `Search results for "${searchQuery}"`,
                })
              : t('home.recommendedLive', { defaultValue: '推荐直播' })}
          </div>
        </div>

        {recommended.isPending ? (
          <div className="gl-grid" aria-busy="true">
            {Array.from({ length: RECOMMENDED_LIVE_LIMIT }).map((_, i) => (
              <LiveCardSkeleton key={i} />
            ))}
          </div>
        ) : recommended.isError ? (
          <div className="gl-error" role="alert">
            <CloudOff size={64} strokeWidth={1.5} />
            <div className="gl-empty-title">{t('home.errorTitle')}</div>
            <div className="gl-empty-sub">{t('home.errorSub')}</div>
            <button className="gl-retry-btn" onClick={() => recommended.refetch()}>
              {t('home.retry')}
            </button>
          </div>
        ) : recommended.data && recommended.data.items.length === 0 ? (
          <HomeNoLiveEmpty />
        ) : recommendedItems.length === 0 ? (
          <div className="gl-empty">
            <Inbox size={64} strokeWidth={1.5} />
            <div className="gl-empty-title">
              {t('home.searchEmpty', { defaultValue: 'No matching live rooms' })}
            </div>
            <div className="gl-empty-sub">
              {t('home.searchEmptySub', { defaultValue: 'Try another keyword.' })}
            </div>
          </div>
        ) : (
          <div className="gl-grid">
            {recommendedItems.map((s, i) => (
              <LiveCard key={s.id} stream={s} priority={i < 4} />
            ))}
          </div>
        )}

        <UpcomingAppointmentsSection category={categoryParam} />
        <HomeRecommendationsSection category={categoryParam} />
        <HomeHotReplaysSection category={categoryParam} />
        <HomeAllLiveSection category={categoryParam} searchQuery={searchQuery} />
      </div>
    </>
  );
}

function UpcomingAppointmentsSection({ category }: { category?: string }) {
  const { t } = useTranslation('pages');
  const [shuffleSeed, setShuffleSeed] = useState(() => Date.now());
  const appointments = useUpcomingAppointments(1, 100, category);
  const items = useMemo(() => appointments.data?.items ?? [], [appointments.data?.items]);
  const visibleItems = useMemo(
    () =>
      items
        .map((item, index) => ({
          item,
          score: appointmentShuffleScore(`${item.id}:${item.scheduledAt}:${index}`, shuffleSeed),
        }))
        .sort((a, b) => a.score - b.score)
        .slice(0, UPCOMING_APPOINTMENT_LIMIT)
        .map(({ item }) => item),
    [items, shuffleSeed],
  );

  const refreshAppointments = () => {
    setShuffleSeed((current) => {
      const next = Math.floor(Math.random() * Number.MAX_SAFE_INTEGER);
      return next === current ? current + 1 : next;
    });
  };

  return (
    <section className="gl-home-my-appointments">
      <div className="gl-section-title-row">
        <div>
          <h2>{t('home.upcoming.title')}</h2>
          <span>{t('home.upcoming.subtitle')}</span>
        </div>
        <button
          type="button"
          className="gl-home-upcoming-refresh"
          disabled={appointments.isPending || items.length <= 1}
          aria-label={t('home.upcoming.refresh', { defaultValue: '刷新预约推荐' })}
          onClick={refreshAppointments}
        >
          <RefreshCw size={16} />
          <span>{t('home.upcoming.refresh', { defaultValue: '刷新' })}</span>
        </button>
      </div>
      {appointments.isPending ? (
        <div className="gl-grid" aria-busy="true">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="h-[280px] animate-pulse rounded-card bg-bg-hover" />
          ))}
        </div>
      ) : items.length > 0 ? (
        <div className="gl-home-appointment-grid">
          {visibleItems.map((item) => (
            <AppointmentViewerCard
              key={item.id}
              appointment={item}
              to={`/live/${encodeURIComponent(item.roomId)}`}
            />
          ))}
        </div>
      ) : (
        <div className="gl-creator-empty-soft">{t('home.upcoming.empty')}</div>
      )}
    </section>
  );
}

function appointmentShuffleScore(key: string, seed: number): number {
  let hash = seed >>> 0;
  for (let index = 0; index < key.length; index += 1) {
    hash ^= key.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return hash >>> 0;
}

function HomeNoLiveEmpty() {
  const { t } = useTranslation('pages');

  return (
    <div className="gl-empty gl-home-empty-compact">
      <Inbox size={56} strokeWidth={1.5} />
      <div className="gl-empty-title">{t('home.empty')}</div>
      <div className="gl-empty-sub">{t('home.emptySub')}</div>
    </div>
  );
}

function HomeRecommendationsSection({ category }: { category?: string }) {
  const { t } = useTranslation('pages');
  const recommendations = useRecommendedCreators(8, true, category);

  return (
    <section className="gl-home-recs" aria-label={t('home.recommendations.title')}>
      <div className="gl-section-title-row">
        <div>
          <h2>{t('home.recommendations.title')}</h2>
          <span>{t('home.recommendations.subtitle')}</span>
        </div>
      </div>
      {recommendations.isPending ? (
        <div className="gl-home-rec-grid" aria-busy="true">
          {Array.from({ length: 4 }).map((_, index) => (
            <div className="gl-home-rec-card is-loading" key={index} />
          ))}
        </div>
      ) : recommendations.data?.items.length ? (
        <div className="gl-home-rec-grid">
          {recommendations.data.items.map((creator) => (
            <RecommendedCreatorCard key={creator.channelId} creator={creator} />
          ))}
        </div>
      ) : (
        <div className="gl-creator-empty-soft">{t('home.recommendations.empty')}</div>
      )}
    </section>
  );
}

function HomeHotReplaysSection({ category }: { category?: string }) {
  const { t } = useTranslation('pages');
  const hotReplays = useHotReplays(3, 3, category);
  const items = hotReplays.data?.items ?? [];

  return (
    <section
      className="gl-home-hot-replays"
      aria-label={t('home.hotReplays.title', { defaultValue: 'Hot live replays' })}
    >
      <div className="gl-section-title-row">
        <div>
          <h2>{t('home.hotReplays.title', { defaultValue: 'Hot live replays' })}</h2>
          <span>
            {t('home.hotReplays.subtitle', {
              defaultValue:
                'Top replays from the last three days by likes, comments, coins, and peak viewers.',
            })}
          </span>
        </div>
      </div>
      {hotReplays.isPending ? (
        <div className="gl-home-replay-grid" aria-busy="true">
          {Array.from({ length: 3 }).map((_, index) => (
            <div className="gl-home-replay-card is-loading" key={index} />
          ))}
        </div>
      ) : items.length > 0 ? (
        <div className="gl-home-replay-grid">
          {items.map((replay) => (
            <HotReplayCard key={replay.id} replay={replay} />
          ))}
        </div>
      ) : (
        <div className="gl-creator-empty-soft">
          {t('home.hotReplays.empty', { defaultValue: 'No hot replays in the last three days.' })}
        </div>
      )}
    </section>
  );
}

function HotReplayCard({ replay }: { replay: HotReplayItem }) {
  const { t, i18n } = useTranslation('pages');
  const channelName = streamChannelName(replay);
  const title = i18n.language === 'ja' ? (replay.titleJa ?? replay.title) : replay.title;
  const category =
    i18n.language === 'ja' ? (replay.categoryJa ?? replay.category) : replay.category;
  const hoverStyle = useCoverHoverStyle(replay.cover, channelName || title || replay.id);

  return (
    <Link
      className="gl-home-replay-card gl-video-hover-card"
      style={hoverStyle}
      to={`/live/${encodeURIComponent(replay.id)}`}
    >
      <div className={cn('gl-home-replay-cover', replay.cover && 'has-image')}>
        {replay.cover ? (
          <LoadableImage src={replay.cover} alt="" loading="lazy" />
        ) : (
          <span>{channelName.slice(0, 1).toUpperCase()}</span>
        )}
        <div className="gl-home-replay-badge">
          {t('liveRoom.replay.badge', { defaultValue: 'Replay' })}
        </div>
        <div className="gl-home-replay-duration">{replay.duration}</div>
      </div>
      <div className="gl-home-replay-body">
        <Avatar name={channelName} src={replay.avatar} size={44} />
        <div className="gl-home-replay-copy">
          <div className="gl-home-replay-title" title={title}>
            {title}
          </div>
          <div className="gl-home-replay-channel">{channelName}</div>
          <div className="gl-home-replay-meta">
            {t('liveRoom.replay.badge', { defaultValue: 'Replay' })} · {category}
          </div>
        </div>
      </div>
    </Link>
  );
}

function HomeAllLiveSection({ category, searchQuery }: { category?: string; searchQuery: string }) {
  const { t } = useTranslation('pages');
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const rooms = useInfiniteRooms({ category, size: LIVE_PAGE_SIZE });
  const { data, fetchNextPage, hasNextPage, isError, isFetchingNextPage, isPending, refetch } =
    rooms;
  const items = useMemo(() => data?.pages.flatMap((page) => page.items) ?? [], [data?.pages]);
  const visibleItems = useMemo(
    () => filterStreamsBySearch(items, searchQuery),
    [items, searchQuery],
  );

  useEffect(() => {
    const node = sentinelRef.current;
    if (!node || !hasNextPage) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting) && !isFetchingNextPage) {
          void fetchNextPage();
        }
      },
      { rootMargin: '640px 0px' },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [fetchNextPage, hasNextPage, isFetchingNextPage]);

  useEffect(() => {
    if (!searchQuery || visibleItems.length > 0 || !hasNextPage || isFetchingNextPage) {
      return;
    }
    void fetchNextPage();
  }, [fetchNextPage, hasNextPage, isFetchingNextPage, searchQuery, visibleItems.length]);

  return (
    <section
      className="gl-home-live-all"
      aria-label={t('home.allLive.title', { defaultValue: '更多直播' })}
    >
      <div className="gl-section-title-row">
        <div>
          <h2>{t('home.allLive.title', { defaultValue: '更多直播' })}</h2>
        </div>
      </div>

      {isPending ? (
        <div className="gl-grid" aria-busy="true">
          {Array.from({ length: LIVE_PAGE_SIZE }).map((_, index) => (
            <LiveCardSkeleton key={index} />
          ))}
        </div>
      ) : isError ? (
        <div className="gl-error" role="alert">
          <CloudOff size={56} strokeWidth={1.5} />
          <div className="gl-empty-title">{t('home.errorTitle')}</div>
          <button className="gl-retry-btn" onClick={() => refetch()}>
            {t('home.retry')}
          </button>
        </div>
      ) : visibleItems.length > 0 ? (
        <>
          <div className="gl-grid">
            {visibleItems.map((stream, index) => (
              <LiveCard key={stream.id} stream={stream} priority={index < 3} />
            ))}
          </div>
          <div ref={sentinelRef} className="gl-home-live-sentinel" aria-hidden />
          {hasNextPage && (
            <button
              type="button"
              className="gl-home-live-more"
              disabled={isFetchingNextPage}
              onClick={() => fetchNextPage()}
            >
              {isFetchingNextPage
                ? t('home.allLive.loading', { defaultValue: '加载中...' })
                : t('home.allLive.loadMore', { defaultValue: '加载更多' })}
            </button>
          )}
        </>
      ) : (
        <div className="gl-creator-empty-soft">
          {searchQuery
            ? t('home.searchEmpty', { defaultValue: 'No matching live rooms' })
            : t('home.empty', { defaultValue: 'No live streams right now' })}
        </div>
      )}
    </section>
  );
}

function RecommendedCreatorCard({ creator }: { creator: RecommendedCreator }) {
  const { t, i18n } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const [followAnimating, setFollowAnimating] = useState(false);
  const followAnimationFrame = useRef<number | null>(null);
  const followAnimationTimer = useRef<number | null>(null);
  const follow = useFollow(creator.channelId);
  const unfollow = useUnfollow(creator.channelId);
  const pending = follow.isPending || unfollow.isPending;
  const channelPath = `/channel/${encodeURIComponent(creator.channelId)}`;

  const runFollowAnimation = () => {
    if (followAnimationFrame.current !== null) {
      window.cancelAnimationFrame(followAnimationFrame.current);
    }
    if (followAnimationTimer.current !== null) {
      window.clearTimeout(followAnimationTimer.current);
    }
    setFollowAnimating(false);
    followAnimationFrame.current = window.requestAnimationFrame(() => {
      setFollowAnimating(true);
      followAnimationFrame.current = null;
    });
    followAnimationTimer.current = window.setTimeout(() => {
      setFollowAnimating(false);
      followAnimationTimer.current = null;
    }, 460);
  };

  useEffect(() => {
    return () => {
      if (followAnimationFrame.current !== null) {
        window.cancelAnimationFrame(followAnimationFrame.current);
      }
      if (followAnimationTimer.current !== null) {
        window.clearTimeout(followAnimationTimer.current);
      }
    };
  }, []);

  const toggleFollow = () => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    runFollowAnimation();
    if (creator.following) unfollow.mutate();
    else follow.mutate();
  };

  return (
    <article className="gl-home-rec-card">
      <div className="gl-home-rec-top">
        <Link className="gl-home-rec-main" to={channelPath}>
          <Avatar name={creator.name} src={creator.avatar} size={54} />
          <div className="gl-home-rec-copy">
            <h3>
              <span>{creator.name}</span>
              {creator.verified && <CheckCircle2 size={15} />}
            </h3>
            <span>
              {t('home.recommendations.followers', {
                count: creator.subscriberCount,
              })}
            </span>
          </div>
        </Link>
        <button
          type="button"
          className={cn(
            'gl-home-rec-follow',
            creator.following && 'is-following',
            followAnimating && 'is-transitioning',
          )}
          disabled={pending}
          aria-label={
            creator.following
              ? t('home.recommendations.subscribed')
              : t('home.recommendations.subscribe')
          }
          title={
            creator.following
              ? t('home.recommendations.subscribed')
              : t('home.recommendations.subscribe')
          }
          onClick={toggleFollow}
        >
          <span className="gl-home-rec-follow-icon" aria-hidden="true">
            {creator.following ? <Bell size={17} /> : <UserPlus size={17} />}
          </span>
        </button>
      </div>
      <div className="gl-home-rec-meta">
        <Radio size={14} />
        <span>
          {creator.lastLiveAt
            ? t('home.recommendations.lastLive', {
                time: formatRecommendationTime(creator.lastLiveAt, i18n.language),
              })
            : t('home.recommendations.noLive')}
        </span>
      </div>
      {creator.lastTitle && <p>{creator.lastTitle}</p>}
    </article>
  );
}

function filterStreamsBySearch(items: Stream[], searchQuery: string): Stream[] {
  const q = searchQuery.trim().toLowerCase();
  if (!q) return items;

  return items.filter((stream) => {
    const haystack = [
      stream.title,
      stream.titleJa,
      stream.description,
      stream.category,
      stream.categoryJa,
      stream.channel,
      stream.channelId,
      streamChannelName(stream),
      stream.id,
    ]
      .filter(Boolean)
      .join(' ')
      .toLowerCase();
    return haystack.includes(q);
  });
}

function formatRecommendationTime(value: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value));
}
