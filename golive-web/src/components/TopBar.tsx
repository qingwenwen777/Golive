import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import { Coins, Plus, User as UserIcon } from 'lucide-react';
import { logout as doLogout, useMe } from '@/api/auth';
import { Avatar } from '@/components/Avatar';
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
  const currentUser = me.data ?? user;
  const balance = me.data?.coinBalance ?? user?.coinBalance ?? 0;
  const activeLiveId = useActiveCreatorLiveId(currentUser?.id);

  useEffect(() => {
    setSearch(new URLSearchParams(location.search).get('q') ?? '');
  }, [location.search]);

  const handleSearchSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const q = search.trim();
    navigate(q ? `/?q=${encodeURIComponent(q)}` : '/');
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
          <button type="button" className="gl-logo" onClick={onLogoClick} aria-label="GoLive">
            <GoLiveLogo height={20} />
            <span className="gl-logo-word">GoLive</span>
            <span className="gl-logo-country">{t(`lang.short.${lang}`)}</span>
          </button>
        </div>

        <form className="gl-topbar-search" onSubmit={handleSearchSubmit} role="search">
          <div className="gl-search-pill">
            <input
              className="gl-search-input"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder={t('searchPlaceholder')}
              aria-label={t('search')}
            />
            <button type="submit" className="gl-search-btn" aria-label={t('search')}>
              <Icons.Search size={22} />
            </button>
          </div>
        </form>

        <div className="gl-topbar-right">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button type="button" className="gl-lang-toggle" aria-label={t('lang.toggle')}>
                {t(`lang.short.${lang}`)}
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
              <button
                type="button"
                className="gl-create-btn"
                onClick={() => navigate(activeLiveId ? `/studio/live/${activeLiveId}` : '/studio/prepare')}
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
                    <DropdownMenuItem onClick={() => navigate('/admin')}>
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
