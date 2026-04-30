import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';

import commonEn from './locales/en-US/common.json';
import commonJa from './locales/ja-JP/common.json';
import pagesEn from './locales/en-US/pages.json';
import pagesJa from './locales/ja-JP/pages.json';

export const LANG_STORAGE_KEY = 'golive-lang';

export type AppLang = 'en' | 'ja';

const I18N_CODE: Record<AppLang, string> = {
  en: 'en-US',
  ja: 'ja-JP',
};

export function toAppLang(code: string | undefined): AppLang {
  if (!code) return 'en';
  const lc = code.toLowerCase();
  if (lc.startsWith('ja')) return 'ja';
  return 'en';
}

export function appLangToI18nCode(lang: AppLang): string {
  return I18N_CODE[lang];
}

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      'en-US': { common: commonEn, pages: pagesEn },
      'ja-JP': { common: commonJa, pages: pagesJa },
      // Aliases so detector results like `en` / `ja` (or any en-XX/ja-XX)
      // resolve without falling through to the missing-key string.
      en: { common: commonEn, pages: pagesEn },
      ja: { common: commonJa, pages: pagesJa },
    },
    fallbackLng: { 'ja-JP': ['ja', 'en'], ja: ['en'], 'en-US': ['en'], default: ['en'] },
    defaultNS: 'common',
    ns: ['common', 'pages'],
    supportedLngs: ['en-US', 'ja-JP', 'en', 'ja'],
    nonExplicitSupportedLngs: true,
    load: 'languageOnly',
    detection: {
      order: ['localStorage', 'navigator'],
      lookupLocalStorage: LANG_STORAGE_KEY,
      caches: ['localStorage'],
    },
    interpolation: { escapeValue: false },
    returnNull: false,
  });

export default i18n;
