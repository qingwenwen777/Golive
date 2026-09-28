// @vitest-environment jsdom
import { cleanup, render } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { GiftArt } from './GiftArt';
import { builtInGiftKey } from './giftArt';

afterEach(cleanup);

describe('builtInGiftKey', () => {
  it('finds a built-in gift by id, English name or emoji', () => {
    expect(builtInGiftKey({ id: 'galaxy_ship' })).toBe('galaxy_ship');
    expect(builtInGiftKey({ name: 'Royal Crown', icon: '\u{1F451}' })).toBe('royal_crown');
    expect(builtInGiftKey({ name: '火箭', icon: '\u{1F680}' })).toBe('rocket');
    expect(builtInGiftKey({ icon: '\u{1F6E5}\uFE0F' })).toBe('yacht');
  });

  it('still recognises the ring the Royal Crown used to be', () => {
    expect(builtInGiftKey({ name: 'ロイヤルクラウン', icon: '\u{1F48D}' })).toBe('royal_crown');
  });

  it('leaves gifts outside the built-in set alone', () => {
    expect(
      builtInGiftKey({ id: 'custom_panda', name: 'Panda', icon: '\u{1F43C}' }),
    ).toBeUndefined();
  });
});

describe('GiftArt', () => {
  it('draws built-in gifts instead of printing their emoji', () => {
    const { container } = render(<GiftArt gift={{ id: 'royal_crown', icon: '\u{1F48D}' }} />);
    const art = container.querySelector('.gl-gift-art');
    expect(art?.querySelector('svg')).toBeTruthy();
    expect(art?.textContent).toBe('');
  });

  it("shows a newer gift's own emoji", () => {
    const { container } = render(<GiftArt gift={{ id: 'custom_panda', icon: '\u{1F43C}' }} />);
    expect(container.querySelector('.gl-gift-art.is-plain')?.textContent).toBe('\u{1F43C}');
  });
});
