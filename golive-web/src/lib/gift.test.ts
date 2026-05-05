import { describe, expect, it, vi } from 'vitest';

import { localizedGiftName } from './gift';

describe('localizedGiftName', () => {
  const gift = { id: 'rocket', name: 'Rocket', nameJa: 'Rocket JA' };

  it('uses the Japanese fallback only for Japanese locales', () => {
    expect(localizedGiftName(gift, 'ja-JP')).toBe('Rocket JA');
    expect(localizedGiftName(gift, 'en-US')).toBe('Rocket');
  });

  it('falls back to the default name when the Japanese name is blank', () => {
    expect(localizedGiftName({ ...gift, nameJa: '  ' }, 'ja-JP')).toBe('Rocket');
  });

  it('delegates to the translator with the resolved fallback', () => {
    const translate = vi.fn(
      (_key: string, options: { defaultValue: string }) => options.defaultValue,
    );

    expect(localizedGiftName(gift, 'ja', translate)).toBe('Rocket JA');
    expect(translate).toHaveBeenCalledWith('giftCatalog.names.rocket', {
      defaultValue: 'Rocket JA',
    });
  });
});
