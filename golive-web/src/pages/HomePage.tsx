import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useSearchParams } from 'react-router-dom';
import {
  Bell,
  CheckCircle2,
  CloudOff,
  Coins,
  Gift as GiftIcon,
  Inbox,
  LockKeyhole,
  Radio,
  Sparkles,
  UserPlus,
} from 'lucide-react';
import { CategoryChips } from '@/components/CategoryChips';
import { AppointmentViewerCard } from '@/components/AppointmentViewerCard';
import { LiveCard } from '@/components/LiveCard';
import { LiveCardSkeleton } from '@/components/Skeleton';
import { useGifts } from '@/api/gift';
import {
  useFollow,
  useRecommendedCreators,
  useRooms,
  useUnfollow,
  useUpcomingAppointments,
  type RecommendedCreator,
} from '@/api/room';
import { Avatar } from '@/components/Avatar';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import { cn } from '@/lib/cn';
import { localizedGiftName } from '@/lib/gift';
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
          <HomeNoLiveEmpty />
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

        <UpcomingAppointmentsSection />
        <HomeRecommendationsSection />
        <HomeLevelGiftsSection />
      </div>
    </>
  );
}

function UpcomingAppointmentsSection() {
  const { t } = useTranslation('pages');
  const appointments = useUpcomingAppointments(1, 100);
  const items = appointments.data?.items ?? [];

  return (
    <section className="gl-home-my-appointments">
      <div className="gl-section-title-row">
        <div>
          <h2>{t('home.upcoming.title')}</h2>
          <span>{t('home.upcoming.subtitle')}</span>
        </div>
      </div>
      {appointments.isPending ? (
        <div className="gl-grid" aria-busy="true">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="h-[280px] animate-pulse rounded-card bg-bg-hover" />
          ))}
        </div>
      ) : items.length > 0 ? (
        <div className="gl-home-appointment-grid">
          {items.map((item) => (
            <AppointmentViewerCard
              key={item.id}
              appointment={item}
              to={`/live/${encodeURIComponent(item.roomId)}`}
              compact
            />
          ))}
        </div>
      ) : (
        <div className="gl-creator-empty-soft">{t('home.upcoming.empty')}</div>
      )}
    </section>
  );
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

function HomeRecommendationsSection() {
  const { t } = useTranslation('pages');
  const recommendations = useRecommendedCreators(8);

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

function HomeLevelGiftsSection() {
  const { t, i18n } = useTranslation('pages');
  const gifts = useGifts();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const levelGifts = useMemo(
    () =>
      (gifts.data ?? [])
        .filter((gift) => (gift.unlockLevel ?? 1) > 1)
        .sort(
          (a, b) =>
            (a.unlockLevel ?? 1) - (b.unlockLevel ?? 1) ||
            a.priceCoin - b.priceCoin ||
            a.id.localeCompare(b.id),
        )
        .slice(0, 8),
    [gifts.data],
  );

  return (
    <section
      className="gl-home-level-gifts"
      aria-label={t('home.levelGifts.title', { defaultValue: 'Level gift showcase' })}
    >
      <div className="gl-section-title-row">
        <div>
          <h2>{t('home.levelGifts.title', { defaultValue: 'Level gift showcase' })}</h2>
          <span>
            {t('home.levelGifts.subtitle', {
              defaultValue: 'Charge to level up and unlock premium gifts for live rooms.',
            })}
          </span>
        </div>
      </div>

      {gifts.isPending ? (
        <div className="gl-home-level-gift-grid" aria-busy="true">
          {Array.from({ length: 4 }).map((_, index) => (
            <div className="gl-home-level-gift-card is-loading" key={index} />
          ))}
        </div>
      ) : levelGifts.length > 0 ? (
        <div className="gl-home-level-gift-grid">
          {levelGifts.map((gift) => {
            const unlockLevel = gift.unlockLevel ?? 1;
            const giftName = localizedGiftName(gift, locale, t);
            const tierLevel = gift.tier + 1;
            return (
              <article className="gl-home-level-gift-card" key={gift.id}>
                <div className="gl-home-level-gift-top">
                  <span className="gl-home-level-gift-unlock">
                    <LockKeyhole size={12} />
                    {t('home.levelGifts.unlockLevel', {
                      level: unlockLevel,
                      defaultValue: 'Unlocks at Lv.{{level}}',
                    })}
                  </span>
                  <span className={cn('gl-home-level-gift-tier', `is-tier-${tierLevel}`)}>
                    {unlockLevel >= 60 ? <Sparkles size={12} /> : <GiftIcon size={12} />}
                    {t('home.levelGifts.tier', {
                      tier: tierLevel,
                      defaultValue: 'Tier {{tier}}',
                    })}
                  </span>
                </div>
                <div className="gl-home-level-gift-icon" aria-hidden>
                  {gift.icon}
                </div>
                <h3 title={giftName}>{giftName}</h3>
                <div className={cn('gl-home-level-gift-meta', `is-tier-${tierLevel}`)}>
                  <Coins size={14} />
                  <span>
                    {t('home.levelGifts.price', {
                      coins: gift.priceCoin.toLocaleString(locale),
                      defaultValue: '{{coins}} coins',
                    })}
                  </span>
                </div>
              </article>
            );
          })}
        </div>
      ) : (
        <div className="gl-creator-empty-soft">
          {t('home.levelGifts.empty', { defaultValue: 'No level gifts yet.' })}
        </div>
      )}
    </section>
  );
}

function RecommendedCreatorCard({ creator }: { creator: RecommendedCreator }) {
  const { t, i18n } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const follow = useFollow(creator.channelId);
  const unfollow = useUnfollow(creator.channelId);
  const pending = follow.isPending || unfollow.isPending;
  const channelPath = `/channel/${encodeURIComponent(creator.channelId)}`;

  const toggleFollow = () => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (creator.following) unfollow.mutate();
    else follow.mutate();
  };

  return (
    <article className="gl-home-rec-card">
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
      <button
        type="button"
        className={creator.following ? 'gl-secondary-btn' : 'gl-retry-btn'}
        disabled={pending}
        onClick={toggleFollow}
      >
        {creator.following ? <Bell size={15} /> : <UserPlus size={15} />}
        {creator.following
          ? t('home.recommendations.subscribed')
          : t('home.recommendations.subscribe')}
      </button>
    </article>
  );
}

function formatRecommendationTime(value: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value));
}
