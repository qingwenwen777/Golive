import { useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate } from 'react-router-dom';
import {
  Bell,
  Bookmark,
  Camera,
  CheckCircle2,
  ChevronRight,
  Clock3,
  Crown,
  Heart,
  History,
  Radio,
  Settings,
  Sparkles,
  Trash2,
  Video,
  Wallet,
} from 'lucide-react';
import { useMe } from '@/api/auth';
import { useFanBadges } from '@/api/gift';
import { useRooms, useSubscriptions } from '@/api/room';
import { Avatar } from '@/components/Avatar';
import { LiveCard } from '@/components/LiveCard';
import { LiveCardSkeleton } from '@/components/Skeleton';
import { AvatarUploadDialog } from '@/features/account/AvatarUploadDialog';
import { fanBadgeToneClass } from '@/lib/fanBadgeTone';
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
import { userDisplayName } from '@/types/user';

export function SubscriptionsPage() {
  const { t } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const subscriptions = useSubscriptions(isAuthed);
  const channels = subscriptions.data?.items ?? [];
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
            {channels.map((channel) => (
              <Link
                key={channel.channelId}
                to={`/channel/${encodeURIComponent(channel.key)}`}
                className="gl-yt-channel-chip"
                role="listitem"
              >
                <div className="gl-yt-channel-avatar">
                  <Avatar name={channel.name} src={channel.avatar} size={64} />
                </div>
                <div className="gl-yt-channel-name" title={channel.name}>
                  <span>{channel.name}</span>
                  {channel.verified && <CheckCircle2 size={12} />}
                </div>
                <div className={`gl-yt-channel-status${channel.live ? '' : 'is-offline'}`}>
                  {channel.live ? t('library.status.live') : t('library.status.offline')}
                </div>
              </Link>
            ))}
          </div>
        </section>
      )}

      <section className="gl-library-section">
        <div className="gl-section-title-row">
          <h2>{t('library.latest')}</h2>
          <Link className="gl-text-link" to="/">
            {t('library.browseAll')}
          </Link>
        </div>
        <StreamGrid
          isPending={isAuthed && subscriptions.isPending}
          streams={streams.slice(0, 12)}
          emptyTitle={
            isAuthed
              ? t('library.subscriptions.emptyAuthed')
              : t('library.subscriptions.emptyGuest')
          }
          emptySub={t('library.subscriptions.emptySub')}
        />
      </section>
    </div>
  );
}

