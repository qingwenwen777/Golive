import { create } from 'zustand';

export type Theme = 'light' | 'dark';

const STORAGE_KEY = 'golive-theme';
// Set when the user picks a theme. Older builds saved "light" on every first
// visit, so a stored "light" without this flag is not treated as a choice.
const EXPLICIT_KEY = 'golive-theme-explicit';
const THEME_COLOR: Record<Theme, string> = { light: '#ffffff', dark: '#0f0f0f' };
const DARK_QUERY = '(prefers-color-scheme: dark)';

// The same rules run before first paint in src/theme-init.js; keep them in sync.
export function readStoredTheme(): Theme | null {
  if (typeof window === 'undefined') return null;
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    const explicit = window.localStorage.getItem(EXPLICIT_KEY) === '1';
    if (stored === 'dark' || (stored === 'light' && explicit)) return stored;
  } catch {
    /* storage unavailable */
  }
  return null;
}

function systemTheme(): Theme {
  if (typeof window === 'undefined' || !window.matchMedia) return 'light';
  return window.matchMedia(DARK_QUERY).matches ? 'dark' : 'light';
}

export function resolveTheme(): Theme {
  return readStoredTheme() ?? systemTheme();
}

function applyTheme(theme: Theme): void {
  const root = document.documentElement;
  root.classList.toggle('dark', theme === 'dark');
  root.style.colorScheme = theme;
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', THEME_COLOR[theme]);
}

function persistTheme(theme: Theme): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, theme);
    window.localStorage.setItem(EXPLICIT_KEY, '1');
  } catch {
    /* storage unavailable: the choice lasts for this page only */
  }
}

interface ThemeState {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  toggleTheme: () => void;
}

export const useThemeStore = create<ThemeState>((set, get) => ({
  theme: resolveTheme(),
  setTheme: (theme) => {
    persistTheme(theme);
    applyTheme(theme);
    set({ theme });
  },
  toggleTheme: () => {
    const next: Theme = get().theme === 'dark' ? 'light' : 'dark';
    persistTheme(next);
    applyTheme(next);
    set({ theme: next });
  },
}));

export function bootstrapTheme(): void {
  applyTheme(useThemeStore.getState().theme);
  // Until the user picks a theme, follow the system setting as it changes.
  const media = window.matchMedia?.(DARK_QUERY);
  media?.addEventListener?.('change', (event) => {
    if (readStoredTheme()) return;
    const theme: Theme = event.matches ? 'dark' : 'light';
    applyTheme(theme);
    useThemeStore.setState({ theme });
  });
}
