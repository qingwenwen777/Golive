import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate } from 'react-router-dom';
import { AxiosError } from 'axios';
import { useQueries } from '@tanstack/react-query';
import {
  AtSign,
  Ban,
  Bell,
  Camera,
  ChevronLeft,
  ChevronRight,
  Crown,
  Heart,
  History,
  KeyRound,
  Languages,
  Moon,
  PencilLine,
  Radio,
  ShieldCheck,
  Settings,
  Sparkles,
  Trash2,
  UserRound,
  UsersRound,
  Video,
  Wallet,
} from 'lucide-react';
import { toast } from 'sonner';
import {
  useBindGoogleAccount,
  useChangePassword,
  useMe,
  useUnbindGoogleAccount,
  useUpdateProfile,
} from '@/api/auth';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog';
import { useFanBadges } from '@/api/gift';
import { useBlockedUsers, useUnblockUser, type BlockedUser } from '@/api/messages';
import { useChannelPosts, useSubscriptionPosts } from '@/api/posts';
import {
  useChannelAppointments,
  useReservedAppointments,
  useRooms,
  useSubscriptions,
  useSubscriptionAppointments,
} from '@/api/room';
import { Avatar } from '@/components/Avatar';
import { GoogleIcon } from '@/components/GoogleIcon';
import { UserLevelBadge } from '@/components/UserLevelBadge';
import { AppointmentViewerCard } from '@/components/AppointmentViewerCard';
import { Icons } from '@/components/Icons';
import { LiveCard } from '@/components/LiveCard';
import { LiveCardSkeleton } from '@/components/Skeleton';
import { AvatarUploadDialog } from '@/features/account/AvatarUploadDialog';
import { PostCard } from '@/features/posts/PostCard';
import { cn } from '@/lib/cn';
import { fanBadgeToneClass } from '@/lib/fanBadgeTone';
import { http } from '@/lib/axios';
import { levelProgressRatio, normalizeLevelInfo } from '@/lib/userLevel';
import { GoogleIdentityButton, isGoogleConfigured } from '@/lib/googleIdentity';
import {
  LIKED_STREAMS_KEY,
  WATCH_HISTORY_KEY,
  WATCH_LATER_KEY,
  filterLibraryItemsByLiveRooms,
  readLibrary,
  removeFromLibrary,
  syncLibraryWithLiveRooms,
  type LibraryStream,
} from '@/lib/liveLibrary';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import { APP_LANGS, useLangStore } from '@/stores/useLangStore';
import { useThemeStore } from '@/stores/useThemeStore';
import type { FanBadge } from '@/types/gift';
import type { Stream } from '@/types/stream';
import { userDisplayName, type User } from '@/types/user';

export function SubscriptionsPage() {
  const { t } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const [appointmentPage, setAppointmentPage] = useState(1);
  const [selectedChannelId, setSelectedChannelId] = useState<'all' | string>('all');
  const subscriptions = useSubscriptions(isAuthed);
  const subscriptionAppointments = useSubscriptionAppointments(isAuthed, appointmentPage, 8);
  const subscriptionPosts = useSubscriptionPosts(isAuthed, 8);
  const channels = subscriptions.data?.items ?? [];
  const selectedChannel = useMemo(
    () => channels.find((channel) => channel.channelId === selectedChannelId),
    [channels, selectedChannelId],
  );
  const allSelected = selectedChannelId === 'all' || !selectedChannel;
  const channelAppointments = useChannelAppointments(
    selectedChannel?.key ?? '',
    isAuthed && !allSelected,
    appointmentPage,
    8,
  );
  const channelPosts = useChannelPosts(selectedChannel?.key ?? '', isAuthed && !allSelected, 1, 8);
  const visibleAppointments = allSelected ? subscriptionAppointments : channelAppointments;
  const visiblePosts = allSelected ? subscriptionPosts : channelPosts;
  const appointmentTotal = visibleAppointments.data?.total ?? 0;
  const appointmentPageSize = visibleAppointments.data?.size ?? 8;
  const appointmentPageCount = Math.max(1, Math.ceil(appointmentTotal / appointmentPageSize));
  const streams = useMemo(
    () =>
      channels.map(
        (channel): Stream =>
          channel.stream ??
          ({
            id: channel.channelId,
            title: channel.name,
            channel: channel.name,
            channelId: channel.channelId,
            verified: channel.verified,
            avatar: channel.avatar,
            cover: '',
            viewers: 0,
            duration: '',
            category: '',
            startedAt: '',
            isLive: false,
            status: channel.status || 'ended',
            subscriberCount: channel.subscriberCount,
          } as Stream),
      ),
    [channels],
  );
  const hydratedStreams = useReplayHydratedStreams(streams);

  useEffect(() => {
    setAppointmentPage(1);
  }, [isAuthed, selectedChannelId]);

  useEffect(() => {
    if (selectedChannelId !== 'all' && !selectedChannel && !subscriptions.isPending) {
      setSelectedChannelId('all');
    }
  }, [selectedChannel, selectedChannelId, subscriptions.isPending]);

  useEffect(() => {
    if (appointmentPage > appointmentPageCount) {
      setAppointmentPage(appointmentPageCount);
    }
  }, [appointmentPage, appointmentPageCount]);

  const visibleStreams = useMemo(() => {
    if (allSelected) return hydratedStreams.slice(0, 12);
    return hydratedStreams
      .filter((stream) => stream.channelId === selectedChannel?.channelId)
      .slice(0, 1);
  }, [allSelected, hydratedStreams, selectedChannel?.channelId]);
  const selectedChannelName = selectedChannel?.name ?? '';
  const selectedChannelUrl = selectedChannel
    ? `/channel/${encodeURIComponent(selectedChannel.key)}`
    : '';

  return (
    <div className="gl-page gl-library-page">
      {!isAuthed && (
        <div className="gl-yt-banner">
          <div>
            <strong>{t('library.subscriptions.signInTitle')}</strong>
            <span>{t('library.subscriptions.signInSub')}</span>
          </div>
          <button className="gl-secondary-btn" type="button" onClick={() => openLogin()}>
            {t('library.signIn')}
          </button>
        </div>
      )}

      <header className="gl-yt-page-head">
        <div className="gl-yt-page-icon">
          <Bell size={22} />
        </div>
        <h1>{t('library.subscriptions.title')}</h1>
      </header>

      {channels.length > 0 && (
        <section className="gl-yt-channel-rail-wrap">
          <div className="gl-yt-channel-rail" role="list">
            <button
              type="button"
              className={cn('gl-yt-channel-chip gl-yt-channel-all', allSelected && 'is-active')}
              role="listitem"
              aria-pressed={allSelected}
              onClick={() => setSelectedChannelId('all')}
            >
              <div className="gl-yt-channel-all-icon">
                <UsersRound size={28} />
              </div>
              <div className="gl-yt-channel-name" title={t('library.subscriptions.all')}>
                <span>{t('library.subscriptions.all')}</span>
              </div>
            </button>
            {channels.map((channel) => (
              <button
                type="button"
                key={channel.channelId}
                className={cn(
                  'gl-yt-channel-chip',
                  !allSelected && selectedChannel?.channelId === channel.channelId && 'is-active',
                )}
                role="listitem"
                aria-pressed={!allSelected && selectedChannel?.channelId === channel.channelId}
                onClick={() => setSelectedChannelId(channel.channelId)}
              >
                <div className="gl-yt-channel-avatar">
                  <Avatar name={channel.name} src={channel.avatar} size={64} />
                  {channel.live && <span className="gl-yt-live-dot" aria-hidden="true" />}
                </div>
                <div className="gl-yt-channel-name" title={channel.name}>
                  <span>{channel.name}</span>
                </div>
              </button>
            ))}
          </div>
        </section>
      )}

      {!allSelected && selectedChannelUrl && (
        <div className="gl-subscription-channel-action-row">
          <Link className="gl-subscription-channel-action" to={selectedChannelUrl}>
            {t('library.subscriptions.enterChannel')}
            <ChevronRight size={14} />
          </Link>
        </div>
      )}

      <section className="gl-library-section">
        <div className="gl-section-title-row">
          <div>
            <h2>
              {allSelected ? t('library.latest') : t('library.subscriptions.latest.channelTitle')}
            </h2>
            {!allSelected && (
              <span>
                {t('library.subscriptions.latest.channelSubtitle', { name: selectedChannelName })}
              </span>
            )}
          </div>
        </div>
        <StreamGrid
          isPending={isAuthed && subscriptions.isPending}
          streams={visibleStreams}
          emptyTitle={
            isAuthed
              ? t('library.subscriptions.emptyAuthed')
              : t('library.subscriptions.emptyGuest')
          }
          emptySub={t('library.subscriptions.emptySub')}
        />
      </section>

      <section className="gl-library-section">
        <div className="gl-section-title-row">
          <div>
            <h2>
              {allSelected
                ? t('library.subscriptions.appointments.title')
                : t('library.subscriptions.appointments.channelTitle')}
            </h2>
            <span>
              {allSelected
                ? t('library.subscriptions.appointments.subtitle')
                : t('library.subscriptions.appointments.channelSubtitle', {
                    name: selectedChannelName,
                  })}
            </span>
          </div>
        </div>
        {isAuthed && visibleAppointments.isPending ? (
          <div className="gl-grid" aria-busy="true">
            {Array.from({ length: 4 }).map((_, i) => (
              <LiveCardSkeleton key={i} />
            ))}
          </div>
        ) : isAuthed && visibleAppointments.data?.items.length ? (
          <>
            <div className="gl-appointment-grid">
              {visibleAppointments.data.items.map((item) => (
                <AppointmentViewerCard
                  key={item.id}
                  appointment={item}
                  to={`/live/${encodeURIComponent(item.roomId)}`}
                />
              ))}
            </div>
            {appointmentPageCount > 1 && (
              <AppointmentPager
                page={appointmentPage}
                pageCount={appointmentPageCount}
                total={appointmentTotal}
                pageSize={appointmentPageSize}
                onPageChange={setAppointmentPage}
              />
            )}
          </>
        ) : (
          <div className="gl-creator-empty-soft">
            {isAuthed
              ? allSelected
                ? t('library.subscriptions.appointments.empty')
                : t('library.subscriptions.appointments.channelEmpty')
              : t('library.subscriptions.appointments.signIn')}
          </div>
        )}
      </section>

      <section className="gl-library-section">
        <div className="gl-section-title-row">
          <div>
            <h2>
              {allSelected
                ? t('library.subscriptions.posts.title')
                : t('library.subscriptions.posts.channelTitle')}
            </h2>
            <span>
              {allSelected
                ? t('library.subscriptions.posts.subtitle')
                : t('library.subscriptions.posts.channelSubtitle', {
                    name: selectedChannelName,
                  })}
            </span>
          </div>
        </div>
        {isAuthed && visiblePosts.isPending ? (
          <div className="gl-post-feed-list gl-subscription-post-feed" aria-busy="true">
            {Array.from({ length: 2 }).map((_, index) => (
              <div className="gl-post-card is-loading" key={index} />
            ))}
          </div>
        ) : isAuthed && visiblePosts.data?.items.length ? (
          <div className="gl-post-feed-list gl-subscription-post-feed">
            {visiblePosts.data.items.map((post) => (
              <PostCard key={post.id} post={post} />
            ))}
          </div>
        ) : (
          <div className="gl-creator-empty-soft">
            {isAuthed
              ? allSelected
                ? t('library.subscriptions.posts.empty')
                : t('library.subscriptions.posts.channelEmpty')
              : t('library.subscriptions.posts.signIn')}
          </div>
        )}
      </section>
    </div>
  );
}

