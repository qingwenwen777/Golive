import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import type { LucideProps } from 'lucide-react';
import { Icons } from '@/components/Icons';
import { cn } from '@/lib/cn';

export interface SidebarProps {
  collapsed: boolean;
  activeKey?: string;
  onNav?: (key: string) => void;
}

type IconComponent = React.ComponentType<LucideProps>;

interface NavItem {
  key: string;
  icon: IconComponent;
  label: string;
  chev?: boolean;
  route?: string;
}

export function Sidebar({ collapsed, activeKey, onNav }: SidebarProps) {
  const { t } = useTranslation('common');
  const navigate = useNavigate();
  const location = useLocation();

  const active = activeKey ?? deriveActiveKey(location.pathname);

  const handleNav = (key: string, route?: string): void => {
    if (onNav) onNav(key);
    else if (route) navigate(route);
  };

  const mainItems: NavItem[] = [
    { key: 'home', icon: Icons.Home, label: t('nav.home'), route: '/' },
    { key: 'subs', icon: Icons.Subs, label: t('nav.subscriptions'), route: '/subscriptions' },
    { key: 'coins', icon: Icons.Wallet, label: t('nav.coins', 'Coins'), route: '/coins' },
  ];

  const youItems: NavItem[] = [
    { key: 'you', icon: Icons.Library, label: t('nav.yourLibrary'), chev: true, route: '/you' },
    { key: 'history', icon: Icons.History, label: t('nav.history'), route: '/history' },
    { key: 'later', icon: Icons.Clock, label: t('nav.watchLater'), route: '/watch-later' },
    { key: 'liked', icon: Icons.Heart, label: t('nav.liked'), route: '/liked' },
  ];

  const renderItem = (it: NavItem) => {
    const Icon = it.icon;
    return (
      <button
        key={it.key}
        type="button"
        aria-label={it.label}
        title={it.label}
        className={cn('gl-side-item', active === it.key && 'is-active', collapsed && 'is-col')}
        onClick={() => handleNav(it.key, it.route)}
      >
        <Icon size={22} />
        {!collapsed && <span className="gl-side-label">{it.label}</span>}
        {!collapsed && it.chev && <Icons.ChevronRight size={16} />}
      </button>
    );
  };

  return (
    <aside className={cn('gl-sidebar', collapsed && 'is-col')}>
      <nav className="gl-side-sec">{mainItems.map(renderItem)}</nav>
      {!collapsed && (
        <>
          <div className="gl-side-divider" />
          <div className="gl-side-sec">
            <div className="gl-side-heading">{t('nav.yourLibrary')}</div>
            {youItems.map(renderItem)}
          </div>
          <div className="gl-side-divider" />
          <div className="gl-side-foot">
            <div>{t('sidebar.foot.about')}</div>
            <div>{t('sidebar.foot.contact')}</div>
            <div>{t('sidebar.foot.ads')}</div>
            <div style={{ marginTop: 8 }}>{t('sidebar.foot.copyright')}</div>
          </div>
        </>
      )}
    </aside>
  );
}

function deriveActiveKey(pathname: string): string {
  if (pathname === '/') return 'home';
  if (pathname === '/subscriptions' || pathname.startsWith('/channel/')) return 'subs';
  if (pathname === '/coins') return 'coins';
  if (pathname === '/you') return 'you';
  if (pathname === '/history') return 'history';
  if (pathname === '/watch-later') return 'later';
  if (pathname === '/liked') return 'liked';
  return '';
}
