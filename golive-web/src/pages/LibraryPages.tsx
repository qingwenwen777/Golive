import { useEffect, useMemo, useState, type FormEvent } from 'react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate } from 'react-router-dom';
import { AxiosError } from 'axios';
import {
  AtSign,
  Bell,
  Bookmark,
  Camera,
  CheckCircle2,
  ChevronRight,
  Clock3,
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
  Video,
  Wallet,
} from 'lucide-react';
import { toast } from 'sonner';
import { useChangePassword, useMe, useUpdateProfile } from '@/api/auth';
import { useFanBadges } from '@/api/gift';
import { useRooms, useSubscriptions, useSubscriptionAppointments } from '@/api/room';
import { Avatar } from '@/components/Avatar';
import { AppointmentViewerCard } from '@/components/AppointmentViewerCard';
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
  const [appointmentPage, setAppointmentPage] = useState(1);
  const subscriptions = useSubscriptions(isAuthed);
  const subscriptionAppointments = useSubscriptionAppointments(isAuthed, appointmentPage, 8);
  const channels = subscriptions.data?.items ?? [];
  const appointmentTotal = subscriptionAppointments.data?.total ?? 0;
  const appointmentPageSize = subscriptionAppointments.data?.size ?? 8;
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

  useEffect(() => {
    setAppointmentPage(1);
  }, [isAuthed]);

  useEffect(() => {
    if (appointmentPage > appointmentPageCount) {
      setAppointmentPage(appointmentPageCount);
    }
  }, [appointmentPage, appointmentPageCount]);

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
          <div>
            <h2>{t('library.subscriptions.appointments.title', { defaultValue: 'Appointments from subscriptions' })}</h2>
            <span>{t('library.subscriptions.appointments.subtitle', { defaultValue: 'Browse upcoming live rooms from creators you follow.' })}</span>
          </div>
        </div>
        {isAuthed && subscriptionAppointments.isPending ? (
          <div className="gl-grid" aria-busy="true">
            {Array.from({ length: 4 }).map((_, i) => (
              <LiveCardSkeleton key={i} />
            ))}
          </div>
        ) : isAuthed && subscriptionAppointments.data?.items.length ? (
          <>
            <div className="gl-appointment-grid">
              {subscriptionAppointments.data.items.map((item) => (
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
              ? t('library.subscriptions.appointments.empty', { defaultValue: 'No upcoming appointments from your subscriptions.' })
              : t('library.subscriptions.appointments.signIn', { defaultValue: 'Sign in to browse appointment schedules.' })}
          </div>
        )}
      </section>

      <section className="gl-library-section">
        <div className="gl-section-title-row">
          <h2>{t('library.latest')}</h2>
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

type SettingsTab = 'profile' | 'security' | 'preferences';

export function SettingsPage() {
  const { t } = useTranslation('pages');
  const navigate = useNavigate();
  const { theme, toggleTheme } = useThemeStore();
  const { lang, setLang } = useLangStore();
  const user = useAuthStore((s) => s.user);
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const me = useMe();
  const updateProfile = useUpdateProfile();
  const changePassword = useChangePassword();
  const [tab, setTab] = useState<SettingsTab>('profile');
  const [avatarOpen, setAvatarOpen] = useState(false);
  const [profileUsername, setProfileUsername] = useState('');
  const [profileDisplayName, setProfileDisplayName] = useState('');
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const currentUser = me.data ?? user;
  const displayName = userDisplayName(currentUser);
  const usernameAvailableAt = parseDate(currentUser?.usernameChangeAvailableAt);
  const usernameLocked = Boolean(
    usernameAvailableAt && usernameAvailableAt.getTime() > Date.now(),
  );

  useEffect(() => {
    setProfileUsername(currentUser?.username ?? '');
    setProfileDisplayName(userDisplayName(currentUser));
  }, [currentUser]);

  const tabs: Array<{ id: SettingsTab; label: string; sub: string; icon: ReactNode }> = [
    { id: 'profile', label: '账号资料', sub: '用户名、昵称与头像', icon: <UserRound size={18} /> },
    { id: 'security', label: '安全', sub: '登录密码与账号保护', icon: <ShieldCheck size={18} /> },
    { id: 'preferences', label: '偏好', sub: '外观、语言与快捷入口', icon: <Sparkles size={18} /> },
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
      toast.error('用户名需为 3-32 位字母、数字、点、横线或下划线。');
      return;
    }
    if (!nextDisplayName) {
      toast.error('昵称不能为空。');
      return;
    }
    updateProfile.mutate(
      { username, displayName: nextDisplayName },
      {
        onSuccess: () => toast.success('账号资料已更新。'),
        onError: (err) => toast.error(settingsErrorMessage(err)),
      },
    );
  };

  const submitPassword = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (newPassword.length < 6) {
      toast.error('新密码至少需要 6 位。');
      return;
    }
    if (newPassword !== confirmPassword) {
      toast.error('两次输入的新密码不一致。');
      return;
    }
    changePassword.mutate(
      { currentPassword, newPassword },
      {
        onSuccess: () => {
          setCurrentPassword('');
          setNewPassword('');
          setConfirmPassword('');
          toast.success('密码已更新。');
        },
        onError: (err) => toast.error(settingsErrorMessage(err)),
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
          <p>管理你的资料、安全设置和常用体验。</p>
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
              <h2>登录后管理账号</h2>
              <p>设置会同步到你的频道、直播预约和个人资料。</p>
              <button className="gl-settings-button is-primary" type="button" onClick={() => openLogin()}>
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
              usernameAvailableAt={usernameAvailableAt}
              onCurrentPasswordChange={setCurrentPassword}
              onNewPasswordChange={setNewPassword}
              onConfirmPasswordChange={setConfirmPassword}
              onSubmit={submitPassword}
            />
          ) : (
            <PreferencesSettings
              lang={lang}
              theme={theme}
              onThemeToggle={toggleTheme}
              onLangChange={setLang}
              onCoins={() => navigate('/coins')}
              onStudio={() => navigate('/studio')}
            />
          )}
        </div>
      </div>
      <AvatarUploadDialog open={avatarOpen} onOpenChange={setAvatarOpen} user={currentUser} />
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
      className={`gl-settings-nav-item${active ? ' is-active' : ''}`}
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
  return (
    <>
      <section className="gl-settings-card gl-settings-profile-card">
        <Avatar name={displayName} src={avatar} size={74} />
        <div className="gl-settings-profile-main">
          <h2>{displayName}</h2>
          <span>{username ? `@${username}` : '尚未设置用户名'}</span>
        </div>
        <div className="gl-settings-profile-actions">
          <button className="gl-settings-button" type="button" onClick={onAvatar}>
            <Camera size={15} />
            更换头像
          </button>
          <button className="gl-settings-button" type="button" onClick={onChannel}>
            查看频道
          </button>
        </div>
      </section>

      <form className="gl-settings-card gl-settings-form" onSubmit={onSubmit}>
        <div className="gl-settings-card-head">
          <span className="gl-settings-card-icon">
            <PencilLine size={18} />
          </span>
          <div>
            <h2>编辑资料</h2>
            <p>用户名用于唯一识别账号，修改后 7 天内不能再次修改。</p>
          </div>
        </div>
        <div className="gl-settings-form-grid">
          <label className="gl-settings-field">
            <span>
              <AtSign size={14} />
              用户名
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
                ? `${formatSettingsDate(usernameAvailableAt)} 后可再次修改`
                : '3-32 位，支持字母、数字、点、横线和下划线'}
            </small>
          </label>
          <label className="gl-settings-field">
            <span>
              <UserRound size={14} />
              昵称
            </span>
            <input
              value={profileDisplayName}
              onChange={(event) => onDisplayNameChange(event.target.value)}
              maxLength={64}
              autoComplete="name"
              className="gl-settings-input"
            />
            <small>展示在频道页、直播间和通知里的名字。</small>
          </label>
        </div>
        <div className="gl-settings-actions">
          <button className="gl-settings-button is-primary" type="submit" disabled={pending}>
            保存资料
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
  usernameAvailableAt,
  onCurrentPasswordChange,
  onNewPasswordChange,
  onConfirmPasswordChange,
  onSubmit,
}: {
  currentPassword: string;
  newPassword: string;
  confirmPassword: string;
  pending: boolean;
  usernameAvailableAt: Date | null;
  onCurrentPasswordChange: (value: string) => void;
  onNewPasswordChange: (value: string) => void;
  onConfirmPasswordChange: (value: string) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}) {
  return (
    <>
      <form className="gl-settings-card gl-settings-form" onSubmit={onSubmit}>
        <div className="gl-settings-card-head">
          <span className="gl-settings-card-icon">
            <KeyRound size={18} />
          </span>
          <div>
            <h2>修改密码</h2>
            <p>需要先输入当前密码，修改成功后下次登录使用新密码。</p>
          </div>
        </div>
        <div className="gl-settings-form-grid">
          <label className="gl-settings-field">
            <span>当前密码</span>
            <input
              type="password"
              value={currentPassword}
              onChange={(event) => onCurrentPasswordChange(event.target.value)}
              autoComplete="current-password"
              className="gl-settings-input"
            />
          </label>
          <label className="gl-settings-field">
            <span>新密码</span>
            <input
              type="password"
              value={newPassword}
              onChange={(event) => onNewPasswordChange(event.target.value)}
              autoComplete="new-password"
              className="gl-settings-input"
            />
          </label>
          <label className="gl-settings-field">
            <span>确认新密码</span>
            <input
              type="password"
              value={confirmPassword}
              onChange={(event) => onConfirmPasswordChange(event.target.value)}
              autoComplete="new-password"
              className="gl-settings-input"
            />
          </label>
        </div>
        <div className="gl-settings-actions">
          <button className="gl-settings-button is-primary" type="submit" disabled={pending}>
            更新密码
          </button>
        </div>
      </form>

      <section className="gl-settings-card gl-settings-note">
        <span className="gl-settings-card-icon">
          <ShieldCheck size={18} />
        </span>
        <div>
          <h2>用户名保护</h2>
          <p>
            用户名全站唯一。成功修改用户名后，系统会开启 7 天冷却期
            {usernameAvailableAt ? `，下一次可修改时间为 ${formatSettingsDate(usernameAvailableAt)}` : '。'}
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
            <h2>显示偏好</h2>
            <p>让界面更贴合你的使用习惯。</p>
          </div>
        </div>
        <SettingRow
          title={t('library.settings.darkTheme')}
          sub={theme === 'dark' ? '当前为深色模式' : '当前为浅色模式'}
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
            <h2>常用入口</h2>
            <p>把高频操作留在设置页，避免来回找菜单。</p>
          </div>
        </div>
        <div className="gl-settings-shortcut-row">
          <button className="gl-settings-button" type="button" onClick={onCoins}>
            <Wallet size={15} />
            金币中心
          </button>
          <button className="gl-settings-button" type="button" onClick={onStudio}>
            <Video size={15} />
            创作者中心
          </button>
        </div>
      </section>
    </>
  );
}

function parseDate(value: string | undefined): Date | null {
  if (!value) return null;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

function formatSettingsDate(date: Date): string {
  return new Intl.DateTimeFormat('zh-CN', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function settingsErrorMessage(err: Error): string {
  if (err instanceof AxiosError) {
    const data = err.response?.data as { message?: string; reason?: string; availableAt?: string } | undefined;
    if (data?.reason === 'username_taken') return '这个用户名已被使用。';
    if (data?.reason === 'username_cooldown') {
      const date = parseDate(data.availableAt);
      return date ? `用户名冷却中，${formatSettingsDate(date)} 后可再次修改。` : '用户名修改仍在冷却期。';
    }
    if (data?.reason === 'invalid_current_password') return '当前密码不正确。';
    if (data?.message) return data.message;
  }
  return err.message || '操作失败，请稍后重试。';
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
    <div className="gl-history-pager" aria-label={t('appointments.pagination', { defaultValue: 'Appointment pagination' })}>
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
