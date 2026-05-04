// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { AuthRefreshError, refreshAuthToken } from './authToken';
import { useAuthStore } from '@/stores/useAuthStore';

const axiosPostMock = vi.hoisted(() => vi.fn());

vi.mock('axios', () => ({
  default: {
    isAxiosError: (err: unknown) => Boolean((err as { isAxiosError?: boolean }).isAxiosError),
    post: axiosPostMock,
  },
}));

function axiosError(status: number) {
  return {
    isAxiosError: true,
    response: { status },
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe('refreshAuthToken', () => {
  beforeEach(() => {
    vi.stubEnv('VITE_API_BASE', '/api');
    window.localStorage.clear();
    axiosPostMock.mockReset();
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

  it('deduplicates concurrent refresh requests and stores the returned token pair', async () => {
    const request = deferred<{ data: { token: string; refreshToken: string } }>();
    axiosPostMock.mockReturnValueOnce(request.promise);

    const first = refreshAuthToken();
    const second = refreshAuthToken();

    expect(axiosPostMock).toHaveBeenCalledTimes(1);
    expect(axiosPostMock).toHaveBeenCalledWith(
      '/api/auth/refresh',
      { refreshToken: 'refresh-token' },
      { timeout: 10_000 },
    );

    request.resolve({ data: { token: 'new-token', refreshToken: 'new-refresh' } });

    await expect(Promise.all([first, second])).resolves.toEqual(['new-token', 'new-token']);
    expect(useAuthStore.getState().token).toBe('new-token');
    expect(useAuthStore.getState().refreshToken).toBe('new-refresh');
  });

  it('clears the in-flight guard after failure so a later refresh can retry', async () => {
    axiosPostMock.mockRejectedValueOnce(new Error('refresh failed'));

    await expect(refreshAuthToken()).rejects.toMatchObject({
      kind: 'transient',
      message: 'refresh-failed',
    });

    axiosPostMock.mockResolvedValueOnce({
      data: { token: 'retry-token', refreshToken: 'retry-refresh' },
    });

    await expect(refreshAuthToken()).resolves.toBe('retry-token');
    expect(axiosPostMock).toHaveBeenCalledTimes(2);
    expect(useAuthStore.getState().refreshToken).toBe('retry-refresh');
  });

  it('fails before calling the network when no refresh token is available', async () => {
    useAuthStore.setState({ refreshToken: null });

    await expect(refreshAuthToken()).rejects.toThrow('no-refresh-token');

    expect(axiosPostMock).not.toHaveBeenCalled();
  });

  it('marks refresh 401 responses as unauthorized for session cleanup', async () => {
    axiosPostMock.mockRejectedValueOnce(axiosError(401));

    const refresh = refreshAuthToken();
    await expect(refresh).rejects.toBeInstanceOf(AuthRefreshError);
    await expect(refresh).rejects.toMatchObject({
      kind: 'unauthorized',
      refreshToken: 'refresh-token',
      status: 401,
    });
  });

  it('adopts a newer token pair written by another tab after stale refresh 401', async () => {
    window.localStorage.setItem(
      'golive-auth',
      JSON.stringify({
        state: { token: 'tab-token', refreshToken: 'tab-refresh' },
        version: 0,
      }),
    );
    axiosPostMock.mockRejectedValueOnce(axiosError(401));

    await expect(refreshAuthToken()).resolves.toBe('tab-token');

    expect(useAuthStore.getState().token).toBe('tab-token');
    expect(useAuthStore.getState().refreshToken).toBe('tab-refresh');
  });
});
