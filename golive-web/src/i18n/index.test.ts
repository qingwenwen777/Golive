// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import commonEn from './locales/en-US/common.json';
import commonJa from './locales/ja-JP/common.json';
import commonZh from './locales/zh-CN/common.json';

async function startLanguage() {
  const { default: i18n } = await import('./index');
  await vi.waitFor(() => expect(i18n.isInitialized).toBe(true));
  const { bootstrapLang, useLangStore } = await import('@/stores/useLangStore');
  bootstrapLang();
  return { i18n, useLangStore };
}

describe('language preferences', () => {
  beforeEach(() => {
    vi.resetModules();
    window.localStorage.clear();
    document.documentElement.lang = '';
  });

  afterEach(() => {
    vi.restoreAllMocks();
    window.localStorage.clear();
    document.documentElement.lang = '';
  });

  it.each(['zh-CN', 'ja-JP', 'en-US'])(
    'defaults to English on a first visit with a %s browser',
    async (browserLanguage) => {
      vi.spyOn(window.navigator, 'language', 'get').mockReturnValue(browserLanguage);
      vi.spyOn(window.navigator, 'languages', 'get').mockReturnValue([browserLanguage]);

      const { i18n, useLangStore } = await startLanguage();

      expect(useLangStore.getState().lang).toBe('en');
      expect(i18n.t('search')).toBe(commonEn.search);
      expect(document.documentElement.lang).toBe('en-US');
    },
  );

  it.each([
    ['zh', 'zh-CN', commonZh.search],
    ['ja', 'ja-JP', commonJa.search],
    ['en', 'en-US', commonEn.search],
  ] as const)('restores a saved %s preference', async (lang, code, searchLabel) => {
    window.localStorage.setItem('golive-lang', code);

    const { i18n, useLangStore } = await startLanguage();

    expect(useLangStore.getState().lang).toBe(lang);
    expect(i18n.t('search')).toBe(searchLabel);
    expect(document.documentElement.lang).toBe(code);
  });

  it('keeps all languages selectable and remembers the choice after a reload', async () => {
    const { i18n, useLangStore } = await startLanguage();

    for (const [lang, code, searchLabel] of [
      ['ja', 'ja-JP', commonJa.search],
      ['en', 'en-US', commonEn.search],
      ['zh', 'zh-CN', commonZh.search],
    ] as const) {
      useLangStore.getState().setLang(lang);
      expect(i18n.t('search')).toBe(searchLabel);
      expect(window.localStorage.getItem('golive-lang')).toBe(code);
      expect(document.documentElement.lang).toBe(code);
    }

    vi.resetModules();
    const restored = await startLanguage();
    expect(restored.useLangStore.getState().lang).toBe('zh');
    expect(restored.i18n.t('search')).toBe(commonZh.search);
  });

  it('falls back to English for an unsupported saved language', async () => {
    window.localStorage.setItem('golive-lang', 'fr-FR');

    const { i18n, useLangStore } = await startLanguage();

    expect(useLangStore.getState().lang).toBe('en');
    expect(i18n.t('search')).toBe(commonEn.search);
    expect(document.documentElement.lang).toBe('en-US');
  });
});
