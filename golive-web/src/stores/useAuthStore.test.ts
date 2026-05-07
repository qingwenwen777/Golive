// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest';

import {
  AUTH_STORAGE_KEY,
  scrubPersistedAuthToken,
  useAuthStore,
} from './useAuthStore';
import type { User } from '@/types/user';

function makeUser(): User {
  return {
    id: 'user-1',
    username: 'streamer',
    displayName: 'Streamer',
    avatar: '',
    coinBalance: 0,
    role: 'user',
    livePermissionStatus: 'approved',
  };
}

function readPersistedAuth() {
  const raw = window.localStorage.getItem(AUTH_STORAGE_KEY);
  expect(raw).toBeTruthy();
  return JSON.parse(raw ?? '{}') as {
    state?: Record<string, unknown>;
    version?: number;
  };
}

describe('useAuthStore persistence', () => {
  beforeEach(() => {
    window.localStorage.clear();
    useAuthStore.setState({
      token: null,
      user: null,
      hasHydrated: true,
    });
  });

  it('keeps the access token in memory while persisting only the user', () => {
    const user = makeUser();

    useAuthStore.getState().login({ token: 'access-token', user });

    expect(useAuthStore.getState().token).toBe('access-token');
    const persisted = readPersistedAuth();
    expect(persisted.state).toEqual({ user });
    expect(JSON.stringify(persisted)).not.toContain('access-token');
  });

  it('does not persist access tokens received from refresh', () => {
    useAuthStore.getState().setTokens('refreshed-access-token');

    expect(useAuthStore.getState().token).toBe('refreshed-access-token');
    const persisted = readPersistedAuth();
    expect(persisted.state?.token).toBeUndefined();
    expect(JSON.stringify(persisted)).not.toContain('refreshed-access-token');
  });

  it('scrubs legacy tokens that were previously written to localStorage', () => {
    window.localStorage.setItem(
      AUTH_STORAGE_KEY,
      JSON.stringify({
        state: { token: 'legacy-access-token', user: makeUser() },
        version: 0,
      }),
    );

    scrubPersistedAuthToken();

    const persisted = readPersistedAuth();
    expect(persisted.state?.token).toBeUndefined();
    expect(JSON.stringify(persisted)).not.toContain('legacy-access-token');
    expect((persisted.state?.user as User | undefined)?.id).toBe('user-1');
  });
});
