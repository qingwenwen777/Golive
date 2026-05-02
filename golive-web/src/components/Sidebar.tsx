import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import type { LucideProps } from 'lucide-react';
import { useMe } from '@/api/auth';
import { useRooms } from '@/api/room';
import { Icons } from '@/components/Icons';
import { loadPublisherSession } from '@/features/creator/CreateLiveDialog';
import { cn } from '@/lib/cn';
import type { Stream } from '@/types/stream';

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
  const me = useMe();
  const liveRooms = useRooms({ size: 100 });

  const active = activeKey ?? deriveActiveKey(location.pathname);
  const activeLiveId = useMemo(
    () => resolveActiveLiveId(liveRooms.data?.items ?? [], me.data?.id),
    [liveRooms.data?.items, me.data?.id],
  );

  const handleNav = (key: string, route?: string): void => {
    if (onNav) onNav(key);
    else if (route) navigate(route);
  };

  const mainItems: NavItem[] = [
    { key: 'home', icon: Icons.Home, label: t('nav.home'), route: '/' },
    { key: 'subs', icon: Icons.Subs, label: t('nav.subscriptions'), route: '/subscriptions' },
    { key: 'coins', icon: Icons.Wallet, label: t('nav.coins', 'Coins'), route: '/coins' },
    {
      key: 'studio',
      icon: Icons.Live,
      label: t('nav.creatorStudio', { defaultValue: 'Creator Studio' }),
      route: activeLiveId ? `/studio/live/${activeLiveId}` : '/studio/overview',
    },
  ];

  const studioItems: NavItem[] = [
    {
      key: 'studio-overview',
      icon: Icons.Home,
      label: t('nav.studioOverview', { defaultValue: 'Overview' }),
      route: '/studio/overview',
    },
    {
      key: 'studio-prepare',
      icon: Icons.Live,
      label: t('nav.studioPrepare', { defaultValue: 'Stream setup' }),
      route: '/studio/prepare',
    },
    {
      key: 'studio-replay',
      icon: Icons.History,
      label: t('nav.studioReplay', { defaultValue: 'Data replay' }),
      route: '/studio/replay',
    },
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
        className={cn(
          'gl-side-item',
          isNavItemActive(it.key, active) && 'is-active',
          collapsed && 'is-col',
        )}
        onClick={() => handleNav(it.key, it.route)}
      >
        <Icon size={22} />
        {!collapsed && <span className="gl-side-label">{it.label}</span>}
        {!collapsed && it.chev && <Icons.ChevronRight size={16} />}
      </button>
    );
  };

  const renderSubItem = (it: NavItem) => {
    const Icon = it.icon;
    return (
      <button
        key={it.key}
        type="button"
        aria-label={it.label}
        title={it.label}
        className={cn('gl-side-sub-item', active === it.key && 'is-active')}
        onClick={() => handleNav(it.key, it.route)}
      >
        <Icon size={17} />
        <span>{it.label}</span>
      </button>
    );
  };

  return (
    <aside className={cn('gl-sidebar', collapsed && 'is-col')}>
      <nav className="gl-side-sec">
        {mainItems.map(renderItem)}
        {!collapsed && active.startsWith('studio') && (
          <div className="gl-side-subsec">{studioItems.map(renderSubItem)}</div>
        )}
      </nav>
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

function isNavItemActive(key: string, active: string): boolean {
  if (key === 'studio') return active.startsWith('studio');
  return active === key;
}

function resolveActiveLiveId(items: Stream[], userId?: string): string {
  const session = loadPublisherSession();
  const liveByUser = userId
    ? items.find((item) => item.ownerId === userId && isLiveStream(item))
    : undefined;
  if (liveByUser) return liveByUser.id;

  const liveBySession = session?.streamId
    ? items.find((item) => item.id === session.streamId && isLiveStream(item))
    : undefined;
  if (liveBySession) return liveBySession.id;

  return session?.streamId ?? '';
}

function isLiveStream(stream: Pick<Stream, 'isLive' | 'status'>): boolean {
  return stream.isLive === true || stream.status === 'live';
}

function deriveActiveKey(pathname: string): string {
  if (pathname === '/') return 'home';
  if (pathname === '/studio' || pathname === '/studio/overview') return 'studio-overview';
  if (pathname.startsWith('/studio/prepare') || pathname.startsWith('/studio/live')) {
    return 'studio-prepare';
  }
  if (pathname.startsWith('/studio/replay') || pathname.startsWith('/studio/analytics')) {
    return 'studio-replay';
  }
  if (pathname === '/subscriptions' || pathname.startsWith('/channel/')) return 'subs';
  if (pathname === '/coins') return 'coins';
  if (pathname === '/you') return 'you';
  if (pathname === '/history') return 'history';
  if (pathname === '/watch-later') return 'later';
  if (pathname === '/liked') return 'liked';
  return '';
}
