import axios from 'axios';

import { useAuthStore } from '@/stores/useAuthStore';
import { CSRF_HEADER, clearCsrfToken, getCsrfToken } from '@/lib/csrfToken';

export type AuthRefreshFailureKind =
  | 'unauthorized'
  | 'transient';

export class AuthRefreshError extends Error {
  readonly kind: AuthRefreshFailureKind;
  readonly status: number | null;

  constructor(
    message: string,
    opts: {
      kind: AuthRefreshFailureKind;
      status?: number | null;
      cause?: unknown;
    },
  ) {
    super(message);
    this.name = 'AuthRefreshError';
    this.kind = opts.kind;
    this.status = opts.status ?? null;
    if (opts.cause !== undefined) {
      this.cause = opts.cause;
    }
  }
}

export function isSessionInvalidAfterRefreshFailure(
  err: unknown,
): boolean {
  if (!(err instanceof AuthRefreshError)) return false;
  return err.kind === 'unauthorized';
}

export function getAuthToken(): string | null {
  return useAuthStore.getState().token;
}

let refreshPromise: Promise<string> | null = null;

export async function refreshAuthToken(): Promise<string> {
  if (refreshPromise) return refreshPromise;

  refreshPromise = (async () => {
    try {
      const { data } = await postRefresh();
      useAuthStore.getState().setTokens(data.token);
      return data.token;
    } catch (err) {
      if (axios.isAxiosError(err)) {
        const status = err.response?.status ?? null;
        throw new AuthRefreshError('refresh-failed', {
          kind: status === 401 || status === 403 ? 'unauthorized' : 'transient',
          status,
          cause: err,
        });
      }
      throw new AuthRefreshError('refresh-failed', {
        kind: 'transient',
        cause: err,
      });
    }
  })().finally(() => {
    refreshPromise = null;
  });

  return refreshPromise;
}

async function postRefresh() {
  for (let attempt = 0; attempt < 2; attempt += 1) {
    const csrf = await getCsrfToken(attempt > 0);
    try {
      return await axios.post<{ token: string }>(
        `${import.meta.env.VITE_API_BASE}/auth/refresh`,
        {},
        {
          timeout: 10_000,
          withCredentials: true,
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
  });
}

function isCsrfInvalid(err: unknown): boolean {
  return (
    axios.isAxiosError(err) &&
    err.response?.status === 403 &&
    (err.response.data as { reason?: unknown } | undefined)?.reason === 'csrf_invalid'
  );
}
