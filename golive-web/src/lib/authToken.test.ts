// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { AuthRefreshError, refreshAuthToken } from './authToken';
import { clearCsrfToken } from '@/lib/csrfToken';
import { useAuthStore } from '@/stores/useAuthStore';

const axiosPostMock = vi.hoisted(() => vi.fn());
const axiosGetMock = vi.hoisted(() => vi.fn());

vi.mock('axios', () => ({
  default: {
    isAxiosError: (err: unknown) => Boolean((err as { isAxiosError?: boolean }).isAxiosError),
    get: axiosGetMock,
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
    clearCsrfToken();
    axiosPostMock.mockReset();
    axiosGetMock.mockReset();
    axiosGetMock.mockResolvedValue({ data: { token: 'csrf-token' } });
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

  it('deduplicates concurrent refresh requests and stores the returned access token', async () => {
    const request = deferred<{ data: { token: string } }>();
    axiosPostMock.mockReturnValueOnce(request.promise);

    const first = refreshAuthToken();
    const second = refreshAuthToken();

    await vi.waitFor(() => expect(axiosPostMock).toHaveBeenCalledTimes(1));
    expect(axiosPostMock).toHaveBeenCalledWith(
      '/api/auth/refresh',
      {},
      {
        timeout: 10_000,
        withCredentials: true,
        headers: { 'X-CSRF-Token': 'csrf-token' },
      },
    );

    request.resolve({ data: { token: 'new-token' } });

    await expect(Promise.all([first, second])).resolves.toEqual(['new-token', 'new-token']);
    expect(useAuthStore.getState().token).toBe('new-token');
  });

  it('clears the in-flight guard after failure so a later refresh can retry', async () => {
    axiosPostMock.mockRejectedValueOnce(new Error('refresh failed'));

    await expect(refreshAuthToken()).rejects.toMatchObject({
      kind: 'transient',
      message: 'refresh-failed',
    });

    axiosPostMock.mockResolvedValueOnce({
      data: { token: 'retry-token' },
    });

    await expect(refreshAuthToken()).resolves.toBe('retry-token');
    expect(axiosPostMock).toHaveBeenCalledTimes(2);
  });

  it('marks refresh 401 responses as unauthorized for session cleanup', async () => {
    axiosPostMock.mockRejectedValueOnce(axiosError(401));

    const refresh = refreshAuthToken();
    await expect(refresh).rejects.toBeInstanceOf(AuthRefreshError);
    await expect(refresh).rejects.toMatchObject({
      kind: 'unauthorized',
      status: 401,
    });
  });
});
