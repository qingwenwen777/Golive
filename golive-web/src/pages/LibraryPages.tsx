import { useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { toast } from 'sonner';
import {
  Bell,
  Bookmark,
  Camera,
  CheckCircle2,
  ChevronRight,
  Clock3,
  Heart,
  History,
  Play,
  Radio,
  Settings,
  Sparkles,
  Trash2,
  Video,
  Wallet,
} from 'lucide-react';
import { useMe, useTopupCoins } from '@/api/auth';
import { useRooms, useSubscriptions } from '@/api/room';
import { Avatar } from '@/components/Avatar';
import { LiveCard } from '@/components/LiveCard';
import { LiveCardSkeleton } from '@/components/Skeleton';
import { AvatarUploadDialog } from '@/features/account/AvatarUploadDialog';
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
import { useLangStore } from '@/stores/useLangStore';
import { useThemeStore } from '@/stores/useThemeStore';
import type { Stream } from '@/types/stream';
import { userDisplayName } from '@/types/user';

export function SubscriptionsPage() {
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
            <strong>Sign in to see your subscriptions</strong>
            <span>Follow creators to keep their live rooms in this feed.</span>
          </div>
          <button className="gl-secondary-btn" type="button" onClick={() => openLogin()}>
            Sign in
          </button>
        </div>
      )}

      <header className="gl-yt-page-head">
        <div className="gl-yt-page-icon">
          <Bell size={22} />
        </div>
        <h1>Subscriptions</h1>
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
                <div className={`gl-yt-channel-status${channel.live ? '' : ' is-offline'}`}>
                  {channel.live ? 'LIVE' : 'Offline'}
                </div>
              </Link>
            ))}
          </div>
        </section>
      )}

      <section className="gl-library-section">
        <div className="gl-section-title-row">
          <h2>Latest</h2>
          <Link className="gl-text-link" to="/">
            Browse all
          </Link>
        </div>
        <StreamGrid
          isPending={isAuthed && subscriptions.isPending}
          streams={streams.slice(0, 12)}
          emptyTitle={isAuthed ? 'No subscribed creators yet' : 'No subscriptions to show'}
          emptySub="Explore the live directory and follow rooms from the player page."
        />
      </section>
    </div>
  );
}

export function YouPage() {
  const navigate = useNavigate();
  const user = useAuthStore((s) => s.user);
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const me = useMe();
  const topup = useTopupCoins();
  const rooms = useRooms({ size: 100 });
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

  const handleAddCoins = () => {
    if (!user) {
      openLogin();
      return;
    }
    topup.mutate(
      { amount: 1000 },
      {
        onSuccess: () => toast.success('Added 1,000 coins.'),
        onError: (err) => toast.error(err.message || 'Top-up failed.'),
      },
    );
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
                View channel <ChevronRight size={14} />
              </Link>
            ) : (
              <button className="gl-yt-you-link" type="button" onClick={() => openLogin()}>
                Sign in <ChevronRight size={14} />
              </button>
            )}
          </div>
          <div className="gl-yt-you-chips">
            <button
              type="button"
              className="gl-yt-coin-chip"
              onClick={handleAddCoins}
              disabled={topup.isPending}
              title="Add 1,000 coins"
            >
              <Wallet size={14} />
              <span>{balance.toLocaleString()}</span>
              <span className="gl-yt-coin-chip-add">{topup.isPending ? '…' : '+1,000'}</span>
            </button>
            <Link className="gl-yt-chip" to="/watch-later">
              <Bookmark size={14} /> Watch later · {saved.length}
            </Link>
            <Link className="gl-yt-chip" to="/liked">
              <Heart size={14} /> Liked · {liked.length}
            </Link>
            <Link className="gl-yt-chip" to="/history">
              <History size={14} /> History · {history.length}
            </Link>
            <Link className="gl-yt-chip" to="/settings">
              <Settings size={14} /> Settings
            </Link>
          </div>
        </div>
      </section>

      {ownLive && (
        <Shelf title="Your live room" actionLabel="Open studio" actionTo={`/live/${ownLive.id}`}>
          <div className="gl-feature-live">
            <LiveCard stream={ownLive} priority />
          </div>
        </Shelf>
      )}

      <Shelf title="History" actionLabel="View all" actionTo="/history">
        <HorizontalShelf items={history.slice(0, 8)} emptyText="No watch history yet." />
      </Shelf>

      <Shelf title="Watch later" actionLabel="See all" actionTo="/watch-later">
        <HorizontalShelf items={saved.slice(0, 8)} emptyText="Save a live room to see it here." />
      </Shelf>

      <Shelf title="Liked rooms" actionLabel="See all" actionTo="/liked">
        <HorizontalShelf items={liked.slice(0, 8)} emptyText="Liked rooms appear here." />
      </Shelf>

      <Shelf title="Quick actions">
        <div className="gl-yt-quick-row">
          <QuickChip icon={<Radio size={16} />} label="Start live" onClick={() => navigate('/')} />
          <QuickChip
            icon={<Clock3 size={16} />}
            label="Watch later"
            onClick={() => navigate('/watch-later')}
          />
          <QuickChip icon={<Heart size={16} />} label="Liked" onClick={() => navigate('/liked')} />
          <QuickChip
            icon={<Settings size={16} />}
            label="Settings"
            onClick={() => navigate('/settings')}
          />
        </div>
      </Shelf>
    </div>
  );
}

