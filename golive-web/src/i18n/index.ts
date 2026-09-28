import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';

import { formatCount } from '@/lib/format';

import commonEn from './locales/en-US/common.json';
import commonJa from './locales/ja-JP/common.json';
import commonZh from './locales/zh-CN/common.json';
import pagesEn from './locales/en-US/pages.json';
import pagesJa from './locales/ja-JP/pages.json';
import pagesZh from './locales/zh-CN/pages.json';

export const LANG_STORAGE_KEY = 'golive-lang';

export type AppLang = 'zh' | 'ja' | 'en';

const I18N_CODE: Record<AppLang, string> = {
  zh: 'zh-CN',
  en: 'en-US',
  ja: 'ja-JP',
};

export function toAppLang(code: string | undefined): AppLang {
  if (!code) return 'en';
  const lc = code.toLowerCase();
  if (lc.startsWith('zh')) return 'zh';
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
      'zh-CN': { common: commonZh, pages: pagesZh },
      // Aliases so detector results like `en` / `ja` / `zh` (or regional variants)
      // resolve without falling through to the missing-key string.
      en: { common: commonEn, pages: pagesEn },
      ja: { common: commonJa, pages: pagesJa },
      zh: { common: commonZh, pages: pagesZh },
    },
    fallbackLng: {
      'zh-CN': ['zh', 'en'],
      zh: ['en'],
      'ja-JP': ['ja', 'en'],
      ja: ['en'],
      'en-US': ['en'],
      default: ['en'],
    },
    defaultNS: 'common',
    ns: ['common', 'pages'],
    supportedLngs: ['zh-CN', 'ja-JP', 'en-US', 'zh', 'ja', 'en'],
    nonExplicitSupportedLngs: true,
    load: 'languageOnly',
    detection: {
      // Honor a saved preference; otherwise use English regardless of browser language.
      order: ['localStorage'],
      lookupLocalStorage: LANG_STORAGE_KEY,
      caches: ['localStorage'],
    },
    interpolation: { escapeValue: false },
    returnNull: false,
  });

// Number formats for translations: {{count, compact}} for audience sizes
// ("233K", "23万"); i18next's built-in {{count, number}} for exact figures
// ("12,480"). Both follow the UI language.
i18n.services.formatter?.add('compact', (value, lng) =>
  formatCount(Number(value), lng || undefined),
);

export default i18n;
