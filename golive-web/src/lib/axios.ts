import axios, { AxiosError, type AxiosRequestConfig, type InternalAxiosRequestConfig } from 'axios';
import { useAuthStore } from '@/stores/useAuthStore';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { isSessionInvalidAfterRefreshFailure, refreshAuthToken } from '@/lib/authToken';
import { CSRF_HEADER, clearCsrfToken, getCsrfToken, needsCsrfToken } from '@/lib/csrfToken';

interface RetriableConfig extends InternalAxiosRequestConfig {
  _retry?: boolean;
  _csrfRetry?: boolean;
}

export const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE,
  timeout: 10_000,
});

http.interceptors.request.use(async (config) => {
  const token = useAuthStore.getState().token;
  if (token) {
    config.headers = config.headers ?? {};
    config.headers.Authorization = `Bearer ${token}`;
  }
  if (needsCsrfToken(config.method)) {
    config.headers = config.headers ?? {};
    config.headers[CSRF_HEADER] = await getCsrfToken();
  }
  return config;
});

async function doRefresh(): Promise<string> {
  return refreshAuthToken();
}

function shouldResetSessionAfterRefreshFailure(err: unknown): boolean {
  return isSessionInvalidAfterRefreshFailure(err, useAuthStore.getState().refreshToken);
}

function isAuthEndpoint(url: string | undefined): boolean {
  if (!url) return false;
  return (
    url.includes('/auth/login') ||
    url.includes('/auth/google/') ||
    url.includes('/auth/refresh') ||
    url.includes('/auth/logout')
  );
}

function isCsrfInvalid(err: AxiosError): boolean {
  const data = err.response?.data as { reason?: unknown } | undefined;
  return err.response?.status === 403 && data?.reason === 'csrf_invalid';
}

http.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    const config = error.config as RetriableConfig | undefined;
    const status = error.response?.status;

    if (import.meta.env.DEV) {
      console.error('[api]', config?.url, error.message);
    }

    if (config && isCsrfInvalid(error) && !config._csrfRetry) {
      clearCsrfToken();
      config._csrfRetry = true;
      if (config.headers) {
        delete (config.headers as Record<string, unknown>)[CSRF_HEADER];
      }
      return http.request(config as AxiosRequestConfig);
    }

    if (status === 401 && config && !config._retry && !isAuthEndpoint(config.url)) {
      try {
        const newToken = await doRefresh();
        config._retry = true;
        config.headers = config.headers ?? {};
        config.headers.Authorization = `Bearer ${newToken}`;
        return http.request(config as AxiosRequestConfig);
      } catch (refreshErr) {
        if (shouldResetSessionAfterRefreshFailure(refreshErr)) {
          useAuthStore.getState().logout();
          useAuthModalStore.getState().openLogin();
        }
        return Promise.reject(refreshErr);
      }
    }

    return Promise.reject(error);
  },
);
