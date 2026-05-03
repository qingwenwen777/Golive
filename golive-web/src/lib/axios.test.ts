// @vitest-environment jsdom
import {
  AxiosError,
  type AxiosAdapter,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { http } from './axios';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useAuthStore } from '@/stores/useAuthStore';

const refreshAuthTokenMock = vi.hoisted(() => vi.fn());

vi.mock('@/lib/authToken', () => ({
  refreshAuthToken: refreshAuthTokenMock,
}));

function response<T>(config: InternalAxiosRequestConfig, data: T): AxiosResponse<T> {
  return {
    data,
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  };
}

function unauthorized(config: InternalAxiosRequestConfig) {
  return new AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, undefined, {
    data: { message: 'Unauthorized' },
    status: 401,
    statusText: 'Unauthorized',
    headers: {},
    config,
  });
}

function headerValue(config: InternalAxiosRequestConfig, name: string): string | undefined {
  const headers = config.headers as unknown as {
    get?: (header: string) => string | undefined;
  };
  const record = headers as Record<string, string | undefined>;
  return headers.get?.(name) ?? record[name];
}

function setAdapter(adapter: AxiosAdapter) {
  http.defaults.adapter = adapter;
}

describe('http axios client', () => {
  beforeEach(() => {
    vi.stubEnv('VITE_API_BASE', '/api');
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    refreshAuthTokenMock.mockReset();
    useAuthModalStore.setState({ open: false, afterLogin: null });
    useAuthStore.setState({
      token: 'old-token',
      refreshToken: 'refresh-token',
      user: {
        id: 'user-1',
        username: 'streamer',
        displayName: 'Streamer',
        avatar: '',
        coinBalance: 0,
        role: 'user',
        livePermissionStatus: 'approved',
      },
      hasHydrated: true,
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('adds the current bearer token to outgoing requests', async () => {
    const adapter = vi.fn<AxiosAdapter>(async (config) => response(config, { ok: true }));
    setAdapter(adapter);

    await http.get('/rooms');

    expect(adapter).toHaveBeenCalledTimes(1);
    expect(headerValue(adapter.mock.calls[0][0], 'Authorization')).toBe('Bearer old-token');
  });

  it('refreshes once and retries the original request with the new token after a 401', async () => {
    refreshAuthTokenMock.mockImplementationOnce(async () => {
      useAuthStore.getState().setTokens('new-token', 'new-refresh');
      return 'new-token';
    });
    const adapter = vi.fn<AxiosAdapter>(async (config) => {
      if (adapter.mock.calls.length === 1) throw unauthorized(config);
      return response(config, { ok: true });
    });
    setAdapter(adapter);

    await expect(http.get('/rooms/room-1')).resolves.toMatchObject({
      data: { ok: true },
    });

    expect(refreshAuthTokenMock).toHaveBeenCalledTimes(1);
    expect(adapter).toHaveBeenCalledTimes(2);
    expect(headerValue(adapter.mock.calls[1][0], 'Authorization')).toBe('Bearer new-token');
    expect(
      (adapter.mock.calls[1][0] as InternalAxiosRequestConfig & { _retry?: boolean })._retry,
    ).toBe(true);
  });

  it('logs out and opens the login modal when token refresh fails', async () => {
    refreshAuthTokenMock.mockRejectedValueOnce(new Error('refresh down'));
    const adapter = vi.fn<AxiosAdapter>(async (config) => {
      throw unauthorized(config);
    });
    setAdapter(adapter);

    await expect(http.get('/rooms/room-1')).rejects.toThrow('refresh down');

    expect(adapter).toHaveBeenCalledTimes(1);
    expect(useAuthStore.getState().token).toBeNull();
    expect(useAuthStore.getState().refreshToken).toBeNull();
    expect(useAuthStore.getState().user).toBeNull();
    expect(useAuthModalStore.getState().open).toBe(true);
  });

  it('does not try to refresh auth endpoint failures', async () => {
    const adapter = vi.fn<AxiosAdapter>(async (config) => {
      throw unauthorized(config);
    });
    setAdapter(adapter);

    await expect(http.post('/auth/login', {})).rejects.toThrow('Unauthorized');

    expect(refreshAuthTokenMock).not.toHaveBeenCalled();
    expect(adapter).toHaveBeenCalledTimes(1);
  });
});
