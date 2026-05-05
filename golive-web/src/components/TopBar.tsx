import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import { Bell, Coins, Plus, User as UserIcon, X } from 'lucide-react';
import { logout as doLogout, useMe } from '@/api/auth';
import {
  useMarkAllNotificationsRead,
  useMarkNotificationRead,
  useNotifications,
  type NotificationItem,
} from '@/api/room';
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
  const searchRef = useRef<HTMLFormElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const currentUser = me.data ?? user;
  const balance = me.data?.coinBalance ?? user?.coinBalance ?? 0;
  const activeLiveId = useActiveCreatorLiveId(currentUser?.id);
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

  const commitSearch = (value = search) => {
    const q = value.trim();
    setSuggestionsOpen(false);
    setActiveSuggestion(-1);
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
      return;
    }
    if (event.key === 'Escape') {
      setSuggestionsOpen(false);
      setActiveSuggestion(-1);
    }
  };

  return (
    <>
      <header className="gl-topbar">
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
          role="search"
        >
          <div className="gl-search-pill">
            <input
              ref={inputRef}
              className="gl-search-input"
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
                    <em>{suggestionTypeLabel(item.type)}</em>
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
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button type="button" className="gl-lang-toggle" aria-label={t('lang.toggle')}>
                {t(`lang.${lang}`)}
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
          <button
            type="button"
            className="gl-icon-btn"
            onClick={toggleTheme}
            aria-label={t('theme.toggle')}
          >
            {isDark ? <Icons.Sun size={22} /> : <Icons.Moon size={22} />}
          </button>

          {isAuthed ? (
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
                  <DropdownMenuItem className="flex items-center gap-2">
                    <Coins size={14} />
                    <span>{t('account.coins', { amount: balance.toLocaleString() })}</span>
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    className="flex items-center gap-2"
                    onClick={() => navigate('/coins?focus=recharge')}
                  >
                    <Plus size={14} />
                    <span>{t('account.rechargeCoins')}</span>
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  {currentUser?.role === 'admin' && (
                    <DropdownMenuItem onClick={() => navigate('/admin/dashboard')}>
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

function suggestionTypeLabel(type: string): string {
  switch (type) {
    case 'creator':
      return '主播';
    case 'live':
      return '直播';
    case 'replay':
      return '回放';
    case 'appointment':
      return '预告';
    case 'post':
      return '帖子';
    default:
      return '';
  }
}

function NotificationBell() {
  const { t, i18n } = useTranslation('common');
  const navigate = useNavigate();
  const notifications = useNotifications(true, 1, 8);
  const markRead = useMarkNotificationRead();
  const markAllRead = useMarkAllNotificationsRead();
  const items = notifications.data?.items ?? [];
  const unread = notifications.data?.unread ?? 0;

  const openNotification = (item: NotificationItem) => {
    if (!item.readAt) markRead.mutate(item.id);
    if (item.link) navigate(item.link);
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className="gl-icon-btn gl-notification-btn"
          aria-label={t('notifications')}
        >
          <Bell size={22} />
          {unread > 0 && <span className="gl-bell-dot" />}
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="gl-notification-menu w-80">
        <DropdownMenuLabel className="gl-notification-head">
          <span>{t('notifications')}</span>
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
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        {notifications.isPending ? (
          <div className="gl-notification-state">
            {t('loading', { defaultValue: 'Loading...' })}
          </div>
        ) : items.length === 0 ? (
          <div className="gl-notification-state">
            {t('notificationsEmpty', { defaultValue: 'No notifications yet' })}
          </div>
        ) : (
          <div className="gl-notification-list">
            {items.map((item) => (
              <DropdownMenuItem
                key={item.id}
                className={`gl-notification-item${item.readAt ? '' : 'is-unread'}`}
                onClick={() => openNotification(item)}
              >
                <span className="gl-notification-dot" aria-hidden="true" />
                <Avatar
                  name={notificationActorName(item)}
                  src={item.actorAvatar}
                  size={42}
                  className="gl-notification-avatar"
                />
                <span className="gl-notification-copy">
                  <strong>{notificationTitle(item, t)}</strong>
                  {notificationActorLabel(item) && (
                    <span className="gl-notification-actor">
                      {notificationActorLabel(item)}
                      {item.actorVerified && <VerifiedBadge size={12} />}
                    </span>
                  )}
                  {item.body && <small>{item.body}</small>}
                  <time>{formatNotificationTime(item.createdAt, i18n.language)}</time>
                </span>
              </DropdownMenuItem>
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

function notificationActorName(item: NotificationItem): string {
  return item.actorName || item.actorUsername || item.body || 'GoLive';
}

function notificationActorLabel(item: NotificationItem): string {
  if (item.actorUsername) return `@${item.actorUsername}`;
  return item.actorName || '';
}

function formatNotificationTime(value: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value));
}
