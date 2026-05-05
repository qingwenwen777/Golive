import { Suspense, useEffect, useRef, useState, type ReactNode } from 'react';
import { useLocation, useOutlet } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { TopBar } from '@/components/TopBar';
import { Sidebar } from '@/components/Sidebar';
import { TopProgressBar } from '@/components/TopProgressBar';
import { Toaster } from '@/components/ui/sonner';
import { LoginModal } from '@/features/auth/LoginModal';
import { useMediaQuery } from '@/hooks/useMediaQuery';

export default function App() {
  const location = useLocation();
  const isNarrow = useMediaQuery('(max-width: 767px)');
  const [userCollapsed, setUserCollapsed] = useState(false);
  const isAdminRoute = location.pathname === '/admin' || location.pathname.startsWith('/admin/');
  const collapsed = isNarrow || userCollapsed;

  return (
    <div className="min-h-screen bg-bg text-text">
      <TopProgressBar />
      <TopBar onMenuClick={() => setUserCollapsed((v) => !v)} />
      <div className="gl-layout">
        {!isAdminRoute && <Sidebar collapsed={collapsed} />}
        <main className="gl-main">
          <RouteOutlet />
        </main>
      </div>
      <Toaster />
      <LoginModal />
    </div>
  );
}

function RouteOutlet() {
  const location = useLocation();
  const outlet = useOutlet();
  const displayedKey = useRef(location.key);
  const latestOutlet = useRef(outlet);
  const timers = useRef<number[]>([]);
  const [routeState, setRouteState] = useState<{
    key: string;
    outlet: ReactNode;
    phase: 'idle' | 'entering';
  }>(() => ({
    key: location.key,
    outlet,
    phase: 'idle',
  }));

  latestOutlet.current = outlet;

  const clearTimers = () => {
    timers.current.forEach((timer) => window.clearTimeout(timer));
    timers.current = [];
  };

  useEffect(() => {
    if (displayedKey.current === location.key) {
      return;
    }

    clearTimers();
    displayedKey.current = location.key;
    setRouteState({
      key: location.key,
      outlet: latestOutlet.current,
      phase: 'entering',
    });

    const settleTimer = window.setTimeout(() => {
      setRouteState((current) =>
        current.key === location.key ? { ...current, phase: 'idle' } : current,
      );
    }, 170);
    timers.current = [settleTimer];

    return clearTimers;
  }, [location.key]);

  useEffect(() => clearTimers, []);

  return (
    <Suspense fallback={<RouteLoading />}>
      <div className={`gl-route-page is-${routeState.phase}`} key={routeState.key}>
        {routeState.outlet}
      </div>
    </Suspense>
  );
}

function RouteLoading() {
  const { t } = useTranslation('common');

  return (
    <div className="gl-page gl-route-loading" role="status" aria-live="polite">
      <span className="gl-route-loading-dot" aria-hidden="true" />
      <span>{t('loading', { defaultValue: 'Loading...' })}</span>
    </div>
  );
}