export function YouPage() {
  const { t } = useTranslation('pages');
  const navigate = useNavigate();
  const user = useAuthStore((s) => s.user);
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const me = useMe();
  const rooms = useRooms({ size: 100 });
  const fanBadges = useFanBadges(isAuthed, user?.id);
  const liveStreams = rooms.data?.items;
  const history = useMemo(
    () =>
      liveStreams
        ? filterLibraryItemsByLiveRooms(readLibrary(WATCH_HISTORY_KEY), liveStreams)
        : readLibrary(WATCH_HISTORY_KEY),
    [liveStreams],
  );
  const saved = useMemo(
    () =>
      liveStreams
        ? filterLibraryItemsByLiveRooms(readLibrary(WATCH_LATER_KEY), liveStreams)
        : readLibrary(WATCH_LATER_KEY),
    [liveStreams],
  );
  const liked = useMemo(
    () =>
      liveStreams
        ? filterLibraryItemsByLiveRooms(readLibrary(LIKED_STREAMS_KEY), liveStreams)
        : readLibrary(LIKED_STREAMS_KEY),
    [liveStreams],
  );
  const displayName = userDisplayName(user);
  const ownLive = rooms.data?.items.find((stream) => stream.ownerId === user?.id);
  const balance = me.data?.coinBalance ?? user?.coinBalance ?? 0;

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
          <h1>{displayName}</h1>
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
              <Bookmark size={14} /> {t('library.you.watchLaterCount', { count: saved.length })}
            </Link>
            <Link className="gl-yt-chip" to="/liked">
              <Heart size={14} /> {t('library.you.likedCount', { count: liked.length })}
            </Link>
            <Link className="gl-yt-chip" to="/history">
              <History size={14} /> {t('library.you.historyCount', { count: history.length })}
            </Link>
            <Link className="gl-yt-chip" to="/settings">
              <Settings size={14} /> {t('library.you.settings')}
            </Link>
          </div>
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
          badges={fanBadges.data ?? []}
          isPending={fanBadges.isPending && isAuthed}
          isAuthed={isAuthed}
          onLogin={openLogin}
        />
      </Shelf>

      <Shelf title={t('library.you.history')} actionLabel={t('library.you.viewAll')} actionTo="/history">
        <HorizontalShelf items={history.slice(0, 8)} emptyText={t('library.you.noHistory')} />
      </Shelf>

      <Shelf
        title={t('library.you.watchLater')}
        actionLabel={t('library.you.seeAll')}
        actionTo="/watch-later"
      >
        <HorizontalShelf items={saved.slice(0, 8)} emptyText={t('library.you.noWatchLater')} />
      </Shelf>

      <Shelf
        title={t('library.you.likedRooms')}
        actionLabel={t('library.you.seeAll')}
        actionTo="/liked"
      >
        <HorizontalShelf items={liked.slice(0, 8)} emptyText={t('library.you.noLiked')} />
      </Shelf>

      <Shelf title={t('library.you.quickActions')}>
        <div className="gl-yt-quick-row">
          <QuickChip icon={<Radio size={16} />} label={t('library.you.startLive')} onClick={() => navigate('/')} />
          <QuickChip
            icon={<Clock3 size={16} />}
            label={t('library.you.watchLater')}
            onClick={() => navigate('/watch-later')}
          />
          <QuickChip icon={<Heart size={16} />} label={t('library.you.liked')} onClick={() => navigate('/liked')} />
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

  if (!isAuthed) {
    return (
      <button type="button" className="gl-fan-badge-empty" onClick={onLogin}>
        {t('library.fanBadges.signIn')}
      </button>
    );
  }

  if (isPending) {
    return (
      <div className="gl-fan-badge-grid">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="gl-fan-badge-card is-loading" />
        ))}
      </div>
    );
  }

  if (badges.length === 0) {
    return <div className="gl-fan-badge-empty">{t('library.fanBadges.empty')}</div>;
  }

  return (
    <div className="gl-fan-badge-grid">
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
      icon={<Bookmark size={22} />}
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

type SettingsTab = 'account' | 'experience' | 'live';

export function SettingsPage() {
  const { t } = useTranslation('pages');
  const { theme, toggleTheme } = useThemeStore();
  const { lang, setLang } = useLangStore();
  const user = useAuthStore((s) => s.user);
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const me = useMe();
  const [lowLatency, setLowLatency] = useState(true);
  const [chatAssist, setChatAssist] = useState(true);
  const [category, setCategory] = useState('Just Chatting');
  const [tab, setTab] = useState<SettingsTab>('account');
  const [avatarOpen, setAvatarOpen] = useState(false);
  const currentUser = me.data ?? user;

  const tabs: Array<{ id: SettingsTab; label: string }> = [
    { id: 'account', label: t('library.settings.tabs.account') },
    { id: 'experience', label: t('library.settings.tabs.experience') },
    { id: 'live', label: t('library.settings.tabs.live') },
  ];

  return (
    <div className="gl-page gl-library-page">
      <header className="gl-yt-page-head">
        <div className="gl-yt-page-icon">
          <Settings size={22} />
        </div>
        <h1>{t('library.settings.title')}</h1>
      </header>

      <div className="gl-yt-settings">
        <nav className="gl-yt-settings-nav" aria-label={t('library.settings.sections')}>
          {tabs.map((t) => (
            <button
              key={t.id}
              type="button"
              className={`gl-yt-settings-tab${tab === t.id ? 'is-active' : ''}`}
              onClick={() => setTab(t.id)}
            >
              {t.label}
            </button>
          ))}
        </nav>

        <div className="gl-yt-settings-panel">
          {tab === 'account' && (
            <>
              <h2>{t('library.settings.tabs.account')}</h2>
              <div className="gl-account-row">
                <Avatar name={userDisplayName(currentUser)} src={currentUser?.avatar} size={56} />
                <div>
                  <div className="gl-account-name">{userDisplayName(currentUser)}</div>
                  <div className="gl-muted-line">
                    {isAuthed ? currentUser?.username : t('library.settings.notSignedIn')}
                  </div>
                </div>
              </div>
              {isAuthed ? (
                <button
                  className="gl-retry-btn gl-account-avatar-btn"
                  type="button"
                  onClick={() => setAvatarOpen(true)}
                >
                  <Camera size={16} />
                  {t('library.settings.changeAvatar')}
                </button>
              ) : (
                <button className="gl-retry-btn" type="button" onClick={() => openLogin()}>
                  {t('library.signIn')}
                </button>
              )}
            </>
          )}

          {tab === 'experience' && (
            <>
              <h2>{t('library.settings.tabs.experience')}</h2>
              <SettingRow
                title={t('library.settings.darkTheme')}
                sub={t('library.settings.currentTheme', {
                  mode:
                    theme === 'dark'
                      ? t('library.settings.dark')
                      : t('library.settings.light'),
                })}
                checked={theme === 'dark'}
                onChange={toggleTheme}
              />
              <label className="gl-setting-field">
                <span>{t('library.settings.language')}</span>
                <select
                  value={lang}
                  onChange={(e) => setLang(e.target.value as (typeof APP_LANGS)[number])}
                >
                  {APP_LANGS.map((option) => (
                    <option key={option} value={option}>
                      {t(`library.settings.languageNames.${option}`)}
                    </option>
                  ))}
                </select>
              </label>
              <SettingRow
                title={t('library.settings.lowLatency')}
                sub={t('library.settings.lowLatencySub')}
                checked={lowLatency}
                onChange={() => setLowLatency((v) => !v)}
              />
            </>
          )}

          {tab === 'live' && (
            <>
              <h2>{t('library.settings.tabs.live')}</h2>
              <label className="gl-setting-field">
                <span>{t('library.settings.defaultCategory')}</span>
                <select value={category} onChange={(e) => setCategory(e.target.value)}>
                  {SETTINGS_CATEGORY_OPTIONS.map((option) => (
                    <option key={option} value={option}>
                      {t(`library.settings.categories.${categoryKey(option)}`)}
                    </option>
                  ))}
                </select>
              </label>
              <SettingRow
                title={t('library.settings.chatAssist')}
                sub={t('library.settings.chatAssistSub')}
                checked={chatAssist}
                onChange={() => setChatAssist((v) => !v)}
              />
            </>
          )}
        </div>
      </div>
      <AvatarUploadDialog open={avatarOpen} onOpenChange={setAvatarOpen} user={currentUser} />
    </div>
  );
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

  const handleRemove = (streamId: string) => {
    setItems(removeFromLibrary(storageKey, streamId));
  };

  const handleClear = () => {
    for (const item of items) removeFromLibrary(storageKey, item.id);
    setItems([]);
  };

  const handleOpenFirstRoom = () => {
    if (items.length > 0) navigate(`/live/${items[0].id}`);
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
            <span>
              {t('library.collection.liveRoomCount', { count: items.length })}
            </span>
          </div>
          <div className="gl-yt-collection-actions">
            <button
              type="button"
              className="gl-yt-open-room"
              onClick={handleOpenFirstRoom}
              disabled={items.length === 0}
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
          {items.length === 0 ? (
            <EmptyState icon={<Sparkles size={36} />} title={emptyTitle} sub={emptySub} />
          ) : (
            <ol className="gl-yt-vlist">
              {items.map((stream, i) => (
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

function EmptyState({ icon, title, sub }: { icon: ReactNode; title: string; sub: string }) {
  return (
    <div className="gl-empty gl-library-empty">
      {icon}
      <div className="gl-empty-title">{title}</div>
      <div className="gl-empty-sub">{sub}</div>
    </div>
  );
}

const SETTINGS_CATEGORY_OPTIONS = ['Just Chatting', 'Gaming', 'Music', 'VTuber', 'News'] as const;

function categoryKey(category: (typeof SETTINGS_CATEGORY_OPTIONS)[number]): string {
  return category.toLowerCase().replace(/\s+/g, '');
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
    <div className="gl-setting-row">
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
