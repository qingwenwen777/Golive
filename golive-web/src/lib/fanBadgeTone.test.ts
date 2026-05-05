import { describe, expect, it } from 'vitest';

import { fanBadgeToneClass } from './fanBadgeTone';

describe('fanBadgeToneClass', () => {
  it.each([
    [Number.NaN, 'is-tone-1'],
    [0, 'is-tone-1'],
    [5, 'is-tone-1'],
    [6, 'is-tone-2'],
    [20, 'is-tone-4'],
    [21, 'is-tone-5'],
    [99, 'is-tone-12'],
    [120, 'is-tone-12'],
  ])('maps level %s to %s', (level, expected) => {
    expect(fanBadgeToneClass(level)).toBe(expected);
  });
});
