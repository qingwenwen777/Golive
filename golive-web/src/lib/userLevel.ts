import type { UserLevelInfo } from '@/types/user';

export const USER_MAX_LEVEL = 99;
export const USER_MAX_TOPUP_COINS = 1_000_000_000;

export function requiredCoinsForLevel(level: number): number {
  if (level <= 1) return 0;
  if (level >= USER_MAX_LEVEL) return USER_MAX_TOPUP_COINS;
  const step = Math.floor(level) - 1;
  const maxStep = USER_MAX_LEVEL - 1;
  return Math.ceil((USER_MAX_TOPUP_COINS * step * step * step) / (maxStep * maxStep * maxStep));
}

export function levelForTotalTopup(totalTopupCoins: number): number {
  const total = Math.max(0, Math.floor(totalTopupCoins));
  if (total >= USER_MAX_TOPUP_COINS) return USER_MAX_LEVEL;
  let level = 1;
  for (let next = 2; next <= USER_MAX_LEVEL; next += 1) {
    if (total < requiredCoinsForLevel(next)) break;
    level = next;
  }
  return level;
}

export function createLevelInfo(totalTopupCoins: number): UserLevelInfo {
  const total = Math.max(0, Math.floor(totalTopupCoins));
  const level = levelForTotalTopup(total);
  const currentLevelMinCoins = requiredCoinsForLevel(level);
  const nextLevelTargetCoins =
    level >= USER_MAX_LEVEL ? USER_MAX_TOPUP_COINS : requiredCoinsForLevel(level + 1);
  return {
    level,
    maxLevel: USER_MAX_LEVEL,
    totalTopupCoins: total,
    currentLevelMinCoins,
    nextLevelTargetCoins,
    coinsToNextLevel: level >= USER_MAX_LEVEL ? 0 : Math.max(0, nextLevelTargetCoins - total),
  };
}

export function normalizeLevelInfo(info?: UserLevelInfo | null): UserLevelInfo {
  return info ?? createLevelInfo(0);
}

export function levelProgressRatio(info?: UserLevelInfo | null): number {
  const levelInfo = normalizeLevelInfo(info);
  if (levelInfo.level >= levelInfo.maxLevel) return 1;
  const span = levelInfo.nextLevelTargetCoins - levelInfo.currentLevelMinCoins;
  if (span <= 0) return 0;
  return Math.max(
    0,
    Math.min(1, (levelInfo.totalTopupCoins - levelInfo.currentLevelMinCoins) / span),
  );
}

export function userLevelToneClass(level: number): string {
  const safeLevel = Number.isFinite(level) ? Math.max(1, Math.min(99, Math.floor(level))) : 1;
  const tone = safeLevel <= 20 ? Math.ceil(safeLevel / 5) : 4 + Math.ceil((safeLevel - 20) / 10);
  return `is-tone-${tone}`;
}
