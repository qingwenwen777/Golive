import axios, { AxiosError, type AxiosRequestConfig, type InternalAxiosRequestConfig } from 'axios';
import { useAuthStore } from '@/stores/useAuthStore';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { refreshAuthToken } from '@/lib/authToken';

interface RetriableConfig extends InternalAxiosRequestConfig {
  _retry?: boolean;
}

export const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE,
  timeout: 10_000,
});

http.interceptors.request.use((config) => {
  const token = useAuthStore.getState().token;
  if (token) {
    config.headers = config.headers ?? {};
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

async function doRefresh(): Promise<string> {
  return refreshAuthToken();
}

function isAuthEndpoint(url: string | undefined): boolean {
  if (!url) return false;
  return url.includes('/auth/login') || url.includes('/auth/refresh') || url.includes('/auth/logout');
}

http.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    const config = error.config as RetriableConfig | undefined;
    const status = error.response?.status;

    if (import.meta.env.DEV) {
      console.error('[api]', config?.url, error.message);
    }

    if (
      status === 401 &&
      config &&
      !config._retry &&
      !isAuthEndpoint(config.url)
    ) {
      try {
        const newToken = await doRefresh();
        config._retry = true;
        config.headers = config.headers ?? {};
        config.headers.Authorization = `Bearer ${newToken}`;
        return http.request(config as AxiosRequestConfig);
      } catch (refreshErr) {
        useAuthStore.getState().logout();
        useAuthModalStore.getState().openLogin();
        return Promise.reject(refreshErr);
      }
    }

    return Promise.reject(error);
  },
);
