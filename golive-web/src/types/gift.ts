import type { SuperChatTier } from './message';

export type GiftCategory = 'basic' | 'premium' | 'luxury';

export interface Gift {
  id: string;
  name: string;
  nameJa?: string;
  icon: string;
  priceCoin: number;
  category: GiftCategory;
  animation?: 'fly' | 'explode' | 'rain';
  tier: 0 | 1 | 2 | 3;
}

export interface GiftOrder {
  orderId: string;
  requestId: string;
  giftId: string;
  count: number;
  totalCoin: number;
  status: 'pending' | 'success' | 'failed';
  failReason?: 'insufficient_coin' | 'network' | 'duplicate';
  createdAt: string;
}

export interface FanBadge {
  userId: string;
  creatorId: string;
  creatorName: string;
  creatorAvatar?: string;
  totalContribution: number;
  level: number;
  createdAt: string;
  updatedAt: string;
}

export interface GiftSendPayload {
  roomId: string;
  giftId: string;
  count: number;
  requestId: string;
}

export interface SuperChatPayload {
  roomId: string;
  amount: number;
  text: string;
  displayName?: string;
  requestId: string;
}

export interface SuperChatOrder {
  orderId: string;
  requestId: string;
  amount: number;
  tier: SuperChatTier;
  text: string;
  status: 'pending' | 'success' | 'failed';
  failReason?: 'insufficient_coin' | 'network' | 'duplicate';
  createdAt: string;
}

export function amountToTier(amount: number): SuperChatTier {
  if (amount >= 10000) return 5;
  if (amount >= 5000) return 4;
  if (amount >= 2000) return 3;
  if (amount >= 1000) return 2;
  if (amount >= 200) return 1;
  return 0;
}

export const SC_MAX_TEXT_BY_TIER: Record<SuperChatTier, number> = {
  0: 0,
  1: 50,
  2: 100,
  3: 150,
  4: 200,
  5: 200,
};
