import { describe, expect, it } from 'vitest';

import {
  formatCount,
  formatDate,
  formatDateTime,
  formatNumber,
  formatRelativeTime,
  toDate,
} from './format';

describe('formatNumber', () => {
  it('groups digits in the given locale and tolerates bad input', () => {
    expect(formatNumber(12480, 'en-US')).toBe('12,480');
    expect(formatNumber(1314000, 'ja-JP')).toBe('1,314,000');
    expect(formatNumber(Number.NaN, 'en-US')).toBe('0');
    expect(formatNumber(undefined, 'en-US')).toBe('0');
  });
});

describe('formatCount', () => {
  it('keeps small counts exact and compacts audience-sized ones', () => {
    expect(formatCount(9311, 'en-US')).toBe('9,311');
    expect(formatCount(12480, 'en-US')).toBe('12K');
    expect(formatCount(233000, 'en-US')).toBe('233K');
    expect(formatCount(1_234_567, 'en-US')).toBe('1.2M');
    expect(formatCount(12480, 'zh-CN')).toBe('1.2万');
    expect(formatCount(233000, 'ja-JP')).toBe('23万');
  });
});

describe('dates', () => {
  const now = new Date('2026-09-28T12:00:00Z');

  it('returns an empty string instead of throwing on bad timestamps', () => {
    expect(toDate('not a date')).toBeNull();
    expect(formatDateTime('not a date', 'en-US', now)).toBe('');
    expect(formatDate(undefined, 'en-US', now)).toBe('');
    expect(formatRelativeTime('', 'en-US', now)).toBe('');
  });

  it('adds the year only outside the current year', () => {
    expect(formatDate('2026-03-05T10:00:00Z', 'en-US', now)).toBe('Mar 5');
    expect(formatDate('2025-03-05T10:00:00Z', 'en-US', now)).toBe('Mar 5, 2025');
    expect(formatDateTime('2025-03-05T10:00:00Z', 'en-US', now)).toMatch(/^Mar 5, 2025/);
  });

  it('describes recent and upcoming times relatively', () => {
    const at = (ms: number) => new Date(now.getTime() + ms);
    expect(formatRelativeTime(at(-10_000), 'en-US', now)).toBe('now');
    expect(formatRelativeTime(at(-5 * 60_000), 'en-US', now)).toBe('5 minutes ago');
    expect(formatRelativeTime(at(3 * 3_600_000), 'en-US', now)).toBe('in 3 hours');
    expect(formatRelativeTime(at(-26 * 3_600_000), 'en-US', now)).toBe('yesterday');
    expect(formatRelativeTime(at(2 * 86_400_000), 'en-US', now)).toBe('in 2 days');
    // beyond a week it switches to a date
    expect(formatRelativeTime(at(-20 * 86_400_000), 'en-US', now)).toMatch(/^Sep 8/);
  });
});
