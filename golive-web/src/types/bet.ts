export type BetOption = 'win' | 'lose';
export type BetRoundStatus = 'open' | 'closed' | 'settled' | 'cancelled';
export type BetWagerStatus = 'locked' | 'won' | 'lost' | 'refunded';

export interface BetRound {
  id: string;
  roomId: string;
  ownerId: string;
  question: string;
  amount: number;
  status: BetRoundStatus;
  winningOption?: BetOption;
  closeAt: string;
  settledAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface BetOptionSummary {
  option: BetOption;
  count: number;
  total: number;
}

export interface BetWager {
  id: string;
  roundId: string;
  roomId: string;
  option: BetOption;
  amount: number;
  payout: number;
  status: BetWagerStatus;
  createdAt: string;
  updatedAt: string;
}

export interface BetRoundView {
  round: BetRound | null;
  summary: BetOptionSummary[];
  myWager?: BetWager;
}

export function betOptionLabel(option: BetOption): string {
  return option === 'win' ? '能' : '不能';
}
