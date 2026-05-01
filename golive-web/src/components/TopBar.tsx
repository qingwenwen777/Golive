import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Coins, Plus, User as UserIcon } from 'lucide-react';
import { toast } from 'sonner';
import { logout as doLogout, useMe, useTopupCoins } from '@/api/auth';
import { Avatar } from '@/components/Avatar';
import { CreateLiveDialog } from '@/features/creator/CreateLiveDialog';
import { AvatarUploadDialog } from '@/features/account/AvatarUploadDialog';
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

export interface TopBarProps {
  onMenuClick: () => void;
  onLogoClick?: () => void;
}

export function TopBar({ onMenuClick, onLogoClick }: TopBarProps) {
  const { t } = useTranslation('common');
  const navigate = useNavigate();
  const { theme, toggleTheme } = useThemeStore();
  const { lang, toggleLang } = useLangStore();
  const isDark = theme === 'dark';
  const isAuthed = useIsAuthed();
  const user = useAuthStore((s) => s.user);
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const me = useMe();
  const topup = useTopupCoins();
  const [createOpen, setCreateOpen] = useState(false);
  const [avatarOpen, setAvatarOpen] = useState(false);
  const currentUser = me.data ?? user;
  const balance = me.data?.coinBalance ?? user?.coinBalance ?? 0;

  const handleTopup = () => {
    topup.mutate(
      { amount: 1000 },
      {
        onSuccess: (next) =>
          toast.success(`Coins added. Balance: ${next.coinBalance.toLocaleString()}.`),
        onError: (err) => toast.error(err.message || 'Could not add coins.'),
      },
    );
  };

  return (
    <>
      <header className="gl-topbar">
        <div className="gl-topbar-left">
          <button
            type="button"
            className="gl-icon-btn"
            onClick={onMenuClick}
            aria-label={t('menu')}
          >
            <Icons.Menu size={24} />
          </button>
          <button type="button" className="gl-logo" onClick={onLogoClick} aria-label="GoLive">
            <GoLiveLogo height={20} />
            <span className="gl-logo-word">GoLive</span>
            <span className="gl-logo-country">JP</span>
          </button>
        </div>

        <div className="gl-topbar-search">
          <div className="gl-search-pill">
            <input
              className="gl-search-input"
              placeholder={t('searchPlaceholder')}
              aria-label={t('search')}
            />
            <button type="button" className="gl-search-btn" aria-label={t('search')}>
              <Icons.Search size={22} />
            </button>
          </div>
          <button type="button" className="gl-icon-btn gl-mic" aria-label={t('voiceSearch')}>
            <Icons.Mic size={22} />
          </button>
        </div>

        <div className="gl-topbar-right">
          <button
            type="button"
            className="gl-lang-toggle"
            onClick={toggleLang}
            aria-label={t('lang.toggle')}
          >
            {lang === 'en' ? t('lang.en') : t('lang.ja')}
          </button>
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
              <button type="button" className="gl-create-btn" onClick={() => setCreateOpen(true)}>
                <Icons.Plus size={22} />
                <span className="gl-create-label">{t('create')}</span>
              </button>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button
                    type="button"
                    className="gl-account-menu-trigger"
                    aria-label="Account menu"
                  >
                    <Avatar
                      name={currentUser?.username ?? 'You'}
                      src={currentUser?.avatar}
                      size={32}
                    />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-52">
                  <DropdownMenuLabel className="truncate">
                    {currentUser?.username ?? 'You'}
                  </DropdownMenuLabel>
                  <DropdownMenuItem className="flex items-center gap-2">
                    <Coins size={14} />
                    <span>{balance.toLocaleString()} coins</span>
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    className="flex items-center gap-2"
                    disabled={topup.isPending}
                    onClick={handleTopup}
                  >
                    <Plus size={14} />
                    <span>{topup.isPending ? 'Adding coins...' : 'Add 1,000 coins'}</span>
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onClick={() => navigate(`/channel/${user?.id ?? ''}`)}>
                    Your channel
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => setAvatarOpen(true)}>
                    Change avatar
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => navigate('/settings')}>
                    Settings
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={toggleTheme}>
                    Appearance: {isDark ? 'Dark' : 'Light'}
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={toggleLang}>
                    Language: {lang === 'en' ? 'English' : 'Japanese'}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    onClick={() => {
                      void doLogout();
                    }}
                  >
                    Sign out
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </>
          ) : (
            <button
              type="button"
              onClick={() => openLogin()}
              className="gl-signin-btn flex items-center gap-2 rounded-full border border-accent/60 px-3 py-1.5 text-sm font-semibold text-accent hover:bg-accent/10"
              aria-label="Sign in"
            >
              <UserIcon size={18} />
              <span>Sign in</span>
            </button>
          )}
        </div>
      </header>
      <CreateLiveDialog open={createOpen} onOpenChange={setCreateOpen} />
      <AvatarUploadDialog open={avatarOpen} onOpenChange={setAvatarOpen} user={currentUser} />
    </>
  );
}
