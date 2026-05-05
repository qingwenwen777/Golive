// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from 'vitest';

import {
  COIN_ACTIVITY_KEY_PREFIX,
  addDailyCoinWatchSeconds,
  coinTodayKey,
  markDailyCoinRoomWatched,
  readDailyCoinActivity,
} from './coinActivity';

const storageKey = `${COIN_ACTIVITY_KEY_PREFIX}:user-1`;

describe('coinActivity', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('uses the Beijing calendar day for daily task keys', () => {
    expect(coinTodayKey(new Date('2026-05-04T15:59:59.000Z'))).toBe('2026-05-04');
    expect(coinTodayKey(new Date('2026-05-04T16:00:00.000Z'))).toBe('2026-05-05');
  });

  it('falls back when storage is missing, stale, malformed, or anonymous', () => {
    expect(readDailyCoinActivity(null)).toEqual({
      date: coinTodayKey(),
      watchedRoomIds: [],
      watchSeconds: 0,
    });

    window.localStorage.setItem(storageKey, '{bad json');
    expect(readDailyCoinActivity('user-1')).toMatchObject({
      watchedRoomIds: [],
      watchSeconds: 0,
    });

    window.localStorage.setItem(
      storageKey,
      JSON.stringify({ date: '2000-01-01', watchedRoomIds: ['old'], watchSeconds: 120 }),
    );
    expect(readDailyCoinActivity('user-1')).toMatchObject({
      date: coinTodayKey(),
      watchedRoomIds: [],
      watchSeconds: 0,
    });
  });

  it('sanitizes stored room ids and watch seconds', () => {
    window.localStorage.setItem(
      storageKey,
      JSON.stringify({
        date: coinTodayKey(),
        watchedRoomIds: ['room-1', 2, '', 'room-2'],
        watchSeconds: Number.POSITIVE_INFINITY,
      }),
    );

    expect(readDailyCoinActivity('user-1')).toEqual({
      date: coinTodayKey(),
      watchedRoomIds: ['room-1', '', 'room-2'],
      watchSeconds: 0,
    });
  });

  it('records watched rooms once and caps daily watch seconds at one day', () => {
    markDailyCoinRoomWatched('user-1', 'room-1');
    markDailyCoinRoomWatched('user-1', 'room-1');
    markDailyCoinRoomWatched('user-1', 'room-2');
    markDailyCoinRoomWatched('user-1', '');

    addDailyCoinWatchSeconds('user-1', 30);
    addDailyCoinWatchSeconds('user-1', -10);
    addDailyCoinWatchSeconds('user-1', 100_000);

    expect(readDailyCoinActivity('user-1')).toEqual({
      date: coinTodayKey(),
      watchedRoomIds: ['room-1', 'room-2'],
      watchSeconds: 24 * 60 * 60,
    });
  });
});
