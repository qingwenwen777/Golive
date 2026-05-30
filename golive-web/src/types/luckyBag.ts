export type LuckyBagAmountMode = 'fixed' | 'random';
export type LuckyBagEligibility = 'all' | 'followers' | 'fans' | 'fans_level';
export type LuckyBagStatus = 'open' | 'drawn' | 'cancelled';
export type LuckyBagEntryStatus = 'joined' | 'won' | 'missed';

export interface LuckyBag {
  id: string;
  roomId: string;
  ownerId: string;
  message?: string;
  totalCoin: number;
  count: number;
  amountMode: LuckyBagAmountMode;
  eligibility: LuckyBagEligibility;
  minFanLevel: number;
  status: LuckyBagStatus;
  closeAt: string;
  drawnAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface LuckyBagEntry {
  id: string;
  bagId: string;
  roomId: string;
  status: LuckyBagEntryStatus;
  payout: number;
  createdAt: string;
  updatedAt: string;
}

export interface LuckyBagWinner {
  userId: string;
  name: string;
  avatar?: string;
  payout: number;
}

export interface LuckyBagView {
  bag: LuckyBag | null;
  myEntry?: LuckyBagEntry;
  participantCount: number;
  winners?: LuckyBagWinner[];
}
