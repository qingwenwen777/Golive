import axios from 'axios';
import { useAuthStore } from '@/stores/useAuthStore';

export function getAuthToken(): string | null {
  return useAuthStore.getState().token;
}

export function getRefreshToken(): string | null {
  return useAuthStore.getState().refreshToken;
}

let refreshPromise: Promise<string> | null = null;

export async function refreshAuthToken(): Promise<string> {
  if (refreshPromise) return refreshPromise;

  refreshPromise = (async () => {
    const refreshToken = getRefreshToken();
    if (!refreshToken) throw new Error('no-refresh-token');

    const { data } = await axios.post<{ token: string; refreshToken: string }>(
      `${import.meta.env.VITE_API_BASE}/auth/refresh`,
      { refreshToken },
      { timeout: 10_000 },
    );
    useAuthStore.getState().setTokens(data.token, data.refreshToken);
    return data.token;
  })().finally(() => {
    refreshPromise = null;
  });

  return refreshPromise;
}
