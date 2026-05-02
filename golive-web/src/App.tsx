import { Suspense, useEffect, useRef, useState } from 'react';
import { Outlet, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { TopBar } from '@/components/TopBar';
import { Sidebar } from '@/components/Sidebar';
import { Toaster } from '@/components/ui/sonner';
import { LoginModal } from '@/features/auth/LoginModal';
import { useMediaQuery } from '@/hooks/useMediaQuery';

export default function App() {
  const isNarrow = useMediaQuery('(max-width: 767px)');
  const [userCollapsed, setUserCollapsed] = useState(false);
  const collapsed = isNarrow || userCollapsed;

  return (
    <div className="min-h-screen bg-bg text-text">
      <TopBar onMenuClick={() => setUserCollapsed((v) => !v)} />
      <div className="gl-layout">
        <Sidebar collapsed={collapsed} />
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
  const firstNavigation = useRef(true);
  const [transitioning, setTransitioning] = useState(false);

  useEffect(() => {
    if (firstNavigation.current) {
      firstNavigation.current = false;
      return;
    }

    setTransitioning(true);
    const timer = window.setTimeout(() => setTransitioning(false), 180);
    return () => window.clearTimeout(timer);
  }, [location.key]);

  return (
    <Suspense fallback={<RouteLoading />}>
      {transitioning ? <RouteLoading /> : <Outlet key={location.key} />}
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