export function YouPage() {
  const { t, i18n } = useTranslation('pages');
  const navigate = useNavigate();
  const user = useAuthStore((s) => s.user);
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const me = useMe();
  const rooms = useRooms({ size: 100 });
  const fanBadges = useFanBadges(isAuthed, user?.id);
  const hydratedFanBadges = useHydratedFanBadges(fanBadges.data ?? []);
  const liveStreams = rooms.data?.items;
  const history = useMemo(
    () =>
      liveStreams
        ? filterLibraryItemsByLiveRooms(readLibrary(WATCH_HISTORY_KEY), liveStreams)
        : readLibrary(WATCH_HISTORY_KEY),
    [liveStreams],
  );
  const hydratedHistory = useReplayHydratedStreams(history);
  const saved = useMemo(
    () =>
      liveStreams
        ? filterLibraryItemsByLiveRooms(readLibrary(WATCH_LATER_KEY), liveStreams)
        : readLibrary(WATCH_LATER_KEY),
    [liveStreams],
  );
  const hydratedSaved = useReplayHydratedStreams(saved);
  const liked = useMemo(
    () =>
      liveStreams
        ? filterLibraryItemsByLiveRooms(readLibrary(LIKED_STREAMS_KEY), liveStreams)
        : readLibrary(LIKED_STREAMS_KEY),
    [liveStreams],
  );
  const hydratedLiked = useReplayHydratedStreams(liked);
  const displayName = userDisplayName(user);
  const ownLive = rooms.data?.items.find((stream) => stream.ownerId === user?.id);
  const balance = me.data?.coinBalance ?? user?.coinBalance ?? 0;
  const levelInfo = normalizeLevelInfo(me.data?.levelInfo ?? user?.levelInfo);
  const levelProgress = levelProgressRatio(levelInfo);
  const locale = i18n.resolvedLanguage ?? i18n.language;

  const handleOpenCoins = () => {
    if (!user) {
      openLogin();
      return;
    }
    navigate('/coins?focus=recharge');
  };

  const channelHref = `/channel/${user?.id ?? ''}`;

  return (
    <div className="gl-page gl-library-page">
      <section className="gl-yt-you-head">
        <Avatar name={displayName} src={user?.avatar} size={112} />
        <div className="gl-yt-you-main">
          <h1 className="gl-yt-you-title">
            <span>{displayName}</span>
            {isAuthed && <UserLevelBadge levelInfo={levelInfo} size="hero" />}
          </h1>
          <div className="gl-yt-you-meta">
            {isAuthed && user?.username && <span>@{user.username}</span>}
            {isAuthed ? (
              <Link className="gl-yt-you-link" to={channelHref}>
                {t('library.you.viewChannel')} <ChevronRight size={14} />
              </Link>
            ) : (
              <button className="gl-yt-you-link" type="button" onClick={() => openLogin()}>
                {t('library.signIn')} <ChevronRight size={14} />
              </button>
            )}
          </div>
          <div className="gl-yt-you-chips">
            <button
              type="button"
              className="gl-yt-coin-chip"
              onClick={handleOpenCoins}
              title={t('library.you.openCoinCenter')}
            >
              <Wallet size={14} />
              <span>{balance.toLocaleString()}</span>
              <span className="gl-yt-coin-chip-add">{t('library.you.recharge')}</span>
            </button>
            <Link className="gl-yt-chip" to="/watch-later">
              <Icons.WatchLater size={14} />{' '}
              {t('library.you.watchLaterCount', { count: hydratedSaved.length })}
            </Link>
            <Link className="gl-yt-chip" to="/liked">
              <Heart size={14} /> {t('library.you.likedCount', { count: hydratedLiked.length })}
            </Link>
            <Link className="gl-yt-chip" to="/history">
              <History size={14} />{' '}
              {t('library.you.historyCount', { count: hydratedHistory.length })}
            </Link>
            <Link className="gl-yt-chip" to="/settings">
              <Settings size={14} /> {t('library.you.settings')}
            </Link>
          </div>
          {isAuthed && (
            <div className="gl-you-level-progress">
              <div className="gl-you-level-progress-top">
                <span>
                  {t('library.you.levelProgress.title', {
                    level: levelInfo.level,
                    defaultValue: 'Lv.{{level}} identity',
                  })}
                </span>
                <strong>
                  {levelInfo.level >= levelInfo.maxLevel
                    ? t('library.you.levelProgress.maxLevel', { defaultValue: 'Max level' })
                    : t('library.you.levelProgress.toNext', {
                        level: levelInfo.level + 1,
                        coins: levelInfo.coinsToNextLevel.toLocaleString(locale),
                        defaultValue: '{{coins}} coins to Lv.{{level}}',
                      })}
                </strong>
              </div>
              <div className="gl-you-level-bar" aria-hidden>
                <i style={{ width: `${Math.round(levelProgress * 100)}%` }} />
              </div>
              <div className="gl-you-level-progress-bottom">
                <span>
                  {t('library.you.levelProgress.charged', {
                    coins: levelInfo.totalTopupCoins.toLocaleString(locale),
                    defaultValue: '{{coins}} charged',
                  })}
                </span>
                <span>
                  {t('library.you.levelProgress.target', {
                    coins: levelInfo.nextLevelTargetCoins.toLocaleString(locale),
                    defaultValue: '{{coins}} target',
                  })}
                </span>
              </div>
            </div>
          )}
        </div>
      </section>

      {ownLive && (
        <Shelf
          title={t('library.you.yourLiveRoom')}
          actionLabel={t('library.you.openStudio')}
          actionTo={`/live/${ownLive.id}`}
        >
          <div className="gl-feature-live">
            <LiveCard stream={ownLive} priority />
          </div>
        </Shelf>
      )}

      <Shelf title={t('library.you.fanBadges')}>
        <FanBadgeShelf
          badges={hydratedFanBadges}
          isPending={fanBadges.isPending && isAuthed}
          isAuthed={isAuthed}
          onLogin={openLogin}
        />
      </Shelf>

      <MyAppointmentsShelf />

      <Shelf
        title={t('library.you.history')}
        actionLabel={t('library.you.viewAll')}
        actionTo="/history"
      >
        <HorizontalShelf
          items={hydratedHistory.slice(0, 8)}
          emptyText={t('library.you.noHistory')}
        />
      </Shelf>

      <Shelf
        title={t('library.you.watchLater')}
        actionLabel={t('library.you.seeAll')}
        actionTo="/watch-later"
      >
        <HorizontalShelf
          items={hydratedSaved.slice(0, 8)}
          emptyText={t('library.you.noWatchLater')}
        />
      </Shelf>

      <Shelf
        title={t('library.you.likedRooms')}
        actionLabel={t('library.you.seeAll')}
        actionTo="/liked"
      >
        <HorizontalShelf items={hydratedLiked.slice(0, 8)} emptyText={t('library.you.noLiked')} />
      </Shelf>

      <Shelf title={t('library.you.quickActions')}>
        <div className="gl-yt-quick-row">
          <QuickChip
            icon={<Radio size={16} />}
            label={t('library.you.startLive')}
            onClick={() => navigate('/')}
          />
          <QuickChip
            icon={<Icons.WatchLater size={16} />}
            label={t('library.you.watchLater')}
            onClick={() => navigate('/watch-later')}
          />
          <QuickChip
            icon={<Heart size={16} />}
            label={t('library.you.liked')}
            onClick={() => navigate('/liked')}
          />
          <QuickChip
            icon={<Settings size={16} />}
            label={t('library.you.settings')}
            onClick={() => navigate('/settings')}
          />
        </div>
      </Shelf>
    </div>
  );
}

