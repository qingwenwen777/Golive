import { create } from 'zustand';
import i18n, { appLangToI18nCode, LANG_STORAGE_KEY, toAppLang, type AppLang } from '@/i18n';

function readStoredLang(): AppLang {
  if (typeof window === 'undefined') return 'en';
  const stored = window.localStorage.getItem(LANG_STORAGE_KEY);
  if (stored) return toAppLang(stored);
  return toAppLang(i18n.language);
}

interface LangState {
  lang: AppLang;
  setLang: (lang: AppLang) => void;
  toggleLang: () => void;
}

export const useLangStore = create<LangState>((set, get) => ({
  lang: readStoredLang(),
  setLang: (lang) => {
    const code = appLangToI18nCode(lang);
    void i18n.changeLanguage(code);
    window.localStorage.setItem(LANG_STORAGE_KEY, code);
    set({ lang });
  },
  toggleLang: () => {
    const next: AppLang = get().lang === 'en' ? 'ja' : 'en';
    get().setLang(next);
  },
}));

export function bootstrapLang(): void {
  const lang = readStoredLang();
  const code = appLangToI18nCode(lang);
  if (i18n.language !== code) void i18n.changeLanguage(code);
}
