import { useEffect, useRef, useState, type ComponentType } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import type { LucideProps } from 'lucide-react';
import { useMe } from '@/api/auth';
import { Icons } from '@/components/Icons';
import { useActiveCreatorLiveId } from '@/features/creator/useActiveCreatorLiveId';
import { cn } from '@/lib/cn';

export interface SidebarProps {
  collapsed: boolean;
  activeKey?: string;
  onNav?: (key: string) => void;
}

type IconComponent = ComponentType<LucideProps>;

interface NavItem {
  key: string;
  icon: IconComponent;
  label: string;
  chev?: boolean;
  route?: string;
}

export function Sidebar({ collapsed, activeKey, onNav }: SidebarProps) {
  const { t, i18n } = useTranslation('common');
  const navigate = useNavigate();
  const location = useLocation();
  const me = useMe();

  const active = activeKey ?? deriveActiveKey(location.pathname);
  const activeLiveId = useActiveCreatorLiveId(me.data?.id);
  const isStudioActive = active.startsWith('studio');
  const isMessagesActive = active.startsWith('messages');
  const canShowStudioSubnav = me.data?.livePermissionStatus === 'approved';
  const studioRoute = canShowStudioSubnav
    ? activeLiveId
      ? `/studio/live/${activeLiveId}`
      : '/studio/overview'
    : '/studio/prepare';
  const [studioSubnavOpen, setStudioSubnavOpen] = useState(() => isStudioActive);
  const [messagesSubnavOpen, setMessagesSubnavOpen] = useState(() => isMessagesActive);
  const wasStudioActiveRef = useRef(isStudioActive);
  const wasCollapsedRef = useRef(collapsed);
  const couldShowStudioSubnavRef = useRef(canShowStudioSubnav);

  useEffect(() => {
    const expandedFromCollapsed = wasCollapsedRef.current && !collapsed;
    const becameAllowed = !couldShowStudioSubnavRef.current && canShowStudioSubnav;

    if (!canShowStudioSubnav || collapsed) {
      setStudioSubnavOpen(false);
    } else if (
      isStudioActive &&
      (!wasStudioActiveRef.current || expandedFromCollapsed || becameAllowed)
    ) {
      setStudioSubnavOpen(true);
    } else if (!isStudioActive) {
      setStudioSubnavOpen(false);
    }

    wasStudioActiveRef.current = isStudioActive;
    wasCollapsedRef.current = collapsed;
    couldShowStudioSubnavRef.current = canShowStudioSubnav;
  }, [canShowStudioSubnav, collapsed, isStudioActive]);

  useEffect(() => {
    if (collapsed || !isMessagesActive) {
      setMessagesSubnavOpen(false);
      return;
    }
    setMessagesSubnavOpen(true);
  }, [collapsed, isMessagesActive]);

  const handleNav = (key: string, route?: string): void => {
    if (key === 'studio') {
      if (canShowStudioSubnav && !collapsed && isStudioActive) {
        setStudioSubnavOpen((open) => !open);
        return;
      }
      if (canShowStudioSubnav && !collapsed) {
        setStudioSubnavOpen(true);
      }
    }
    if (key === 'messages') {
      if (!collapsed && isMessagesActive) {
        setMessagesSubnavOpen((open) => !open);
        return;
      }
      if (!collapsed) setMessagesSubnavOpen(true);
    }

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
      chev: canShowStudioSubnav,
      route: studioRoute,
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
      key: 'studio-posts',
      icon: Icons.FileText,
      label: t('nav.studioPosts'),
      route: '/studio/posts',
    },
    {
      key: 'studio-appointments',
      icon: Icons.CalendarClock,
      label: t('nav.studioAppointments', { defaultValue: '直播预约' }),
      route: '/studio/appointments',
    },
    {
      key: 'studio-moderators',
      icon: Icons.ShieldCheck,
      label: t('nav.studioModerators', { defaultValue: '房间房管' }),
      route: '/studio/moderators',
    },
    {
      key: 'studio-fan-groups',
      icon: Icons.Message,
      label: t('nav.studioFanGroups', { defaultValue: '群聊管理' }),
      route: '/studio/fan-groups',
    },
    {
      key: 'studio-replay',
      icon: Icons.History,
      label: t('nav.studioReplay', { defaultValue: 'Data replay' }),
      route: '/studio/replay',
    },
    {
      key: 'studio-live-replays',
      icon: Icons.Live,
      label: t('nav.studioLiveReplays', { defaultValue: 'Live replays' }),
      route: '/studio/live-replays',
    },
  ];

  const youItems: NavItem[] = [
    { key: 'you', icon: Icons.Library, label: t('nav.yourLibrary'), chev: true, route: '/you' },
    {
      key: 'messages',
      icon: Icons.Message,
      label: t('nav.messages', { defaultValue: '消息' }),
      chev: true,
      route: '/messages/private',
    },
    { key: 'history', icon: Icons.History, label: t('nav.history'), route: '/history' },
    { key: 'later', icon: Icons.WatchLater, label: t('nav.watchLater'), route: '/watch-later' },
    { key: 'liked', icon: Icons.Heart, label: t('nav.liked'), route: '/liked' },
  ];

  const messageItems: NavItem[] = [
    {
      key: 'messages-private',
      icon: Icons.Message,
      label: t('nav.messagesPrivate', { defaultValue: '我的消息' }),
      route: '/messages/private',
    },
    {
      key: 'messages-replies',
      icon: Icons.Bell,
      label: t('nav.messagesReplies', { defaultValue: '回复我的' }),
      route: '/messages/replies',
    },
    {
      key: 'messages-likes',
      icon: Icons.Heart,
      label: t('nav.messagesLikes', { defaultValue: '收到的赞' }),
      route: '/messages/likes',
    },
    {
      key: 'messages-system',
      icon: Icons.ShieldCheck,
      label: t('nav.messagesSystem', { defaultValue: '系统通知' }),
      route: '/messages/system',
    },
    {
      key: 'messages-settings',
      icon: Icons.Wallet,
      label: t('nav.messagesSettings', { defaultValue: '消息设置' }),
      route: '/messages/settings',
    },
  ];

  const renderItem = (it: NavItem) => {
    const Icon = it.icon;
    const controlsStudioSubnav = it.key === 'studio' && canShowStudioSubnav && !collapsed;
    return (
      <button
        key={it.key}
        type="button"
        aria-label={it.label}
        aria-controls={controlsStudioSubnav ? 'gl-sidebar-studio-subnav' : undefined}
        aria-expanded={controlsStudioSubnav ? studioSubnavOpen : undefined}
        title={it.label}
        className={cn(
          'gl-side-item',
          isNavItemActive(it.key, active) && 'is-active',
          it.key === 'studio' && studioSubnavOpen && canShowStudioSubnav && 'is-sub-open',
          it.key === 'coins' && 'is-coins',
          collapsed && 'is-col',
        )}
        onClick={() => handleNav(it.key, it.route)}
      >
        <Icon size={22} />
        {!collapsed && <span className="gl-side-label">{it.label}</span>}
        {!collapsed && it.chev && <Icons.ChevronRight className="gl-side-chevron" size={16} />}
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
    <aside
      className={cn(
        'gl-sidebar',
        collapsed && 'is-col',
        (i18n.resolvedLanguage ?? i18n.language).startsWith('en') && 'is-en',
      )}
    >
      <nav className="gl-side-sec">
        {mainItems.map(renderItem)}
        {!collapsed && canShowStudioSubnav && isStudioActive && studioSubnavOpen && (
          <div id="gl-sidebar-studio-subnav" className="gl-side-subsec">
            {studioItems.map(renderSubItem)}
          </div>
        )}
      </nav>
      {!collapsed && (
        <>
          <div className="gl-side-divider" />
          <div className="gl-side-sec">
            <div className="gl-side-heading">{t('nav.yourLibrary')}</div>
            {youItems.map((item) => (
              <div key={item.key}>
                {renderItem(item)}
                {item.key === 'messages' && messagesSubnavOpen && (
                  <div id="gl-sidebar-messages-subnav" className="gl-side-subsec">
                    {messageItems.map(renderSubItem)}
                  </div>
                )}
              </div>
            ))}
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
  if (key === 'messages') return active.startsWith('messages');
  return active === key;
}

function deriveActiveKey(pathname: string): string {
  if (pathname === '/') return 'home';
  if (pathname === '/studio' || pathname === '/studio/overview') return 'studio-overview';
  if (pathname.startsWith('/studio/appointments')) return 'studio-appointments';
  if (pathname.startsWith('/studio/posts')) return 'studio-posts';
  if (pathname.startsWith('/studio/moderators')) return 'studio-moderators';
  if (pathname.startsWith('/studio/fan-groups')) return 'studio-fan-groups';
  if (pathname.startsWith('/studio/live-replays')) return 'studio-live-replays';
  if (pathname.startsWith('/studio/prepare') || pathname.startsWith('/studio/live/')) {
    return 'studio-prepare';
  }
  if (pathname.startsWith('/studio/replay') || pathname.startsWith('/studio/analytics')) {
    return 'studio-replay';
  }
  if (pathname === '/subscriptions' || pathname.startsWith('/channel/')) return 'subs';
  if (pathname === '/coins') return 'coins';
  if (pathname === '/you') return 'you';
  if (pathname === '/messages' || pathname === '/messages/private') return 'messages-private';
  if (pathname.startsWith('/messages/direct')) return 'messages-private';
  if (pathname.startsWith('/messages/replies')) return 'messages-replies';
  if (pathname.startsWith('/messages/likes')) return 'messages-likes';
  if (pathname.startsWith('/messages/system')) return 'messages-system';
  if (pathname.startsWith('/messages/settings')) return 'messages-settings';
  if (pathname === '/history') return 'history';
  if (pathname === '/watch-later') return 'later';
  if (pathname === '/liked') return 'liked';
  return '';
}