export function HistoryPage() {
  return (
    <LibraryCollectionPage
      storageKey={WATCH_HISTORY_KEY}
      icon={<History size={22} />}
      title="History"
      subtitle="Live rooms you opened on this device."
      emptyTitle="No watch history yet"
      emptySub="When you visit a live room, GoLive adds it here automatically."
      clearLabel="Clear history"
    />
  );
}

export function WatchLaterPage() {
  return (
    <LibraryCollectionPage
      storageKey={WATCH_LATER_KEY}
      icon={<Bookmark size={22} />}
      title="Watch later"
      subtitle="Saved streams and rooms you want to revisit."
      emptyTitle="Nothing saved yet"
      emptySub="Use Save on a live room to build this list."
      clearLabel="Clear all"
    />
  );
}

export function LikedPage() {
  return (
    <LibraryCollectionPage
      storageKey={LIKED_STREAMS_KEY}
      icon={<Heart size={22} />}
      title="Liked live rooms"
      subtitle="Streams you liked are collected here for quick return trips."
      emptyTitle="No liked rooms yet"
      emptySub="Tap Like on a live room to collect it here."
      clearLabel="Clear all"
    />
  );
}

type SettingsTab = 'account' | 'experience' | 'live';

export function SettingsPage() {
  const { theme, toggleTheme } = useThemeStore();
  const { lang, toggleLang } = useLangStore();
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
    { id: 'account', label: 'Account' },
    { id: 'experience', label: 'Experience' },
    { id: 'live', label: 'Live defaults' },
  ];

  return (
    <div className="gl-page gl-library-page">
      <header className="gl-yt-page-head">
        <div className="gl-yt-page-icon">
          <Settings size={22} />
        </div>
        <h1>Settings</h1>
      </header>

      <div className="gl-yt-settings">
        <nav className="gl-yt-settings-nav" aria-label="Settings sections">
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
              <h2>Account</h2>
              <div className="gl-account-row">
                <Avatar
                  name={userDisplayName(currentUser)}
                  src={currentUser?.avatar}
                  size={56}
                />
                <div>
                  <div className="gl-account-name">{userDisplayName(currentUser)}</div>
                  <div className="gl-muted-line">
                    {isAuthed ? currentUser?.username : 'Not signed in'}
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
                  Change avatar
                </button>
              ) : (
                <button className="gl-retry-btn" type="button" onClick={() => openLogin()}>
                  Sign in
                </button>
              )}
            </>
          )}

          {tab === 'experience' && (
            <>
              <h2>Experience</h2>
              <SettingRow
                title="Dark theme"
                sub={`Current: ${theme === 'dark' ? 'Dark' : 'Light'}`}
                checked={theme === 'dark'}
                onChange={toggleTheme}
              />
              <SettingRow
                title="Language"
                sub={`Current: ${lang === 'en' ? 'English' : 'Japanese'}`}
                checked={lang === 'ja'}
                onChange={toggleLang}
              />
              <SettingRow
                title="Low latency playback"
                sub="Prefer a tighter live delay when the stream supports it."
                checked={lowLatency}
                onChange={() => setLowLatency((v) => !v)}
              />
            </>
          )}

          {tab === 'live' && (
            <>
              <h2>Live defaults</h2>
              <label className="gl-setting-field">
                <span>Default category</span>
                <select value={category} onChange={(e) => setCategory(e.target.value)}>
                  <option>Just Chatting</option>
                  <option>Gaming</option>
                  <option>Music</option>
                  <option>VTuber</option>
                  <option>News</option>
                </select>
              </label>
              <SettingRow
                title="Chat safety assist"
                sub="Highlight fast-moving chat and potential moderation spikes."
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
  clearLabel,
}: {
  storageKey: string;
  icon: ReactNode;
  title: string;
  subtitle: string;
  emptyTitle: string;
  emptySub: string;
  clearLabel: string;
}) {
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

  const handlePlayAll = () => {
    if (items.length > 0) navigate(`/live/${items[0].id}`);
  };

  return (
    <div className="gl-page gl-library-page">
      <div className="gl-yt-collection">
        <aside className="gl-yt-collection-side">
          <div className="gl-yt-collection-art" aria-hidden>
            {icon}
          </div>
          <h1>{title}</h1>
          <p className="gl-muted-line">{subtitle}</p>
          <div className="gl-yt-collection-stats">
            <span>
              {items.length} {items.length === 1 ? 'video' : 'videos'}
            </span>
          </div>
          <div className="gl-yt-collection-actions">
            <button
              type="button"
              className="gl-yt-play-all"
              onClick={handlePlayAll}
              disabled={items.length === 0}
            >
              <Play size={16} fill="currentColor" /> Play all
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
                    aria-label="Remove"
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
            <h2>Live rooms to explore</h2>
          </div>
          <StreamGrid
            isPending={rooms.isPending}
            streams={rooms.data?.items.slice(0, 4) ?? []}
            emptyTitle="No live rooms available"
            emptySub="Try again when creators start broadcasting."
          />
        </section>
      )}
    </div>
  );
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
