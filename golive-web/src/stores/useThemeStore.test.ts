// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

function mockSystemDark(dark: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: query === '(prefers-color-scheme: dark)' ? dark : false,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  })) as unknown as typeof window.matchMedia;
}

async function loadStore() {
  vi.resetModules();
  return import('./useThemeStore');
}

beforeEach(() => {
  localStorage.clear();
  document.documentElement.className = '';
  document.head.innerHTML = '<meta name="theme-color" content="#ffffff" />';
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('theme resolution', () => {
  it('follows the system setting when the user has not chosen', async () => {
    mockSystemDark(true);
    const { resolveTheme } = await loadStore();
    expect(resolveTheme()).toBe('dark');
    mockSystemDark(false);
    expect(resolveTheme()).toBe('light');
  });

  it('ignores the "light" that older builds saved automatically', async () => {
    mockSystemDark(true);
    localStorage.setItem('golive-theme', 'light');
    const { resolveTheme } = await loadStore();
    expect(resolveTheme()).toBe('dark');
  });

  it('keeps an explicit choice over the system setting', async () => {
    mockSystemDark(true);
    localStorage.setItem('golive-theme', 'light');
    localStorage.setItem('golive-theme-explicit', '1');
    const { resolveTheme } = await loadStore();
    expect(resolveTheme()).toBe('light');

    mockSystemDark(false);
    localStorage.setItem('golive-theme', 'dark');
    expect(resolveTheme()).toBe('dark');
  });

  it('records the choice and updates the page when the user toggles', async () => {
    mockSystemDark(false);
    const { useThemeStore, bootstrapTheme } = await loadStore();
    bootstrapTheme();
    expect(document.documentElement.classList.contains('dark')).toBe(false);

    useThemeStore.getState().toggleTheme();
    expect(useThemeStore.getState().theme).toBe('dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    expect(document.documentElement.style.colorScheme).toBe('dark');
    expect(document.querySelector('meta[name="theme-color"]')?.getAttribute('content')).toBe('#0f0f0f');
    expect(localStorage.getItem('golive-theme')).toBe('dark');
    expect(localStorage.getItem('golive-theme-explicit')).toBe('1');
  });
});
