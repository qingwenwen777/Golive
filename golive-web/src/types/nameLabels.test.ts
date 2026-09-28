// @vitest-environment jsdom
import { afterAll, describe, expect, it, vi } from 'vitest';

import i18n from '@/i18n';
import commonJa from '@/i18n/locales/ja-JP/common.json';
import commonZh from '@/i18n/locales/zh-CN/common.json';

import { creatorName, guestName, isPlaceholderName, personName, userDisplayName } from './user';

describe('name labels', () => {
  afterAll(() => i18n.changeLanguage('en-US'));

  it('follow the UI language, and are never taken for a name', async () => {
    await vi.waitFor(() => expect(i18n.isInitialized).toBe(true));

    await i18n.changeLanguage('zh-CN');
    expect(personName('')).toBe(commonZh.names.unknownUser);
    expect(creatorName('Creator 550e8400')).toBe(commonZh.names.unknownCreator);
    expect(isPlaceholderName(commonZh.names.unknownCreator)).toBe(true);

    await i18n.changeLanguage('ja-JP');
    expect(guestName()).toBe(commonJa.names.guest);
    expect(userDisplayName(null)).toBe(commonJa.account.you);
    expect(isPlaceholderName(commonJa.names.unknownUser)).toBe(true);
  });
});
