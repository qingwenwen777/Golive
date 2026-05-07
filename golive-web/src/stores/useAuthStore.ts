import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { LoginResp, User } from '@/types/user';

export const AUTH_STORAGE_KEY = 'golive-auth';

interface AuthState {
  token: string | null;
  user: User | null;
  hasHydrated: boolean;
  login: (resp: LoginResp) => void;
  logout: () => void;
  setUser: (u: User) => void;
  setTokens: (token: string) => void;
  setHasHydrated: (hasHydrated: boolean) => void;
}

type PersistedAuthState = Partial<Pick<AuthState, 'user'>>;

export function scrubPersistedAuthToken() {
  if (typeof window === 'undefined') return;

  try {
    const raw = window.localStorage.getItem(AUTH_STORAGE_KEY);
    if (!raw) return;

    const payload = JSON.parse(raw) as {
      state?: Record<string, unknown> | null;
      version?: unknown;
    };
    if (!payload.state || !('token' in payload.state)) return;

    const { token: _token, ...stateWithoutToken } = payload.state;
    window.localStorage.setItem(
      AUTH_STORAGE_KEY,
      JSON.stringify({ ...payload, state: stateWithoutToken }),
    );
  } catch {
    // Ignore malformed legacy storage. Zustand will overwrite it on the next state change.
  }
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      hasHydrated: false,
      login: (resp) => set({ token: resp.token, user: resp.user }),
      logout: () => set({ token: null, user: null }),
      setUser: (user) => set({ user }),
      setTokens: (token) => set({ token }),
      setHasHydrated: (hasHydrated) => set({ hasHydrated }),
    }),
    {
      name: AUTH_STORAGE_KEY,
      partialize: (s): PersistedAuthState => ({ user: s.user }),
      merge: (persisted, current) => {
        const persistedState = persisted as PersistedAuthState | undefined;
        return {
          ...current,
          user: persistedState?.user ?? null,
          token: null,
          hasHydrated: current.hasHydrated,
        };
      },
      onRehydrateStorage: () => (state) => {
        scrubPersistedAuthToken();
        state?.setHasHydrated(true);
      },
    },
  ),
);

export const useIsAuthed = () =>
  useAuthStore((s) => Boolean(s.token) && Boolean(s.user));

export const useAuthHydrated = () => useAuthStore((s) => s.hasHydrated);
