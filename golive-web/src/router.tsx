import { lazy, Suspense } from 'react';
import { Navigate, createBrowserRouter } from 'react-router-dom';
import App from '@/App';
import NotFoundPage from '@/pages/NotFoundPage';
import RouteErrorPage from '@/pages/RouteErrorPage';

const HomePage = lazy(() => import('@/pages/HomePage'));
const SearchPage = lazy(() => import('@/pages/SearchPage'));
const LiveRoomPage = lazy(() => import('@/pages/LiveRoomPage'));
const ChannelPage = lazy(() => import('@/pages/ChannelPage'));
const LoginPage = lazy(() => import('@/pages/LoginPage'));
const BannedAccountPage = lazy(() => import('@/pages/BannedAccountPage'));
const PrivacyPage = lazy(() =>
  import('@/pages/LegalPages').then((module) => ({
    default: module.PrivacyPage,
  })),
);
const TermsPage = lazy(() =>
  import('@/pages/LegalPages').then((module) => ({
    default: module.TermsPage,
  })),
);
const CoinPage = lazy(() => import('@/pages/CoinPage'));
const AdminPage = lazy(() => import('@/pages/AdminPage'));
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
const CreatorFanGroupsPage = lazy(() =>
  import('@/pages/CreatorStudioPage').then((module) => ({
    default: module.CreatorFanGroupsPage,
  })),
);
const CreatorReplayPage = lazy(() =>
  import('@/pages/CreatorStudioPage').then((module) => ({
    default: module.CreatorReplayPage,
  })),
);
const CreatorLiveReplaysPage = lazy(() =>
  import('@/pages/CreatorStudioPage').then((module) => ({
    default: module.CreatorLiveReplaysPage,
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
const MessagesPage = lazy(() => import('@/pages/MessagesPage'));
const MicStagePage = lazy(() => import('@/pages/MicStagePage'));
const SettingsPage = lazy(() =>
  import('@/pages/LibraryPages').then((module) => ({
    default: module.SettingsPage,
  })),
);

export const router = createBrowserRouter([
  {
    element: <App />,
    // A crash in the shell itself replaces the whole page...
    errorElement: <RouteErrorPage />,
    children: [
      {
        // ...while a crash inside a page renders in the shell, so the top bar
        // and sidebar are still there to navigate away.
        errorElement: <RouteErrorPage />,
        children: [
          { index: true, element: <HomePage /> },
          { path: 'search', element: <SearchPage /> },
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
              { path: 'fan-groups', element: <CreatorFanGroupsPage /> },
              { path: 'replay', element: <CreatorReplayPage /> },
              { path: 'live-replays', element: <CreatorLiveReplaysPage /> },
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
          { path: 'messages', element: <MessagesPage /> },
          { path: 'messages/:section', element: <MessagesPage /> },
          { path: 'messages/:section/:targetId', element: <MessagesPage /> },
          { path: 'settings', element: <SettingsPage /> },
          { path: 'account-banned', element: <BannedAccountPage /> },
          { path: 'privacy', element: <PrivacyPage /> },
          { path: 'terms', element: <TermsPage /> },
          { path: 'admin', element: <Navigate to="/admin/dashboard" replace /> },
          { path: 'admin/applications', element: <Navigate to="/admin/creators" replace /> },
          { path: 'admin/:section', element: <AdminPage /> },
          { path: 'admin/*', element: <AdminPage /> },
          { path: 'login', element: <LoginPage /> },
          { path: '*', element: <NotFoundPage /> },
        ],
      },
    ],
  },
  {
    // Standalone (no app shell): headless audio stage for the streamer's OBS
    // Browser Source. Renders only hidden <audio> elements + a tiny badge.
    path: '/mic-stage/:roomId',
    element: (
      <Suspense fallback={null}>
        <MicStagePage />
      </Suspense>
    ),
  },
]);
