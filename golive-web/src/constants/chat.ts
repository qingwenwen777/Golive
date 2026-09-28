import type { SuperChatTier } from '@/types/message';

export interface SuperChatTierSpec {
  tier: SuperChatTier;
  bg: string;
  soft: string;
}

export const SC_TIERS: SuperChatTierSpec[] = [
  { tier: 0, bg: '#1e88e5', soft: '#90caf9' },
  { tier: 1, bg: '#00acc1', soft: '#80deea' },
  { tier: 2, bg: '#43a047', soft: '#a5d6a7' },
  { tier: 3, bg: '#f9a825', soft: '#ffe082' },
  { tier: 4, bg: '#f4511e', soft: '#ffab91' },
  { tier: 5, bg: '#e91e63', soft: '#f48fb1' },
];

export const SC_TIER_MAX: SuperChatTierSpec = {
  tier: 5,
  bg: '#e91e63',
  soft: '#f48fb1',
};

export function tierSpec(tier: SuperChatTier): SuperChatTierSpec {
  return SC_TIERS.find((t) => t.tier === tier) ?? SC_TIER_MAX;
}

export const DANMU_COLORS = ['#ffffff', '#ff6b6b', '#6ec1ff', '#ffde5f', '#7dff9b', '#c59cff'];

export function randomPick<T>(arr: readonly T[]): T {
  return arr[Math.floor(Math.random() * arr.length)]!;
}