function useHydratedFanBadges(badges: FanBadge[]): FanBadge[] {
  const creatorIds = useMemo(
    () =>
      Array.from(
        new Set(
          badges
            .map((badge) => badge.creatorId)
            .filter((creatorId): creatorId is string => Boolean(creatorId)),
        ),
      ).slice(0, 60),
    [badges],
  );
  const profileLookups = useQueries({
    queries: creatorIds.map((creatorId) => ({
      queryKey: ['public-user', creatorId],
      queryFn: async ({ signal }): Promise<User> => {
        const { data } = await http.get<User>(`/users/profile/${encodeURIComponent(creatorId)}`, {
          signal,
        });
        return data;
      },
      staleTime: 0,
      refetchOnMount: 'always' as const,
      refetchOnWindowFocus: true,
      retry: 1,
    })),
  });
  const profiles = new Map<string, User>();
  profileLookups.forEach((query, index) => {
    if (query.data) profiles.set(creatorIds[index], query.data);
  });
  if (profiles.size === 0) return badges;
  return badges.map((badge) => {
    const profile = profiles.get(badge.creatorId);
    if (!profile) return badge;
    return {
      ...badge,
      creatorName: userDisplayName(profile),
      creatorAvatar: profile.avatar || badge.creatorAvatar,
    };
  });
}

function FanBadgeShelf({
  badges,
  isPending,
  isAuthed,
  onLogin,
}: {
  badges: FanBadge[];
  isPending: boolean;
  isAuthed: boolean;
  onLogin: () => void;
}) {
  const { t } = useTranslation('pages');
  const scrollerRef = useRef<HTMLDivElement | null>(null);
  const scrollBadges = (direction: -1 | 1) => {
    const node = scrollerRef.current;
    if (!node) return;
    node.scrollBy({ left: direction * 260, behavior: 'smooth' });
  };

  if (!isAuthed) {
    return (
      <button type="button" className="gl-fan-badge-empty" onClick={onLogin}>
        {t('library.fanBadges.signIn')}
      </button>
    );
  }

  if (isPending) {
    return (
      <div className="gl-fan-badge-rail-wrap">
        <div className="gl-fan-badge-grid">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="gl-fan-badge-card is-loading" />
          ))}
        </div>
      </div>
    );
  }

  if (badges.length === 0) {
    return <div className="gl-fan-badge-empty">{t('library.fanBadges.empty')}</div>;
  }

  return (
    <div className={cn('gl-fan-badge-rail-wrap', badges.length > 3 && 'has-overflow')}>
      <button
        type="button"
        className="gl-fan-badge-scroll is-left"
        aria-label={t('library.fanBadges.scrollLeft', { defaultValue: 'Scroll left' })}
        onClick={() => scrollBadges(-1)}
      >
        <ChevronLeft size={17} />
      </button>
      <div className="gl-fan-badge-grid" ref={scrollerRef}>
        {badges.slice(0, 12).map((badge) => (
          <div key={`${badge.creatorId}:${badge.level}`} className="gl-fan-badge-card">
            <div className="gl-fan-badge-avatar">
              <Avatar name={badge.creatorName} src={badge.creatorAvatar} size={42} />
            </div>
            <div className="gl-fan-badge-copy">
              <div className="gl-fan-badge-name">{badge.creatorName}</div>
              <div className="gl-fan-badge-meta">
                <span className={`gl-fan-badge-level ${fanBadgeToneClass(badge.level)}`}>
                  <Crown size={13} strokeWidth={2.4} /> #{badge.level}
                </span>
                <span>
                  {t('library.fanBadges.contribution', {
                    amount: badge.totalContribution.toLocaleString(),
                  })}
                </span>
              </div>
            </div>
          </div>
        ))}
      </div>
      <button
        type="button"
        className="gl-fan-badge-scroll is-right"
        aria-label={t('library.fanBadges.scrollRight', { defaultValue: 'Scroll right' })}
        onClick={() => scrollBadges(1)}
      >
        <ChevronRight size={17} />
      </button>
    </div>
  );
}

