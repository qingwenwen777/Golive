import axios from 'axios';

import { useAuthStore } from '@/stores/useAuthStore';
import { CSRF_HEADER, clearCsrfToken, getCsrfToken } from '@/lib/csrfToken';

export type AuthRefreshFailureKind =
  | 'missing-refresh-token'
  | 'unauthorized'
  | 'transient';

export class AuthRefreshError extends Error {
  readonly kind: AuthRefreshFailureKind;
  readonly refreshToken: string | null;
  readonly status: number | null;

  constructor(
    message: string,
    opts: {
      kind: AuthRefreshFailureKind;
      refreshToken: string | null;
      status?: number | null;
      cause?: unknown;
    },
  ) {
    super(message);
    this.name = 'AuthRefreshError';
    this.kind = opts.kind;
    this.refreshToken = opts.refreshToken;
    this.status = opts.status ?? null;
    if (opts.cause !== undefined) {
      this.cause = opts.cause;
    }
  }
}

export function isSessionInvalidAfterRefreshFailure(
  err: unknown,
  currentRefreshToken: string | null,
): boolean {
  if (!(err instanceof AuthRefreshError)) return false;
  if (err.kind === 'missing-refresh-token') return true;
  if (err.kind !== 'unauthorized') return false;
  return !err.refreshToken || currentRefreshToken === err.refreshToken;
}

export function getAuthToken(): string | null {
  return useAuthStore.getState().token;
}

export function getRefreshToken(): string | null {
  return useAuthStore.getState().refreshToken;
}

function readPersistedTokenPair(): { token: string; refreshToken: string } | null {
  if (typeof window === 'undefined') return null;
  try {
    const raw = window.localStorage.getItem('golive-auth');
    if (!raw) return null;
    const parsed = JSON.parse(raw) as {
      state?: { token?: unknown; refreshToken?: unknown };
    };
    const token = parsed.state?.token;
    const refreshToken = parsed.state?.refreshToken;
    if (typeof token === 'string' && typeof refreshToken === 'string') {
      return { token, refreshToken };
    }
  } catch {
    return null;
  }
  return null;
}

let refreshPromise: Promise<string> | null = null;

export async function refreshAuthToken(): Promise<string> {
  if (refreshPromise) return refreshPromise;

  refreshPromise = (async () => {
    const refreshToken = getRefreshToken();
    if (!refreshToken) {
      throw new AuthRefreshError('no-refresh-token', {
        kind: 'missing-refresh-token',
        refreshToken: null,
      });
    }

    try {
      const { data } = await postRefresh(refreshToken);
      useAuthStore.getState().setTokens(data.token, data.refreshToken);
      return data.token;
    } catch (err) {
      if (axios.isAxiosError(err)) {
        const status = err.response?.status ?? null;
        if (status === 401 || status === 403) {
          const persisted = readPersistedTokenPair();
          if (persisted && persisted.refreshToken !== refreshToken) {
            useAuthStore.getState().setTokens(persisted.token, persisted.refreshToken);
            return persisted.token;
          }
        }
        throw new AuthRefreshError('refresh-failed', {
          kind: status === 401 || status === 403 ? 'unauthorized' : 'transient',
          refreshToken,
          status,
          cause: err,
        });
      }
      throw new AuthRefreshError('refresh-failed', {
        kind: 'transient',
        refreshToken,
        cause: err,
      });
    }
  })().finally(() => {
    refreshPromise = null;
  });

  return refreshPromise;
}

async function postRefresh(refreshToken: string) {
  for (let attempt = 0; attempt < 2; attempt += 1) {
    const csrf = await getCsrfToken(attempt > 0);
    try {
      return await axios.post<{ token: string; refreshToken: string }>(
        `${import.meta.env.VITE_API_BASE}/auth/refresh`,
        { refreshToken },
        {
          timeout: 10_000,
          headers: { [CSRF_HEADER]: csrf },
        },
      );
    } catch (err) {
      if (!isCsrfInvalid(err) || attempt > 0) throw err;
      clearCsrfToken();
    }
  }
  throw new AuthRefreshError('refresh-failed', {
    kind: 'transient',
    refreshToken,
  });
}

function isCsrfInvalid(err: unknown): boolean {
  return (
    axios.isAxiosError(err) &&
    err.response?.status === 403 &&
    (err.response.data as { reason?: unknown } | undefined)?.reason === 'csrf_invalid'
  );
}
