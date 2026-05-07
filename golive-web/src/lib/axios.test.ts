// @vitest-environment jsdom
import {
  AxiosError,
  type AxiosAdapter,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { http } from './axios';
import { AuthRefreshError } from '@/lib/authToken';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useAuthStore } from '@/stores/useAuthStore';

const refreshAuthTokenMock = vi.hoisted(() => vi.fn());
const getCsrfTokenMock = vi.hoisted(() => vi.fn());

vi.mock('@/lib/authToken', async () => {
  const actual = await vi.importActual<typeof import('@/lib/authToken')>('@/lib/authToken');
  return {
    ...actual,
    refreshAuthToken: refreshAuthTokenMock,
  };
});

vi.mock('@/lib/csrfToken', async () => {
  const actual = await vi.importActual<typeof import('@/lib/csrfToken')>('@/lib/csrfToken');
  return {
    ...actual,
    getCsrfToken: getCsrfTokenMock,
  };
});

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
    getCsrfTokenMock.mockReset();
    getCsrfTokenMock.mockResolvedValue('csrf-token');
    useAuthModalStore.setState({ open: false, afterLogin: null });
    useAuthStore.setState({
      token: 'old-token',
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
    expect(getCsrfTokenMock).not.toHaveBeenCalled();
  });

  it('adds a CSRF token to mutating requests', async () => {
    const adapter = vi.fn<AxiosAdapter>(async (config) => response(config, { ok: true }));
    setAdapter(adapter);

    await http.post('/rooms/room-1/follow', {});

    expect(adapter).toHaveBeenCalledTimes(1);
    expect(headerValue(adapter.mock.calls[0][0], 'X-CSRF-Token')).toBe('csrf-token');
  });

  it('refreshes once and retries the original request with the new token after a 401', async () => {
    refreshAuthTokenMock.mockImplementationOnce(async () => {
      useAuthStore.getState().setTokens('new-token');
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

  it('logs out and opens the login modal when token refresh is unauthorized', async () => {
    refreshAuthTokenMock.mockRejectedValueOnce(
      new AuthRefreshError('refresh-failed', {
        kind: 'unauthorized',
        status: 401,
      }),
    );
    const adapter = vi.fn<AxiosAdapter>(async (config) => {
      throw unauthorized(config);
    });
    setAdapter(adapter);

    await expect(http.get('/rooms/room-1')).rejects.toThrow('refresh-failed');

    expect(adapter).toHaveBeenCalledTimes(1);
    expect(useAuthStore.getState().token).toBeNull();
    expect(useAuthStore.getState().user).toBeNull();
    expect(useAuthModalStore.getState().open).toBe(true);
  });

  it('keeps the session when token refresh fails transiently', async () => {
    refreshAuthTokenMock.mockRejectedValueOnce(
      new AuthRefreshError('refresh-failed', {
        kind: 'transient',
        status: 503,
      }),
    );
    const adapter = vi.fn<AxiosAdapter>(async (config) => {
      throw unauthorized(config);
    });
    setAdapter(adapter);

    await expect(http.get('/rooms/room-1')).rejects.toThrow('refresh-failed');

    expect(adapter).toHaveBeenCalledTimes(1);
    expect(useAuthStore.getState().token).toBe('old-token');
    expect(useAuthStore.getState().user?.id).toBe('user-1');
    expect(useAuthModalStore.getState().open).toBe(false);
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
