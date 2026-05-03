import { lazy } from 'react';
import { Navigate, createBrowserRouter } from 'react-router-dom';
import App from '@/App';
import NotFoundPage from '@/pages/NotFoundPage';

const HomePage = lazy(() => import('@/pages/HomePage'));
const LiveRoomPage = lazy(() => import('@/pages/LiveRoomPage'));
const ChannelPage = lazy(() => import('@/pages/ChannelPage'));
const LoginPage = lazy(() => import('@/pages/LoginPage'));
const CoinPage = lazy(() => import('@/pages/CoinPage'));
const AdminApplicationsPage = lazy(() => import('@/pages/AdminApplicationsPage'));
const CreatorAnalyticsPage = lazy(() =>
  import('@/pages/CreatorAnalyticsPage').then((module) => ({
    default: module.CreatorAnalyticsPage,
  })),
);
const LiveAnalysisPage = lazy(() =>
  import('@/pages/CreatorAnalyticsPage').then((module) => ({
    default: module.LiveAnalysisPage,
  })),
);
const CreatorStudioShell = lazy(() =>
  import('@/pages/CreatorStudioPage').then((module) => ({
    default: module.CreatorStudioShell,
  })),
);
const CreatorStudioIndexPage = lazy(() =>
  import('@/pages/CreatorStudioPage').then((module) => ({
    default: module.CreatorStudioIndexPage,
  })),
);
const CreatorStudioOverviewPage = lazy(() =>
  import('@/pages/CreatorStudioPage').then((module) => ({
    default: module.CreatorStudioOverviewPage,
  })),
);
const CreatorPreparePage = lazy(() =>
  import('@/pages/CreatorStudioPage').then((module) => ({
    default: module.CreatorPreparePage,
  })),
);
const CreatorAppointmentsPage = lazy(() =>
  import('@/pages/CreatorStudioPage').then((module) => ({
    default: module.CreatorAppointmentsPage,
  })),
);
const CreatorPostsPage = lazy(() =>
  import('@/pages/CreatorStudioPage').then((module) => ({
    default: module.CreatorPostsPage,
  })),
);
const CreatorRoomModeratorsPage = lazy(() =>
  import('@/pages/CreatorStudioPage').then((module) => ({
    default: module.CreatorRoomModeratorsPage,
  })),
);
const CreatorReplayPage = lazy(() =>
  import('@/pages/CreatorStudioPage').then((module) => ({
    default: module.CreatorReplayPage,
  })),
);
const CreatorLiveConsolePage = lazy(() =>
  import('@/pages/CreatorStudioPage').then((module) => ({
    default: module.CreatorLiveConsolePage,
  })),
);
const SubscriptionsPage = lazy(() =>
  import('@/pages/LibraryPages').then((module) => ({
    default: module.SubscriptionsPage,
  })),
);
const YouPage = lazy(() =>
  import('@/pages/LibraryPages').then((module) => ({
    default: module.YouPage,
  })),
);
const HistoryPage = lazy(() =>
  import('@/pages/LibraryPages').then((module) => ({
    default: module.HistoryPage,
  })),
);
const WatchLaterPage = lazy(() =>
  import('@/pages/LibraryPages').then((module) => ({
    default: module.WatchLaterPage,
  })),
);
const LikedPage = lazy(() =>
  import('@/pages/LibraryPages').then((module) => ({
    default: module.LikedPage,
  })),
);
const SettingsPage = lazy(() =>
  import('@/pages/LibraryPages').then((module) => ({
    default: module.SettingsPage,
  })),
);

export const router = createBrowserRouter([
  {
    element: <App />,
    errorElement: <NotFoundPage />,
    children: [
      { index: true, element: <HomePage /> },
      { path: 'live-now', element: <Navigate to="/" replace /> },
      { path: 'live/:id', element: <LiveRoomPage /> },
      { path: 'channel/:name', element: <ChannelPage /> },
      {
        path: 'studio',
        element: <CreatorStudioShell />,
        children: [
          { index: true, element: <CreatorStudioIndexPage /> },
          { path: 'overview', element: <CreatorStudioOverviewPage /> },
          { path: 'prepare', element: <CreatorPreparePage /> },
          { path: 'posts', element: <CreatorPostsPage /> },
          { path: 'appointments', element: <CreatorAppointmentsPage /> },
          { path: 'moderators', element: <CreatorRoomModeratorsPage /> },
          { path: 'replay', element: <CreatorReplayPage /> },
        ],
      },
      { path: 'studio/live/:id', element: <CreatorLiveConsolePage /> },
      { path: 'studio/analytics/:name', element: <CreatorAnalyticsPage /> },
      { path: 'studio/analytics/:name/live/:recordId', element: <LiveAnalysisPage /> },
      { path: 'subscriptions', element: <SubscriptionsPage /> },
      { path: 'you', element: <YouPage /> },
      { path: 'coins', element: <CoinPage /> },
      { path: 'history', element: <HistoryPage /> },
      { path: 'watch-later', element: <WatchLaterPage /> },
      { path: 'liked', element: <LikedPage /> },
      { path: 'settings', element: <SettingsPage /> },
      { path: 'admin', element: <Navigate to="/admin/applications" replace /> },
      { path: 'admin/applications', element: <AdminApplicationsPage /> },
      { path: 'admin/*', element: <AdminApplicationsPage /> },
      { path: 'login', element: <LoginPage /> },
      { path: '*', element: <NotFoundPage /> },
    ],
  },
]);
