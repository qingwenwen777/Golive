import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Inbox, CloudOff } from 'lucide-react';
import { CategoryChips } from '@/components/CategoryChips';
import { LiveCard } from '@/components/LiveCard';
import { LiveCardSkeleton } from '@/components/Skeleton';
import { useRooms } from '@/api/room';
import { useAuthStore } from '@/stores/useAuthStore';
import type { Stream } from '@/types/stream';

export default function HomePage() {
  const { t } = useTranslation('pages');
  const [activeCat, setActiveCat] = useState<string>('All');

  const categoryParam = activeCat === 'All' ? undefined : activeCat;
  const { data, isPending, isError, isFetching, refetch } = useRooms({ category: categoryParam });
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

  return (
    <>
      <div className="gl-chips-wrap">
        <CategoryChips active={activeCat} onPick={setActiveCat} />
      </div>
      <div className="gl-page">
        <div className="gl-home-head">
          <div className="gl-home-h2">{t('home.liveNow')}</div>
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
        ) : (
          <div
            className="gl-grid"
            style={{ opacity: isFetching ? 0.7 : 1, transition: 'opacity .15s' }}
          >
            {sortedItems.map((s, i) => (
              <LiveCard key={s.id} stream={s} priority={i < 4} />
            ))}
          </div>
        )}
      </div>
    </>
  );
}
