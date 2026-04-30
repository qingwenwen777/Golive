import { create } from 'zustand';

type AfterLogin = () => void;

interface AuthModalState {
  open: boolean;
  afterLogin: AfterLogin | null;
  openLogin: (afterLogin?: AfterLogin) => void;
  close: () => void;
  consumeAfterLogin: () => void;
}

export const useAuthModalStore = create<AuthModalState>((set, get) => ({
  open: false,
  afterLogin: null,
  openLogin: (afterLogin) => set({ open: true, afterLogin: afterLogin ?? null }),
  close: () => set({ open: false, afterLogin: null }),
  consumeAfterLogin: () => {
    const cb = get().afterLogin;
    set({ afterLogin: null });
    if (cb) cb();
  },
}));
