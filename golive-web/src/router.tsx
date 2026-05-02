import { Navigate, createBrowserRouter } from 'react-router-dom';
import App from '@/App';
import HomePage from '@/pages/HomePage';
import LiveRoomPage from '@/pages/LiveRoomPage';
import ChannelPage from '@/pages/ChannelPage';
import { CreatorAnalyticsPage, LiveAnalysisPage } from '@/pages/CreatorAnalyticsPage';
import LoginPage from '@/pages/LoginPage';
import NotFoundPage from '@/pages/NotFoundPage';
import AdminApplicationsPage, { AdminIndexPage } from '@/pages/AdminApplicationsPage';
import CoinPage from '@/pages/CoinPage';
import {
  HistoryPage,
  LikedPage,
  SettingsPage,
  SubscriptionsPage,
  WatchLaterPage,
  YouPage,
} from '@/pages/LibraryPages';

export const router = createBrowserRouter([
  {
    element: <App />,
    errorElement: <NotFoundPage />,
    children: [
      { index: true, element: <HomePage /> },
      { path: 'live-now', element: <Navigate to="/" replace /> },
      { path: 'live/:id', element: <LiveRoomPage /> },
      { path: 'channel/:name', element: <ChannelPage /> },
      { path: 'studio/analytics/:name', element: <CreatorAnalyticsPage /> },
      { path: 'studio/analytics/:name/live/:recordId', element: <LiveAnalysisPage /> },
      { path: 'subscriptions', element: <SubscriptionsPage /> },
      { path: 'you', element: <YouPage /> },
      { path: 'coins', element: <CoinPage /> },
      { path: 'history', element: <HistoryPage /> },
      { path: 'watch-later', element: <WatchLaterPage /> },
      { path: 'liked', element: <LikedPage /> },
      { path: 'settings', element: <SettingsPage /> },
      { path: 'admin', element: <AdminIndexPage /> },
      { path: 'admin/applications', element: <AdminApplicationsPage /> },
      { path: 'login', element: <LoginPage /> },
      { path: '*', element: <NotFoundPage /> },
    ],
  },
]);
