import { describe, expect, it } from 'vitest';
import {
  USER_MAX_LEVEL,
  USER_MAX_TOPUP_COINS,
  createLevelInfo,
  levelProgressRatio,
  requiredCoinsForLevel,
} from './userLevel';

describe('userLevel', () => {
  it('keeps level 99 capped at one billion topup coins', () => {
    expect(USER_MAX_LEVEL).toBe(99);
    expect(requiredCoinsForLevel(1)).toBe(0);
    expect(requiredCoinsForLevel(USER_MAX_LEVEL)).toBe(USER_MAX_TOPUP_COINS);
    expect(createLevelInfo(USER_MAX_TOPUP_COINS)).toMatchObject({
      level: USER_MAX_LEVEL,
      coinsToNextLevel: 0,
      nextLevelTargetCoins: USER_MAX_TOPUP_COINS,
    });
  });

  it('reports progress toward the next level from total topup coins', () => {
    const level2Target = requiredCoinsForLevel(2);
    const info = createLevelInfo(level2Target - 1);

    expect(info.level).toBe(1);
    expect(info.coinsToNextLevel).toBe(1);
    expect(levelProgressRatio(info)).toBeGreaterThan(0);
    expect(levelProgressRatio(createLevelInfo(USER_MAX_TOPUP_COINS))).toBe(1);
  });
});
