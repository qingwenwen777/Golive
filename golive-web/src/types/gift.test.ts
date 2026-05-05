import { describe, expect, it } from 'vitest';

import { SC_MAX_TEXT_BY_TIER, amountToTier } from './gift';

describe('gift helpers', () => {
  it.each([
    [0, 0],
    [199, 0],
    [200, 1],
    [999, 1],
    [1000, 2],
    [1999, 2],
    [2000, 3],
    [4999, 3],
    [5000, 4],
    [9999, 4],
    [10000, 5],
  ] as const)('maps amount %i to Super Chat tier %i', (amount, tier) => {
    expect(amountToTier(amount)).toBe(tier);
  });

  it('keeps text limits aligned with paid Super Chat tiers', () => {
    expect(SC_MAX_TEXT_BY_TIER).toEqual({
      0: 0,
      1: 50,
      2: 100,
      3: 150,
      4: 200,
      5: 200,
    });
  });
});