function MyAppointmentsShelf() {
  const { t } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const appointments = useReservedAppointments(isAuthed, 1, 6);
  const items = appointments.data?.items ?? [];

  return (
    <Shelf title={t('home.myAppointments.title')}>
      {!isAuthed ? (
        <div className="gl-yt-banner">
          <div>
            <strong>{t('home.myAppointments.signInTitle')}</strong>
            <span>{t('home.myAppointments.signInSub')}</span>
          </div>
          <button className="gl-secondary-btn" type="button" onClick={() => openLogin()}>
            {t('library.signIn')}
          </button>
        </div>
      ) : appointments.isPending ? (
        <div className="gl-home-appointment-grid" aria-busy="true">
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
            />
          ))}
        </div>
      ) : (
        <div className="gl-creator-empty-soft">{t('home.myAppointments.empty')}</div>
      )}
    </Shelf>
  );
}

export function HistoryPage() {
  const { t } = useTranslation('pages');
  return (
    <LibraryCollectionPage
      storageKey={WATCH_HISTORY_KEY}
      icon={<History size={22} />}
      title={t('library.historyPage.title')}
      subtitle={t('library.historyPage.subtitle')}
      emptyTitle={t('library.historyPage.emptyTitle')}
      emptySub={t('library.historyPage.emptySub')}
      primaryActionLabel={t('library.historyPage.primaryAction')}
      clearLabel={t('library.historyPage.clear')}
    />
  );
}

export function WatchLaterPage() {
  const { t } = useTranslation('pages');
  return (
    <LibraryCollectionPage
      storageKey={WATCH_LATER_KEY}
      icon={<Icons.WatchLater size={22} />}
      title={t('library.watchLaterPage.title')}
      subtitle={t('library.watchLaterPage.subtitle')}
      emptyTitle={t('library.watchLaterPage.emptyTitle')}
      emptySub={t('library.watchLaterPage.emptySub')}
      primaryActionLabel={t('library.watchLaterPage.primaryAction')}
      clearLabel={t('library.watchLaterPage.clear')}
    />
  );
}

export function LikedPage() {
  const { t } = useTranslation('pages');
  return (
    <LibraryCollectionPage
      storageKey={LIKED_STREAMS_KEY}
      icon={<Heart size={22} />}
      title={t('library.likedPage.title')}
      subtitle={t('library.likedPage.subtitle')}
      emptyTitle={t('library.likedPage.emptyTitle')}
      emptySub={t('library.likedPage.emptySub')}
      primaryActionLabel={t('library.likedPage.primaryAction')}
      clearLabel={t('library.likedPage.clear')}
    />
  );
}

type SettingsTab = 'profile' | 'security' | 'preferences' | 'blacklist';

