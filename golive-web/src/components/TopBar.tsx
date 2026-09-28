import { useEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent } from 'react';
import { flushSync } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import { ArrowLeft, Bell, Coins, Globe, Plus, User as UserIcon, X } from 'lucide-react';
import { logout as doLogout, useMe } from '@/api/auth';
import {
  useMarkAllNotificationsRead,
  useMarkNotificationRead,
  useNotifications,
  type NotificationItem,
} from '@/api/room';
import {
  useDirectThreads,
  useJoinedFanGroups,
  type DirectThread,
  type FanGroup,
} from '@/api/messages';
import { useSearchSuggestions } from '@/api/search';
import { Avatar } from '@/components/Avatar';
import { VerifiedBadge } from '@/components/VerifiedBadge';
import { AvatarUploadDialog } from '@/features/account/AvatarUploadDialog';
import { useActiveCreatorLiveId } from '@/features/creator/useActiveCreatorLiveId';
import { Icons } from '@/components/Icons';
import { GoLiveLogo } from '@/components/Logo';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import { useLangStore } from '@/stores/useLangStore';
import { useThemeStore } from '@/stores/useThemeStore';
import type { AppLang } from '@/i18n';
import { formatNumber, formatRelativeTime } from '@/lib/format';
import { isUuidLike, personName } from '@/types/user';

export interface TopBarProps {
  onMenuClick: () => void;
  onLogoClick?: () => void;
}

