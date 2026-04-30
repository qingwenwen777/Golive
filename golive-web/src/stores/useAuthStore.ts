import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { LoginResp, User } from '@/types/user';

interface AuthState {
  token: string | null;
  refreshToken: string | null;
  user: User | null;
  hasHydrated: boolean;
  login: (resp: LoginResp) => void;
  logout: () => void;
  setUser: (u: User) => void;
  setTokens: (token: string, refreshToken: string) => void;
  setHasHydrated: (hasHydrated: boolean) => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      refreshToken: null,
      user: null,
      hasHydrated: false,
      login: (resp) =>
        set({ token: resp.token, refreshToken: resp.refreshToken, user: resp.user }),
      logout: () => set({ token: null, refreshToken: null, user: null }),
      setUser: (user) => set({ user }),
      setTokens: (token, refreshToken) => set({ token, refreshToken }),
      setHasHydrated: (hasHydrated) => set({ hasHydrated }),
    }),
    {
      name: 'golive-auth',
      partialize: (s) => ({ token: s.token, refreshToken: s.refreshToken, user: s.user }),
      onRehydrateStorage: () => (state) => {
        state?.setHasHydrated(true);
      },
    },
  ),
);

export const useIsAuthed = () =>
  useAuthStore((s) => Boolean(s.token) && Boolean(s.user));

export const useAuthHydrated = () => useAuthStore((s) => s.hasHydrated);