export function SettingsPage() {
  const { t, i18n } = useTranslation('pages');
  const navigate = useNavigate();
  const { theme, toggleTheme } = useThemeStore();
  const { lang, setLang } = useLangStore();
  const user = useAuthStore((s) => s.user);
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const me = useMe();
  const updateProfile = useUpdateProfile();
  const changePassword = useChangePassword();
  const bindGoogle = useBindGoogleAccount();
  const unbindGoogle = useUnbindGoogleAccount();
  const [tab, setTab] = useState<SettingsTab>('profile');
  const blockedUsers = useBlockedUsers(isAuthed && tab === 'blacklist', 1, 100);
  const unblockUser = useUnblockUser();
  const [avatarOpen, setAvatarOpen] = useState(false);
  const [googleUnbindOpen, setGoogleUnbindOpen] = useState(false);
  const [profileUsername, setProfileUsername] = useState('');
  const [profileDisplayName, setProfileDisplayName] = useState('');
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [googleUnbindPassword, setGoogleUnbindPassword] = useState('');
  const currentUser = me.data ?? user;
  const displayName = userDisplayName(currentUser);
  const usernameAvailableAt = parseDate(currentUser?.usernameChangeAvailableAt);
  const usernameLocked = Boolean(usernameAvailableAt && usernameAvailableAt.getTime() > Date.now());

  useEffect(() => {
    setProfileUsername(currentUser?.username ?? '');
    setProfileDisplayName(userDisplayName(currentUser));
  }, [currentUser]);

  const tabs: Array<{ id: SettingsTab; label: string; sub: string; icon: ReactNode }> = [
    {
      id: 'profile',
      label: t('library.settings.nav.profile.label'),
      sub: t('library.settings.nav.profile.sub'),
      icon: <UserRound size={18} />,
    },
    {
      id: 'security',
      label: t('library.settings.nav.security.label'),
      sub: t('library.settings.nav.security.sub'),
      icon: <ShieldCheck size={18} />,
    },
    {
      id: 'preferences',
      label: t('library.settings.nav.preferences.label'),
      sub: t('library.settings.nav.preferences.sub'),
      icon: <Sparkles size={18} />,
    },
    {
      id: 'blacklist',
      label: t('library.settings.nav.blacklist.label', { defaultValue: '黑名单' }),
      sub: t('library.settings.nav.blacklist.sub', {
        defaultValue: '管理不再互动的用户和主播',
      }),
      icon: <Ban size={18} />,
    },
  ];

  const submitProfile = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!isAuthed) {
      openLogin();
      return;
    }
    const username = profileUsername.trim();
    const nextDisplayName = profileDisplayName.trim();
    if (!/^[A-Za-z0-9][A-Za-z0-9_.-]{2,31}$/.test(username)) {
      toast.error(t('library.settings.errors.invalidUsername'));
      return;
    }
    if (!nextDisplayName) {
      toast.error(t('library.settings.errors.displayNameRequired'));
      return;
    }
    updateProfile.mutate(
      { username, displayName: nextDisplayName },
      {
        onSuccess: () => toast.success(t('library.settings.profile.updated')),
        onError: (err) => toast.error(settingsErrorMessage(err, t, i18n.language)),
      },
    );
  };

  const submitPassword = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (!/^(?=.*[A-Za-z])(?=.*\d).{8,}$/.test(newPassword)) {
      toast.error(t('library.settings.errors.weakPassword'));
      return;
    }
    if (newPassword !== confirmPassword) {
      toast.error(t('library.settings.errors.passwordMismatch'));
      return;
    }
    changePassword.mutate(
      { currentPassword, newPassword },
      {
        onSuccess: () => {
          setCurrentPassword('');
          setNewPassword('');
          setConfirmPassword('');
          toast.success(t('library.settings.security.updated'));
        },
        onError: (err) => toast.error(settingsErrorMessage(err, t, i18n.language)),
      },
    );
  };

  const handleGoogleBind = (credential: string) => {
    bindGoogle.mutate(
      { credential },
      {
        onSuccess: () =>
          toast.success(
            t('library.settings.security.googleLinked', {
              defaultValue: 'Google account connected.',
            }),
          ),
        onError: (err) => toast.error(settingsErrorMessage(err, t, i18n.language)),
      },
    );
  };

  const submitGoogleUnbind = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (!googleUnbindPassword) {
      toast.error(
        t('library.settings.errors.googleUnlinkPasswordRequired', {
          defaultValue: 'Enter your current password to unlink Google.',
        }),
      );
      return;
    }
    unbindGoogle.mutate(
      { password: googleUnbindPassword },
      {
        onSuccess: () => {
          setGoogleUnbindPassword('');
          setGoogleUnbindOpen(false);
          toast.success(
            t('library.settings.security.googleUnlinked', {
              defaultValue: 'Google account unlinked.',
            }),
          );
        },
        onError: (err) => toast.error(settingsErrorMessage(err, t, i18n.language)),
      },
    );
  };

  return (
    <div className="gl-page gl-library-page gl-settings-page">
      <header className="gl-settings-hero">
        <div className="gl-settings-hero-icon">
          <Settings size={22} />
        </div>
        <div>
          <h1>{t('library.settings.title')}</h1>
          <p>{t('library.settings.heroSub')}</p>
        </div>
      </header>

      <div className="gl-settings-shell">
        <nav className="gl-settings-rail" aria-label={t('library.settings.sections')}>
          {tabs.map((item) => (
            <SettingsNavButton
              key={item.id}
              active={tab === item.id}
              icon={item.icon}
              label={item.label}
              sub={item.sub}
              onClick={() => setTab(item.id)}
            />
          ))}
        </nav>

        <div className="gl-settings-content">
          {!isAuthed ? (
            <section className="gl-settings-card gl-settings-auth-card">
              <h2>{t('library.settings.auth.title')}</h2>
              <p>{t('library.settings.auth.subtitle')}</p>
              <button
                className="gl-settings-button is-primary"
                type="button"
                onClick={() => openLogin()}
              >
                {t('library.signIn')}
              </button>
            </section>
          ) : tab === 'profile' ? (
            <ProfileSettings
              displayName={displayName}
              username={currentUser?.username ?? ''}
              avatar={currentUser?.avatar}
              profileUsername={profileUsername}
              profileDisplayName={profileDisplayName}
              usernameLocked={usernameLocked}
              usernameAvailableAt={usernameAvailableAt}
              pending={updateProfile.isPending}
              onAvatar={() => setAvatarOpen(true)}
              onChannel={() => navigate(`/channel/${currentUser?.id ?? ''}`)}
              onUsernameChange={setProfileUsername}
              onDisplayNameChange={setProfileDisplayName}
              onSubmit={submitProfile}
            />
          ) : tab === 'security' ? (
            <SecuritySettings
              currentPassword={currentPassword}
              newPassword={newPassword}
              confirmPassword={confirmPassword}
              pending={changePassword.isPending}
              googleLinked={Boolean(currentUser?.googleLinked)}
              googlePending={bindGoogle.isPending}
              googleUnlinkPending={unbindGoogle.isPending}
              usernameAvailableAt={usernameAvailableAt}
              onCurrentPasswordChange={setCurrentPassword}
              onNewPasswordChange={setNewPassword}
              onConfirmPasswordChange={setConfirmPassword}
              onSubmit={submitPassword}
              onGoogleCredential={handleGoogleBind}
              onGoogleUnlinkClick={() => setGoogleUnbindOpen(true)}
              onGoogleUnavailable={() =>
                toast.error(
                  t('library.settings.errors.googleLoadFailed', {
                    defaultValue: 'Could not load Google sign-in.',
                  }),
                )
              }
            />
          ) : tab === 'preferences' ? (
            <PreferencesSettings
              lang={lang}
              theme={theme}
              onThemeToggle={toggleTheme}
              onLangChange={setLang}
              onCoins={() => navigate('/coins')}
              onStudio={() => navigate('/studio')}
            />
          ) : (
            <BlacklistSettings
              pending={blockedUsers.isPending}
              items={blockedUsers.data?.items ?? []}
              unblocking={unblockUser.isPending}
              onUnblock={(userId) =>
                unblockUser.mutate(userId, {
                  onSuccess: () =>
                    toast.success(
                      t('library.settings.blacklist.unblocked', {
                        defaultValue: '已移出黑名单。',
                      }),
                    ),
                })
              }
            />
          )}
        </div>
      </div>
      <AvatarUploadDialog open={avatarOpen} onOpenChange={setAvatarOpen} user={currentUser} />
      <Dialog
        open={googleUnbindOpen}
        onOpenChange={(open) => {
          setGoogleUnbindOpen(open);
          if (!open) setGoogleUnbindPassword('');
        }}
      >
        <DialogContent className="gl-google-unbind-dialog p-0 sm:max-w-[380px]">
          <form className="gl-google-unbind-body" onSubmit={submitGoogleUnbind}>
            <GoogleIcon size="large" />
            <DialogTitle>
              {t('library.settings.security.googleUnlinkTitle', {
                defaultValue: 'Unlink Google account',
              })}
            </DialogTitle>
            <DialogDescription>
              {t('library.settings.security.googleUnlinkSub', {
                defaultValue: 'Enter your current password before unlinking Google.',
              })}
            </DialogDescription>
            <label className="gl-settings-field">
              <span>
                {t('library.settings.security.googleUnlinkPassword', {
                  defaultValue: 'Current password',
                })}
              </span>
              <input
                type="password"
                value={googleUnbindPassword}
                onChange={(event) => setGoogleUnbindPassword(event.target.value)}
                autoComplete="current-password"
                className="gl-settings-input"
              />
            </label>
            <div className="gl-google-unbind-actions">
              <button
                className="gl-settings-button"
                type="button"
                onClick={() => setGoogleUnbindOpen(false)}
              >
                {t('library.settings.security.googleUnlinkCancel', { defaultValue: 'Cancel' })}
              </button>
              <button
                className="gl-settings-button is-danger"
                type="submit"
                disabled={unbindGoogle.isPending}
              >
                {unbindGoogle.isPending
                  ? t('library.settings.security.googleUnlinking', {
                      defaultValue: 'Unlinking...',
                    })
                  : t('library.settings.security.googleUnlinkConfirm', {
                      defaultValue: 'Unlink Google',
                    })}
              </button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function SettingsNavButton({
  active,
  icon,
  label,
  sub,
  onClick,
}: {
  active: boolean;
  icon: ReactNode;
  label: string;
  sub: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className={cn('gl-settings-nav-item', active && 'is-active')}
      onClick={onClick}
    >
      <span className="gl-settings-nav-icon">{icon}</span>
      <span>
        <strong>{label}</strong>
        <small>{sub}</small>
      </span>
    </button>
  );
}

function ProfileSettings({
  displayName,
  username,
  avatar,
  profileUsername,
  profileDisplayName,
  usernameLocked,
  usernameAvailableAt,
  pending,
  onAvatar,
  onChannel,
  onUsernameChange,
  onDisplayNameChange,
  onSubmit,
}: {
  displayName: string;
  username: string;
  avatar?: string;
  profileUsername: string;
  profileDisplayName: string;
  usernameLocked: boolean;
  usernameAvailableAt: Date | null;
  pending: boolean;
  onAvatar: () => void;
  onChannel: () => void;
  onUsernameChange: (value: string) => void;
  onDisplayNameChange: (value: string) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}) {
  const { t, i18n } = useTranslation('pages');
  return (
    <>
      <section className="gl-settings-card gl-settings-profile-card">
        <Avatar name={displayName} src={avatar} size={74} />
        <div className="gl-settings-profile-main">
          <h2>{displayName}</h2>
          <span>{username ? `@${username}` : t('library.settings.profile.noUsername')}</span>
        </div>
        <div className="gl-settings-profile-actions">
          <button className="gl-settings-button" type="button" onClick={onAvatar}>
            <Camera size={15} />
            {t('library.settings.profile.changeAvatar')}
          </button>
          <button className="gl-settings-button" type="button" onClick={onChannel}>
            {t('library.settings.profile.viewChannel')}
          </button>
        </div>
      </section>

      <form className="gl-settings-card gl-settings-form" onSubmit={onSubmit}>
        <div className="gl-settings-card-head">
          <span className="gl-settings-card-icon">
            <PencilLine size={18} />
          </span>
          <div>
            <h2>{t('library.settings.profile.editTitle')}</h2>
            <p>{t('library.settings.profile.editSub')}</p>
          </div>
        </div>
        <div className="gl-settings-form-grid">
          <label className="gl-settings-field">
            <span>
              <AtSign size={14} />
              {t('library.settings.profile.username')}
            </span>
            <input
              value={profileUsername}
              onChange={(event) => onUsernameChange(event.target.value)}
              disabled={usernameLocked}
              maxLength={32}
              autoComplete="username"
              className="gl-settings-input"
            />
            <small>
              {usernameLocked && usernameAvailableAt
                ? t('library.settings.profile.usernameAvailable', {
                    date: formatSettingsDate(usernameAvailableAt, i18n.language),
                  })
                : t('library.settings.profile.usernameHelp')}
            </small>
          </label>
          <label className="gl-settings-field">
            <span>
              <UserRound size={14} />
              {t('library.settings.profile.displayName')}
            </span>
            <input
              value={profileDisplayName}
              onChange={(event) => onDisplayNameChange(event.target.value)}
              maxLength={64}
              autoComplete="name"
              className="gl-settings-input"
            />
            <small>{t('library.settings.profile.displayNameHelp')}</small>
          </label>
        </div>
        <div className="gl-settings-actions">
          <button className="gl-settings-button is-primary" type="submit" disabled={pending}>
            {t('library.settings.profile.save')}
          </button>
        </div>
      </form>
    </>
  );
}

function SecuritySettings({
  currentPassword,
  newPassword,
  confirmPassword,
  pending,
  googleLinked,
  googlePending,
  googleUnlinkPending,
  usernameAvailableAt,
  onCurrentPasswordChange,
  onNewPasswordChange,
  onConfirmPasswordChange,
  onSubmit,
  onGoogleCredential,
  onGoogleUnlinkClick,
  onGoogleUnavailable,
}: {
  currentPassword: string;
  newPassword: string;
  confirmPassword: string;
  pending: boolean;
  googleLinked: boolean;
  googlePending: boolean;
  googleUnlinkPending: boolean;
  usernameAvailableAt: Date | null;
  onCurrentPasswordChange: (value: string) => void;
  onNewPasswordChange: (value: string) => void;
  onConfirmPasswordChange: (value: string) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onGoogleCredential: (credential: string) => void;
  onGoogleUnlinkClick: () => void;
  onGoogleUnavailable: () => void;
}) {
  const { t, i18n } = useTranslation('pages');
  return (
    <>
      <form className="gl-settings-card gl-settings-form" onSubmit={onSubmit}>
        <div className="gl-settings-card-head">
          <span className="gl-settings-card-icon">
            <KeyRound size={18} />
          </span>
          <div>
            <h2>{t('library.settings.security.passwordTitle')}</h2>
            <p>{t('library.settings.security.passwordSub')}</p>
          </div>
        </div>
        <div className="gl-settings-form-grid">
          <label className="gl-settings-field">
            <span>{t('library.settings.security.currentPassword')}</span>
            <input
              type="password"
              value={currentPassword}
              onChange={(event) => onCurrentPasswordChange(event.target.value)}
              autoComplete="current-password"
              className="gl-settings-input"
            />
          </label>
          <label className="gl-settings-field">
            <span>{t('library.settings.security.newPassword')}</span>
            <input
              type="password"
              value={newPassword}
              onChange={(event) => onNewPasswordChange(event.target.value)}
              autoComplete="new-password"
              minLength={8}
              pattern="(?=.*[A-Za-z])(?=.*[0-9]).{8,}"
              title={t('library.settings.security.passwordHint')}
              className="gl-settings-input"
            />
          </label>
          <label className="gl-settings-field">
            <span>{t('library.settings.security.confirmPassword')}</span>
            <input
              type="password"
              value={confirmPassword}
              onChange={(event) => onConfirmPasswordChange(event.target.value)}
              autoComplete="new-password"
              minLength={8}
              pattern="(?=.*[A-Za-z])(?=.*[0-9]).{8,}"
              title={t('library.settings.security.passwordHint')}
              className="gl-settings-input"
            />
          </label>
        </div>
        <div className="gl-settings-actions">
          <button className="gl-settings-button is-primary" type="submit" disabled={pending}>
            {t('library.settings.security.updatePassword')}
          </button>
        </div>
      </form>

      <section className="gl-settings-card gl-settings-form gl-settings-google-card">
        <div className="gl-settings-card-head">
          <GoogleIcon />
          <div>
            <h2>
              {t('library.settings.security.googleTitle', { defaultValue: 'Google account' })}
            </h2>
            <p>
              {t('library.settings.security.googleSub', {
                defaultValue: 'Link Google so you can sign in without a password next time.',
              })}
            </p>
          </div>
        </div>
        {googleLinked ? (
          <div className="gl-settings-provider-state">
            <GoogleIcon size="small" />
            <span>
              {t('library.settings.security.googleLinked', {
                defaultValue: 'Google account connected.',
              })}
            </span>
            <button
              className="gl-settings-button is-danger is-compact"
              type="button"
              disabled={googleUnlinkPending}
              onClick={onGoogleUnlinkClick}
            >
              {t('library.settings.security.googleUnlink', { defaultValue: 'Unlink' })}
            </button>
          </div>
        ) : (
          <GoogleIdentityButton
            className="gl-settings-google-button"
            text="continue_with"
            disabled={googlePending}
            fallbackLabel={
              isGoogleConfigured()
                ? t('library.settings.security.googleBind', { defaultValue: 'Link Google account' })
                : t('library.settings.security.googleUnavailable', {
                    defaultValue: 'Google sign-in is not configured',
                  })
            }
            onCredential={onGoogleCredential}
            onUnavailable={onGoogleUnavailable}
          />
        )}
      </section>

      <section className="gl-settings-card gl-settings-note">
        <span className="gl-settings-card-icon">
          <ShieldCheck size={18} />
        </span>
        <div>
          <h2>{t('library.settings.security.usernameProtectionTitle')}</h2>
          <p>
            {usernameAvailableAt
              ? t('library.settings.security.usernameProtectionWithDate', {
                  date: formatSettingsDate(usernameAvailableAt, i18n.language),
                })
              : t('library.settings.security.usernameProtection')}
          </p>
        </div>
      </section>
    </>
  );
}

function PreferencesSettings({
  lang,
  theme,
  onThemeToggle,
  onLangChange,
  onCoins,
  onStudio,
}: {
  lang: (typeof APP_LANGS)[number];
  theme: string;
  onThemeToggle: () => void;
  onLangChange: (value: (typeof APP_LANGS)[number]) => void;
  onCoins: () => void;
  onStudio: () => void;
}) {
  const { t } = useTranslation('pages');
  return (
    <>
      <section className="gl-settings-card gl-settings-form">
        <div className="gl-settings-card-head">
          <span className="gl-settings-card-icon">
            <Moon size={18} />
          </span>
          <div>
            <h2>{t('library.settings.preferences.displayTitle')}</h2>
            <p>{t('library.settings.preferences.displaySub')}</p>
          </div>
        </div>
        <SettingRow
          title={t('library.settings.darkTheme')}
          sub={
            theme === 'dark'
              ? t('library.settings.preferences.darkActive')
              : t('library.settings.preferences.lightActive')
          }
          checked={theme === 'dark'}
          onChange={onThemeToggle}
        />
        <label className="gl-settings-field">
          <span>
            <Languages size={14} />
            {t('library.settings.language')}
          </span>
          <select
            className="gl-settings-input"
            value={lang}
            onChange={(event) => onLangChange(event.target.value as (typeof APP_LANGS)[number])}
          >
            {APP_LANGS.map((option) => (
              <option key={option} value={option}>
                {t(`library.settings.languageNames.${option}`)}
              </option>
            ))}
          </select>
        </label>
      </section>

      <section className="gl-settings-card gl-settings-shortcuts">
        <div className="gl-settings-card-head">
          <span className="gl-settings-card-icon">
            <Wallet size={18} />
          </span>
          <div>
            <h2>{t('library.settings.preferences.shortcutsTitle')}</h2>
            <p>{t('library.settings.preferences.shortcutsSub')}</p>
          </div>
        </div>
        <div className="gl-settings-shortcut-row">
          <button className="gl-settings-button" type="button" onClick={onCoins}>
            <Wallet size={15} />
            {t('library.settings.preferences.coins')}
          </button>
          <button className="gl-settings-button" type="button" onClick={onStudio}>
            <Video size={15} />
            {t('library.settings.preferences.studio')}
          </button>
        </div>
      </section>
    </>
  );
}

function BlacklistSettings({
  pending,
  items,
  unblocking,
  onUnblock,
}: {
  pending: boolean;
  items: BlockedUser[];
  unblocking: boolean;
  onUnblock: (userId: string) => void;
}) {
  const { t } = useTranslation('pages');
  return (
    <section className="gl-settings-card gl-settings-form gl-blacklist-card">
      <div className="gl-settings-card-head">
        <span className="gl-settings-card-icon">
          <Ban size={18} />
        </span>
        <div>
          <h2>{t('library.settings.blacklist.title', { defaultValue: '黑名单管理' })}</h2>
          <p>
            {t('library.settings.blacklist.sub', {
              defaultValue: '被你拉黑的主播不会出现在私信里；主播拉黑观众后，该观众无法观看和互动。',
            })}
          </p>
        </div>
      </div>
      {pending ? (
        <div className="gl-settings-empty-line">
          {t('loading', { defaultValue: 'Loading...' })}
        </div>
      ) : items.length ? (
        <div className="gl-blacklist-list">
          {items.map((item) => (
            <div className="gl-blacklist-row" key={item.user.id}>
              <Avatar name={item.user.name} src={item.user.avatar} size={42} />
              <span>
                <strong>{item.user.name}</strong>
                <small>{item.role === 'creator' ? '主播' : '用户'}</small>
              </span>
              <button
                className="gl-settings-button"
                type="button"
                disabled={unblocking}
                onClick={() => onUnblock(item.user.id)}
              >
                {t('library.settings.blacklist.unblock', { defaultValue: '移出黑名单' })}
              </button>
            </div>
          ))}
        </div>
      ) : (
        <div className="gl-settings-empty-line">
          {t('library.settings.blacklist.empty', { defaultValue: '暂无黑名单用户。' })}
        </div>
      )}
    </section>
  );
}

function parseDate(value: string | undefined): Date | null {
  if (!value) return null;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

function formatSettingsDate(date: Date, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function settingsErrorMessage(
  err: Error,
  t: ReturnType<typeof useTranslation>['t'],
  locale: string,
): string {
  if (err instanceof AxiosError) {
    const data = err.response?.data as
      | { message?: string; reason?: string; availableAt?: string }
      | undefined;
    if (data?.reason === 'username_taken') return t('library.settings.errors.usernameTaken');
    if (data?.reason === 'username_cooldown') {
      const date = parseDate(data.availableAt);
      return date
        ? t('library.settings.errors.usernameCooldown', {
            date: formatSettingsDate(date, locale),
          })
        : t('library.settings.errors.usernameCooldownUnknown');
    }
    if (data?.reason === 'invalid_current_password') {
      return t('library.settings.errors.invalidCurrentPassword');
    }
    if (data?.reason === 'google_email_exists') {
      return t('library.settings.errors.googleEmailExists', {
        defaultValue: 'This Google email is already used by another account.',
      });
    }
    if (data?.reason === 'google_already_linked') {
      return t('library.settings.errors.googleAlreadyLinked', {
        defaultValue: 'This Google account is already linked to another account.',
      });
    }
    if (data?.reason === 'invalid_google_credential') {
      return t('library.settings.errors.invalidGoogleCredential', {
        defaultValue: 'Google sign-in could not be verified.',
      });
    }
    if (data?.reason === 'google_not_configured') {
      return t('library.settings.errors.googleNotConfigured', {
        defaultValue: 'Google sign-in is not configured yet.',
      });
    }
    if (data?.reason === 'google_not_linked') {
      return t('library.settings.errors.googleNotLinked', {
        defaultValue: 'This account is not linked to Google.',
      });
    }
    if (data?.message) return data.message;
  }
  return err.message || t('library.settings.errors.failed');
}

function LibraryCollectionPage({
  storageKey,
  icon,
  title,
  subtitle,
  emptyTitle,
  emptySub,
  primaryActionLabel,
  clearLabel,
}: {
  storageKey: string;
  icon: ReactNode;
  title: string;
  subtitle: string;
  emptyTitle: string;
  emptySub: string;
  primaryActionLabel: string;
  clearLabel: string;
}) {
  const { t } = useTranslation('pages');
  const navigate = useNavigate();
  const [items, setItems] = useState<LibraryStream[]>(() => readLibrary(storageKey));
  const rooms = useRooms({ size: 100 });

  useEffect(() => {
    if (rooms.data?.items) {
      setItems(syncLibraryWithLiveRooms(storageKey, rooms.data.items));
      return;
    }
    setItems(readLibrary(storageKey));
  }, [rooms.data?.items, storageKey]);

  const hydratedItems = useReplayHydratedStreams(items);

  const handleRemove = (streamId: string) => {
    setItems(removeFromLibrary(storageKey, streamId));
  };

  const handleClear = () => {
    for (const item of items) removeFromLibrary(storageKey, item.id);
    setItems([]);
  };

  const handleOpenFirstRoom = () => {
    if (hydratedItems.length > 0) navigate(`/live/${encodeURIComponent(hydratedItems[0].id)}`);
  };

  return (
    <div className="gl-page gl-library-page">
      <div className="gl-yt-collection">
        <aside className={`gl-yt-collection-side ${collectionThemeClass(storageKey)}`}>
          <div className="gl-yt-collection-art" aria-hidden>
            {icon}
          </div>
          <h1>{title}</h1>
          <p className="gl-muted-line">{subtitle}</p>
          <div className="gl-yt-collection-stats">
            <span>{t('library.collection.liveRoomCount', { count: hydratedItems.length })}</span>
          </div>
          <div className="gl-yt-collection-actions">
            <button
              type="button"
              className="gl-yt-open-room"
              onClick={handleOpenFirstRoom}
              disabled={hydratedItems.length === 0}
            >
              <Radio size={16} /> {primaryActionLabel}
            </button>
            {items.length > 0 && (
              <button type="button" className="gl-secondary-btn" onClick={handleClear}>
                <Trash2 size={14} /> {clearLabel}
              </button>
            )}
          </div>
        </aside>

        <div className="gl-yt-collection-main">
          {hydratedItems.length === 0 ? (
            <EmptyState icon={<Sparkles size={36} />} title={emptyTitle} sub={emptySub} />
          ) : (
            <ol className="gl-yt-vlist">
              {hydratedItems.map((stream, i) => (
                <li className="gl-yt-vrow" key={stream.id}>
                  <span className="gl-yt-vrow-index">{i + 1}</span>
                  <div className="gl-yt-vrow-card">
                    <LiveCard stream={stream} />
                  </div>
                  <button
                    type="button"
                    className="gl-yt-vrow-remove"
                    onClick={() => handleRemove(stream.id)}
                    aria-label={t('library.collection.remove')}
                  >
                    <Trash2 size={14} />
                  </button>
                </li>
              ))}
            </ol>
          )}
        </div>
      </div>

      {items.length === 0 && (
        <section className="gl-library-section gl-yt-explore">
          <div className="gl-section-title-row">
            <h2>{t('library.collection.exploreTitle')}</h2>
          </div>
          <StreamGrid
            isPending={rooms.isPending}
            streams={rooms.data?.items.slice(0, 4) ?? []}
            emptyTitle={t('library.collection.exploreEmptyTitle')}
            emptySub={t('library.collection.exploreEmptySub')}
          />
        </section>
      )}
    </div>
  );
}

function collectionThemeClass(storageKey: string): string {
  if (storageKey === WATCH_HISTORY_KEY) return 'is-history';
  if (storageKey === WATCH_LATER_KEY) return 'is-watch-later';
  if (storageKey === LIKED_STREAMS_KEY) return 'is-liked';
  return 'is-default';
}

function useReplayHydratedStreams<T extends Stream>(items: T[]): T[] {
  const lookupItems = useMemo(() => items.filter((item) => Boolean(item.id)).slice(0, 60), [items]);
  const creatorIds = useMemo(
    () =>
      Array.from(
        new Set(
          lookupItems
            .map((item) => item.ownerId)
            .filter((ownerId): ownerId is string => Boolean(ownerId)),
        ),
      ).slice(0, 60),
    [lookupItems],
  );
  const lookups = useQueries({
    queries: lookupItems.map((item) => ({
      queryKey: ['room', item.id],
      queryFn: async ({ signal }): Promise<Stream> => {
        const { data } = await http.get<Stream>(`/rooms/${encodeURIComponent(item.id)}`, {
          signal,
        });
        return data;
      },
      enabled: Boolean(item.id),
      staleTime: 0,
      refetchOnMount: 'always' as const,
      refetchOnWindowFocus: true,
      retry: false,
    })),
  });
  const profileLookups = useQueries({
    queries: creatorIds.map((creatorId) => ({
      queryKey: ['public-user', creatorId],
      queryFn: async ({ signal }): Promise<User> => {
        const { data } = await http.get<User>(`/users/profile/${encodeURIComponent(creatorId)}`, {
          signal,
        });
        return data;
      },
      enabled: Boolean(creatorId),
      staleTime: 0,
      refetchOnMount: 'always' as const,
      refetchOnWindowFocus: true,
      retry: 1,
    })),
  });

  const replayRooms = new Map<string, Stream>();
  lookups.forEach((query, index) => {
    if (query.data) {
      replayRooms.set(lookupItems[index].id, query.data);
    }
  });
  const creatorProfiles = new Map<string, User>();
  profileLookups.forEach((query, index) => {
    if (query.data) creatorProfiles.set(creatorIds[index], query.data);
  });

  if (lookupItems.length === 0) return items;
  const lookupIds = new Set(lookupItems.map((item) => item.id));
  return items.map((item) => {
    if (!lookupIds.has(item.id)) return hydrateStreamCreatorProfile(item, creatorProfiles);
    const replayRoom = replayRooms.get(item.id);
    if (!replayRoom) {
      const fallback =
        item.status === 'ended'
          ? { ...item, replay: undefined, isLive: false, status: 'ended' }
          : item;
      return hydrateStreamCreatorProfile(fallback, creatorProfiles);
    }
    return hydrateStreamCreatorProfile(
      {
        ...item,
        ...replayRoom,
        replay: replayRoom.replay,
      },
      creatorProfiles,
    );
  });
}

function hydrateStreamCreatorProfile<T extends Stream>(stream: T, profiles: Map<string, User>): T {
  if (!stream.ownerId) return stream;
  const profile = profiles.get(stream.ownerId);
  if (!profile) return stream;
  return {
    ...stream,
    channel: userDisplayName(profile),
    avatar: profile.avatar || stream.avatar,
    verified: profile.verified ?? stream.verified,
  };
}

function Shelf({
  title,
  actionLabel,
  actionTo,
  children,
}: {
  title: string;
  actionLabel?: string;
  actionTo?: string;
  children: ReactNode;
}) {
  return (
    <section className="gl-library-section">
      <div className="gl-section-title-row">
        <h2>{title}</h2>
        {actionLabel && actionTo && (
          <Link className="gl-text-link" to={actionTo}>
            {actionLabel}
          </Link>
        )}
      </div>
      {children}
    </section>
  );
}

function HorizontalShelf({ items, emptyText }: { items: LibraryStream[]; emptyText: string }) {
  if (items.length === 0) {
    return <div className="gl-yt-shelf-empty">{emptyText}</div>;
  }
  return (
    <div className="gl-yt-shelf">
      {items.map((stream) => (
        <div key={stream.id} className="gl-yt-shelf-item">
          <LiveCard stream={stream} />
        </div>
      ))}
    </div>
  );
}

function StreamGrid({
  isPending,
  streams,
  emptyTitle,
  emptySub,
}: {
  isPending: boolean;
  streams: Stream[];
  emptyTitle: string;
  emptySub: string;
}) {
  if (isPending) {
    return (
      <div className="gl-grid" aria-busy="true">
        {Array.from({ length: 4 }).map((_, i) => (
          <LiveCardSkeleton key={i} />
        ))}
      </div>
    );
  }

  if (streams.length === 0) {
    return <EmptyState icon={<Video size={40} />} title={emptyTitle} sub={emptySub} />;
  }

  return (
    <div className="gl-grid">
      {streams.map((stream, i) => (
        <LiveCard key={stream.id} stream={stream} priority={i < 2} />
      ))}
    </div>
  );
}

function AppointmentPager({
  page,
  pageCount,
  total,
  pageSize,
  onPageChange,
}: {
  page: number;
  pageCount: number;
  total: number;
  pageSize: number;
  onPageChange: (page: number) => void;
}) {
  const { t } = useTranslation('pages');
  const start = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const end = Math.min(total, page * pageSize);
  return (
    <div
      className="gl-history-pager"
      aria-label={t('appointments.pagination', { defaultValue: 'Appointment pagination' })}
    >
      <div className="gl-history-pager-count">
        {t('appointments.pageCount', {
          start,
          end,
          total,
          defaultValue: '{{start}}-{{end}} of {{total}}',
        })}
      </div>
      <div className="gl-history-pager-controls">
        <button
          type="button"
          disabled={page <= 1}
          onClick={() => onPageChange(Math.max(1, page - 1))}
        >
          {t('appointments.previous', { defaultValue: 'Prev' })}
        </button>
        <button
          type="button"
          disabled={page >= pageCount}
          onClick={() => onPageChange(Math.min(pageCount, page + 1))}
        >
          {t('appointments.next', { defaultValue: 'Next' })}
        </button>
      </div>
    </div>
  );
}

function EmptyState({ icon, title, sub }: { icon: ReactNode; title: string; sub: string }) {
  return (
    <div className="gl-empty gl-library-empty">
      {icon}
      <div className="gl-empty-title">{title}</div>
      <div className="gl-empty-sub">{sub}</div>
    </div>
  );
}

function QuickChip({
  icon,
  label,
  onClick,
}: {
  icon: ReactNode;
  label: string;
  onClick: () => void;
}) {
  return (
    <button type="button" className="gl-yt-chip" onClick={onClick}>
      {icon}
      <span>{label}</span>
    </button>
  );
}

function SettingRow({
  title,
  sub,
  checked,
  onChange,
}: {
  title: string;
  sub: string;
  checked: boolean;
  onChange: () => void;
}) {
  return (
    <div className="gl-settings-toggle-row">
      <span>
        <strong>{title}</strong>
        <small>{sub}</small>
      </span>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={title}
        className={`gl-yt-switch${checked ? 'is-on' : ''}`}
        onClick={onChange}
      >
        <span className="gl-yt-switch-thumb" />
      </button>
    </div>
  );
}