export function TopBar({ onMenuClick, onLogoClick }: TopBarProps) {
  const { t } = useTranslation('common');
  const navigate = useNavigate();
  const location = useLocation();
  const { theme, toggleTheme } = useThemeStore();
  const { lang, setLang } = useLangStore();
  const isDark = theme === 'dark';
  const isAuthed = useIsAuthed();
  const user = useAuthStore((s) => s.user);
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const me = useMe();
  const [avatarOpen, setAvatarOpen] = useState(false);
  const [search, setSearch] = useState(() => new URLSearchParams(location.search).get('q') ?? '');
  const [suggestionsOpen, setSuggestionsOpen] = useState(false);
  const [activeSuggestion, setActiveSuggestion] = useState(-1);
  // Phones have no room for the search pill; a search button opens it over
  // the top bar instead (the CSS only honours this below 768px).
  const [mobileSearchOpen, setMobileSearchOpen] = useState(false);
  const searchRef = useRef<HTMLFormElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const openSearchRef = useRef<HTMLButtonElement | null>(null);
  const currentUser = isAuthed ? (me.data ?? user) : null;
  const balance = isAuthed ? (me.data?.coinBalance ?? user?.coinBalance ?? 0) : 0;
  const isBanned = isAuthed && Boolean(currentUser?.banned);
  const activeLiveId = useActiveCreatorLiveId(isBanned ? undefined : currentUser?.id);
  const trimmedSearch = search.trim();
  const suggestions = useSearchSuggestions(trimmedSearch, suggestionsOpen);
  const suggestionItems = suggestions.data?.items ?? [];

  useEffect(() => {
    setSearch(new URLSearchParams(location.search).get('q') ?? '');
  }, [location.search]);

  useEffect(() => {
    const handlePointerDown = (event: MouseEvent) => {
      const target = event.target;
      if (target instanceof Node && searchRef.current?.contains(target)) return;
      setSuggestionsOpen(false);
      setActiveSuggestion(-1);
    };
    document.addEventListener('mousedown', handlePointerDown);
    return () => document.removeEventListener('mousedown', handlePointerDown);
  }, []);

  useEffect(() => {
    setActiveSuggestion(-1);
  }, [trimmedSearch]);

  useEffect(() => {
    setMobileSearchOpen(false);
  }, [location.pathname]);

  // Both run inside the tap handler: iOS only raises the keyboard for a
  // focus() made during the tap, so the pill must be shown synchronously.
  const openMobileSearch = () => {
    flushSync(() => setMobileSearchOpen(true));
    inputRef.current?.focus();
  };

  const closeMobileSearch = () => {
    setSuggestionsOpen(false);
    setActiveSuggestion(-1);
    flushSync(() => setMobileSearchOpen(false));
    openSearchRef.current?.focus();
  };

  const commitSearch = (value = search) => {
    const q = value.trim();
    setSuggestionsOpen(false);
    setActiveSuggestion(-1);
    if (mobileSearchOpen) {
      setMobileSearchOpen(false);
      inputRef.current?.blur();
    }
    navigate(q ? `/search?q=${encodeURIComponent(q)}` : '/');
  };

  const handleLogoClick = () => {
    if (onLogoClick) {
      onLogoClick();
      return;
    }
    navigate('/');
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  const handleSearchSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    commitSearch();
  };

  const handleSearchKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'ArrowDown' && suggestionItems.length > 0) {
      event.preventDefault();
      setSuggestionsOpen(true);
      setActiveSuggestion((current) => (current + 1) % suggestionItems.length);
      return;
    }
    if (event.key === 'ArrowUp' && suggestionItems.length > 0) {
      event.preventDefault();
      setSuggestionsOpen(true);
      setActiveSuggestion((current) => (current <= 0 ? suggestionItems.length - 1 : current - 1));
      return;
    }
    if (event.key === 'Enter' && activeSuggestion >= 0 && suggestionItems[activeSuggestion]) {
      event.preventDefault();
      commitSearch(suggestionItems[activeSuggestion].value);
    }
  };

  // Escape closes the suggestions first, then the phone search bar.
  const handleSearchFormKeyDown = (event: KeyboardEvent<HTMLFormElement>) => {
    if (event.key !== 'Escape') return;
    if (suggestionsOpen && trimmedSearch) {
      setSuggestionsOpen(false);
      setActiveSuggestion(-1);
    } else if (mobileSearchOpen) {
      closeMobileSearch();
    }
  };

  return (
    <>
      <header className={mobileSearchOpen ? 'gl-topbar is-searching' : 'gl-topbar'}>
        <div className="gl-topbar-left">
          <button
            type="button"
            className="gl-icon-btn gl-menu-btn"
            onClick={onMenuClick}
            aria-label={t('menu')}
          >
            <Icons.Menu size={24} />
          </button>
          <button type="button" className="gl-logo" onClick={handleLogoClick} aria-label="GoLive">
            <GoLiveLogo height={28} />
            <span className="gl-logo-country">{t(`lang.short.${lang}`)}</span>
          </button>
        </div>

        <form
          ref={searchRef}
          className="gl-topbar-search"
          onSubmit={handleSearchSubmit}
          onKeyDown={handleSearchFormKeyDown}
          role="search"
        >
          <button
            type="button"
            className="gl-icon-btn gl-search-back-btn"
            aria-label={t('searchClose', { defaultValue: 'Close search' })}
            onClick={closeMobileSearch}
          >
            <ArrowLeft size={22} />
          </button>
          <div className="gl-search-pill">
            <input
              ref={inputRef}
              className="gl-search-input"
              enterKeyHint="search"
              value={search}
              onChange={(event) => {
                setSearch(event.target.value);
                setSuggestionsOpen(true);
              }}
              onFocus={() => setSuggestionsOpen(true)}
              onKeyDown={handleSearchKeyDown}
              placeholder={t('searchPlaceholder')}
              aria-label={t('search')}
              aria-autocomplete="list"
              aria-expanded={suggestionsOpen && trimmedSearch.length > 0}
              aria-controls="gl-search-suggestions"
            />
            {search && (
              <button
                type="button"
                className="gl-search-clear"
                aria-label={t('clear', { defaultValue: 'Clear' })}
                onClick={() => {
                  setSearch('');
                  setSuggestionsOpen(false);
                  setActiveSuggestion(-1);
                  inputRef.current?.focus();
                }}
              >
                <X size={22} />
              </button>
            )}
            <button type="submit" className="gl-search-btn" aria-label={t('search')}>
              <Icons.Search size={22} />
            </button>
          </div>
          <button
            ref={openSearchRef}
            type="button"
            className="gl-icon-btn gl-search-open-btn"
            aria-label={t('search')}
            onClick={openMobileSearch}
          >
            <Icons.Search size={22} />
          </button>
          {suggestionsOpen && trimmedSearch && (
            <div id="gl-search-suggestions" className="gl-search-suggest-popover" role="listbox">
              {suggestions.isPending && suggestionItems.length === 0 ? (
                <div className="gl-search-suggest-state">
                  {t('loading', { defaultValue: 'Loading...' })}
                </div>
              ) : suggestionItems.length > 0 ? (
                suggestionItems.map((item, index) => (
                  <button
                    key={`${item.type}:${item.value}:${index}`}
                    type="button"
                    role="option"
                    aria-selected={activeSuggestion === index}
                    className={activeSuggestion === index ? 'is-active' : undefined}
                    onMouseEnter={() => setActiveSuggestion(index)}
                    onMouseDown={(event) => {
                      event.preventDefault();
                      commitSearch(item.value);
                    }}
                  >
                    <Icons.Search size={22} />
                    <span>
                      <strong>{item.value}</strong>
                      {item.label && <small>{item.label}</small>}
                    </span>
                    <em>{suggestionTypeLabel(item.type, t)}</em>
                  </button>
                ))
              ) : (
                <button
                  type="button"
                  role="option"
                  onMouseDown={(event) => {
                    event.preventDefault();
                    commitSearch(trimmedSearch);
                  }}
                >
                  <Icons.Search size={22} />
                  <span>
                    <strong>{trimmedSearch}</strong>
                  </span>
                </button>
              )}
            </div>
          )}
        </form>

        <div className="gl-topbar-right">
          {/* Phones show only the globe, and only when signed out: the account
              menu repeats the languages. */}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                className={isAuthed ? 'gl-lang-toggle is-in-menu' : 'gl-lang-toggle'}
                aria-label={t('lang.toggle')}
              >
                <Globe size={22} className="gl-lang-toggle-icon" />
                <span className="gl-lang-toggle-label">{t(`lang.${lang}`)}</span>
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-36">
              {LANG_OPTIONS.map((option) => (
                <DropdownMenuItem
                  key={option}
                  onClick={() => setLang(option)}
                  className={option === lang ? 'font-semibold' : undefined}
                >
                  {t(`lang.${option}`)}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
          {/* The account menu repeats this, so the narrowest phones drop it here. */}
          <button
            type="button"
            className={
              isAuthed ? 'gl-icon-btn gl-theme-btn is-in-menu' : 'gl-icon-btn gl-theme-btn'
            }
            onClick={toggleTheme}
            aria-label={t('theme.toggle')}
          >
            {isDark ? <Icons.Sun size={22} /> : <Icons.Moon size={22} />}
          </button>

          {isAuthed ? (
            <>
              {!isBanned && (
                <>
                  <NotificationBell />
                  <button
                    type="button"
                    className="gl-create-btn"
                    onClick={() =>
                      navigate(activeLiveId ? `/studio/live/${activeLiveId}` : '/studio/prepare')
                    }
                  >
                    {activeLiveId ? <Icons.Live size={22} /> : <Icons.Plus size={22} />}
                    <span className="gl-create-label">
                      {activeLiveId ? t('nav.liveNow') : t('goLive', { defaultValue: t('create') })}
                    </span>
                  </button>
                </>
              )}
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button
                    type="button"
                    className="gl-account-menu-trigger"
                    aria-label={t('account.menu')}
                  >
                    <Avatar
                      name={currentUser?.username ?? t('account.you')}
                      src={currentUser?.avatar}
                      size={32}
                    />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-52">
                  <DropdownMenuLabel className="truncate">
                    {currentUser?.username ?? t('account.you')}
                  </DropdownMenuLabel>
                  {isBanned ? (
                    <DropdownMenuItem onClick={() => navigate('/account-banned')}>
                      {t('account.bannedAppeal', { defaultValue: 'Restriction appeal' })}
                    </DropdownMenuItem>
                  ) : (
                    <>
                      <DropdownMenuItem className="flex items-center gap-2">
                        <Coins size={14} />
                        <span>{t('account.coins', { amount: formatNumber(balance) })}</span>
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        className="flex items-center gap-2"
                        onClick={() => navigate('/coins?focus=recharge')}
                      >
                        <Plus size={14} />
                        <span>{t('account.rechargeCoins')}</span>
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      {(currentUser?.role === 'admin' || currentUser?.role === 'moderator') && (
                        <DropdownMenuItem
                          onClick={() =>
                            navigate(
                              currentUser?.role === 'moderator'
                                ? '/admin/content'
                                : '/admin/dashboard',
                            )
                          }
                        >
                          {t('account.adminDashboard')}
                        </DropdownMenuItem>
                      )}
                      <DropdownMenuItem onClick={() => navigate(`/channel/${user?.id ?? ''}`)}>
                        {t('account.yourChannel')}
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => setAvatarOpen(true)}>
                        {t('account.changeAvatar')}
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => navigate('/settings')}>
                        {t('account.settings')}
                      </DropdownMenuItem>
                    </>
                  )}
                  <DropdownMenuItem onClick={toggleTheme}>
                    {t('account.appearance', {
                      mode: isDark ? t('account.dark') : t('account.light'),
                    })}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  {LANG_OPTIONS.map((option) => (
                    <DropdownMenuItem
                      key={option}
                      onClick={() => setLang(option)}
                      className={option === lang ? 'font-semibold' : undefined}
                    >
                      {t('account.language', { language: t(`lang.${option}`) })}
                    </DropdownMenuItem>
                  ))}
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    onClick={() => {
                      void doLogout();
                    }}
                  >
                    {t('account.signOut')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </>
          ) : (
            <button
              type="button"
              onClick={() => openLogin()}
              className="gl-signin-btn flex items-center gap-2 rounded-full border border-accent/60 px-3 py-1.5 text-sm font-semibold text-accent hover:bg-accent/10"
              aria-label={t('account.signIn')}
            >
              <UserIcon size={18} />
              <span>{t('account.signIn')}</span>
            </button>
          )}
        </div>
      </header>
      <AvatarUploadDialog open={avatarOpen} onOpenChange={setAvatarOpen} user={currentUser} />
    </>
  );
}

const LANG_OPTIONS: AppLang[] = ['zh', 'ja', 'en'];

type NotificationMenuNotice = {
  id: string;
  title: string;
  body?: string;
  link: string;
  actorName: string;
  actorLabel: string;
  actorAvatar?: string;
  actorVerified?: boolean;
  createdAt: string;
  count: number;
  unread: boolean;
  notification?: NotificationItem;
};

function suggestionTypeLabel(type: string, t: ReturnType<typeof useTranslation>['t']): string {
  switch (type) {
    case 'creator':
      return t('searchSuggestionTypes.creator', { defaultValue: 'Creator' });
    case 'live':
      return t('searchSuggestionTypes.live', { defaultValue: 'Live' });
    case 'replay':
      return t('searchSuggestionTypes.replay', { defaultValue: 'Replay' });
    case 'appointment':
      return t('searchSuggestionTypes.appointment', { defaultValue: 'Upcoming' });
    case 'post':
      return t('searchSuggestionTypes.post', { defaultValue: 'Post' });
    default:
      return '';
  }
}

function NotificationBell() {
  const { t, i18n } = useTranslation('common');
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const notifications = useNotifications(true, 1, 8);
  const directThreads = useDirectThreads(true, 1, 50);
  const fanGroups = useJoinedFanGroups(true);
  const markRead = useMarkNotificationRead();
  const markAllRead = useMarkAllNotificationsRead();
  const items = (notifications.data?.items ?? []).filter(
    (item) => !isChatNotificationType(item.type),
  );
  const unread = notifications.data?.unread ?? 0;
  const messageNotices = useMemo(
    () => [
      ...(directThreads.data?.items ?? [])
        .filter((thread) => thread.unread > 0)
        .map((thread) => directThreadNotice(thread, t)),
      ...(fanGroups.data?.items ?? [])
        .filter((group) => group.unread > 0)
        .map((group) => fanGroupNotice(group, t)),
    ],
    [directThreads.data?.items, fanGroups.data?.items, t],
  );
  const menuItems = useMemo(
    () =>
      [...messageNotices, ...items.map((item) => notificationMenuNotice(item, t))]
        .sort((a, b) => notificationNoticeTime(b.createdAt) - notificationNoticeTime(a.createdAt))
        .slice(0, 8),
    [items, messageNotices, t],
  );
  const messageUnread = messageNotices.reduce((sum, item) => sum + item.count, 0);
  const totalUnread = unread + messageUnread;
  const loading =
    notifications.isPending ||
    (directThreads.isPending && !directThreads.data) ||
    (fanGroups.isPending && !fanGroups.data);

  const openNotification = (item: NotificationMenuNotice) => {
    if (item.notification && !item.notification.readAt) markRead.mutate(item.notification.id);
    setOpen(false);
    navigate(item.link);
  };

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className="gl-icon-btn gl-notification-btn"
          aria-label={t('notifications')}
          title={t('notifications')}
        >
          <Bell size={22} />
          {totalUnread > 0 && <span className="gl-bell-dot" />}
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="gl-notification-menu w-80">
        <DropdownMenuLabel className="gl-notification-head">
          <span>{t('notifications')}</span>
          <div>
            {unread > 0 && (
              <button
                type="button"
                disabled={markAllRead.isPending}
                onClick={(event) => {
                  event.preventDefault();
                  event.stopPropagation();
                  markAllRead.mutate();
                }}
              >
                {t('notificationsReadAll', { defaultValue: 'Read all' })}
              </button>
            )}
            <button
              type="button"
              onClick={(event) => {
                event.preventDefault();
                event.stopPropagation();
                setOpen(false);
                navigate(
                  messageUnread > 0 && unread === 0 ? '/messages/private' : '/messages/system',
                );
              }}
            >
              {t('notificationsViewAll', { defaultValue: 'View all' })}
            </button>
          </div>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        {loading ? (
          <div className="gl-notification-state">
            {t('loading', { defaultValue: 'Loading...' })}
          </div>
        ) : menuItems.length === 0 ? (
          <div className="gl-notification-state">
            {t('notificationsEmpty', { defaultValue: 'No notifications yet' })}
          </div>
        ) : (
          <div className="gl-notification-list">
            {menuItems.map((item) => (
              <button
                type="button"
                key={item.id}
                className={item.unread ? 'gl-notification-item is-unread' : 'gl-notification-item'}
                onClick={() => openNotification(item)}
              >
                <span className="gl-notification-dot" aria-hidden="true" />
                <Avatar
                  name={item.actorName}
                  src={item.actorAvatar}
                  size={42}
                  className="gl-notification-avatar"
                />
                <span className="gl-notification-copy">
                  <strong>{item.title}</strong>
                  {item.actorLabel && (
                    <span className="gl-notification-actor">
                      {item.actorLabel}
                      {item.actorVerified && <VerifiedBadge size={12} />}
                    </span>
                  )}
                  {item.body && <small>{item.body}</small>}
                  <time>{formatNotificationTime(item.createdAt, i18n.language)}</time>
                </span>
              </button>
            ))}
          </div>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function notificationTitle(
  item: NotificationItem,
  t: ReturnType<typeof useTranslation>['t'],
): string {
  return t(`notificationTypes.${item.type}.title`, { defaultValue: item.title });
}

function notificationMenuNotice(
  item: NotificationItem,
  t: ReturnType<typeof useTranslation>['t'],
): NotificationMenuNotice {
  return {
    id: `notification:${item.id}`,
    title: notificationTitle(item, t),
    body: item.body,
    link: item.link || '/messages/system',
    actorName: notificationActorName(item),
    actorLabel: notificationActorLabel(item),
    actorAvatar: item.actorAvatar,
    actorVerified: item.actorVerified,
    createdAt: item.createdAt,
    count: 1,
    unread: !item.readAt,
    notification: item,
  };
}

function directThreadNotice(
  thread: DirectThread,
  t: ReturnType<typeof useTranslation>['t'],
): NotificationMenuNotice {
  const count = Math.max(1, thread.unread);
  const name = personName(thread.peer.name);
  return {
    id: `direct:${thread.id}`,
    title: t('notificationTypes.direct_message.title', {
      count,
      name,
      defaultValue: '{{name}} 发来 {{count}} 条私信',
    }),
    body: thread.lastMessagePreview,
    link: `/messages/direct/${encodeURIComponent(thread.creatorId)}?from=notice&unread=${count}`,
    actorName: name,
    actorLabel:
      thread.peer.username && !isUuidLike(thread.peer.username) ? `@${thread.peer.username}` : name,
    actorAvatar: thread.peer.avatar,
    actorVerified: thread.peer.verified,
    createdAt: thread.lastMessageAt || new Date().toISOString(),
    count,
    unread: true,
  };
}

function fanGroupNotice(
  group: FanGroup,
  t: ReturnType<typeof useTranslation>['t'],
): NotificationMenuNotice {
  const count = Math.max(1, group.unread);
  const owner = group.members.find((member) => member.role === 'owner') ?? group.members[0];
  return {
    id: `fan-group:${group.id}`,
    title: t('notificationTypes.fan_group_message.title', {
      count,
      name: group.name,
      defaultValue: '{{name}} 有 {{count}} 条新消息',
    }),
    body: t('notificationTypes.fan_group_message.body', {
      count: group.memberCount,
      defaultValue: '{{count}}/200 人 · 粉丝团群聊',
    }),
    link: `/messages/private?group=${encodeURIComponent(group.id)}&from=notice&unread=${count}`,
    actorName: group.name,
    actorLabel: owner ? personName(owner.user.name) : group.name,
    actorAvatar: owner?.user.avatar,
    actorVerified: owner?.user.verified,
    createdAt: group.updatedAt,
    count,
    unread: true,
  };
}

function isChatNotificationType(type: string): boolean {
  return type === 'direct_message' || type === 'fan_group_message';
}

function notificationNoticeTime(value: string): number {
  const time = Date.parse(value);
  return Number.isFinite(time) ? time : 0;
}

function notificationActorName(item: NotificationItem): string {
  return item.actorName || item.actorUsername || item.body || 'GoLive';
}

function notificationActorLabel(item: NotificationItem): string {
  if (item.actorUsername && !isUuidLike(item.actorUsername)) return `@${item.actorUsername}`;
  return item.actorName ? personName(item.actorName) : '';
}

function formatNotificationTime(value: string, locale: string): string {
  return formatRelativeTime(value, locale);
}
