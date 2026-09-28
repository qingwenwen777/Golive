// @vitest-environment jsdom
import { beforeAll, describe, expect, it, vi } from 'vitest';

import type { i18n as I18n } from 'i18next';

let i18n: I18n;

beforeAll(async () => {
  window.localStorage.clear();
  ({ default: i18n } = await import('./index'));
  await vi.waitFor(() => expect(i18n.isInitialized).toBe(true));
});

describe('count formatting in translations', () => {
  it('compacts audience sizes and picks the singular in English', async () => {
    await i18n.changeLanguage('en-US');
    const t = i18n.getFixedT('en-US', 'pages');
    expect(t('liveRoom.subscribers', { count: 233000 })).toBe('233K subscribers');
    expect(t('liveRoom.subscribers', { count: 9311 })).toBe('9,311 subscribers');
    expect(t('liveRoom.subscribers', { count: 1 })).toBe('1 subscriber');
    expect(t('home.watching', { count: 12480 })).toBe('12K watching');
    expect(t('search.views', { count: 1 })).toBe('1 view');
    expect(t('posts.comments.expandReplies', { count: 1 })).toBe('Show 1 reply');
  });

  it('keeps the live room viewer count exact', async () => {
    const t = i18n.getFixedT('en-US', 'pages');
    expect(t('liveRoom.watching', { count: 12480 })).toBe('12,480 watching');
  });

  it('uses the UI language for compact numbers', async () => {
    await i18n.changeLanguage('zh-CN');
    expect(i18n.getFixedT('zh-CN', 'pages')('liveRoom.subscribers', { count: 233000 })).toBe(
      '23万 位订阅者',
    );
    await i18n.changeLanguage('ja-JP');
    expect(i18n.getFixedT('ja-JP', 'pages')('home.watching', { count: 12480 })).toMatch(/^1\.2万/);
    await i18n.changeLanguage('en-US');
  });
});
