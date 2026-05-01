import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router-dom';
import { Inbox, CloudOff } from 'lucide-react';
import { CategoryChips } from '@/components/CategoryChips';
import { LiveCard } from '@/components/LiveCard';
import { LiveCardSkeleton } from '@/components/Skeleton';
import { useRooms } from '@/api/room';
import { useAuthStore } from '@/stores/useAuthStore';
import { streamChannelName, type Stream } from '@/types/stream';

export default function HomePage() {
  const { t } = useTranslation('pages');
  const [searchParams] = useSearchParams();
  const [activeCat, setActiveCat] = useState<string>('All');
  const searchQuery = searchParams.get('q')?.trim() ?? '';

  const categoryParam = activeCat === 'All' ? undefined : activeCat;
  const { data, isPending, isError, isFetching, refetch } = useRooms({
    category: categoryParam,
    size: 100,
  });
  const userId = useAuthStore((s) => s.user?.id);

  // Pin the current user's own active stream to the front, then sort by
  // startedAt DESC. Backend already orders by started_at DESC but we re-sort
  // defensively (e.g. stale cache slices).
  const sortedItems = useMemo<Stream[]>(() => {
    const items = data?.items ?? [];
    const own: Stream[] = [];
    const rest: Stream[] = [];
    for (const s of items) {
      if (userId && (s.ownerId === userId || s.id === `live-${userId}`)) own.push(s);
      else rest.push(s);
    }
    rest.sort((a, b) => (a.startedAt < b.startedAt ? 1 : a.startedAt > b.startedAt ? -1 : 0));
    return [...own, ...rest];
  }, [data?.items, userId]);

  const visibleItems = useMemo<Stream[]>(() => {
    const q = searchQuery.toLowerCase();
    if (!q) return sortedItems;

    return sortedItems.filter((stream) => {
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
  }, [searchQuery, sortedItems]);

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
              : t('home.liveNow')}
          </div>
        </div>

        {isPending ? (
          <div className="gl-grid" aria-busy="true">
            {Array.from({ length: 8 }).map((_, i) => (
              <LiveCardSkeleton key={i} />
            ))}
          </div>
        ) : isError ? (
          <div className="gl-error" role="alert">
            <CloudOff size={64} strokeWidth={1.5} />
            <div className="gl-empty-title">{t('home.errorTitle')}</div>
            <div className="gl-empty-sub">{t('home.errorSub')}</div>
            <button className="gl-retry-btn" onClick={() => refetch()}>
              {t('home.retry')}
            </button>
          </div>
        ) : data && data.items.length === 0 ? (
          <div className="gl-empty">
            <Inbox size={64} strokeWidth={1.5} />
            <div className="gl-empty-title">{t('home.empty')}</div>
            <div className="gl-empty-sub">{t('home.emptySub')}</div>
          </div>
        ) : visibleItems.length === 0 ? (
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
          <div
            className="gl-grid"
            style={{ opacity: isFetching ? 0.7 : 1, transition: 'opacity .15s' }}
          >
            {visibleItems.map((s, i) => (
              <LiveCard key={s.id} stream={s} priority={i < 4} />
            ))}
          </div>
        )}
      </div>
    </>
  );
}
