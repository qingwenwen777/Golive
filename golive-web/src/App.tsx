import { Suspense, useState } from 'react';
import { Outlet } from 'react-router-dom';
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
          <Suspense fallback={<RouteLoading />}>
            <Outlet />
          </Suspense>
        </main>
      </div>
      <Toaster />
      <LoginModal />
    </div>
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
