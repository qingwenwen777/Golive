export type MessageKind = 'chat' | 'super_chat' | 'system' | 'gift';

export type SuperChatTier = 0 | 1 | 2 | 3 | 4 | 5;

export interface ChatFanBadge {
  creatorId: string;
  level: number;
}

export interface ChatMessage {
  id: string;
  kind: 'chat';
  userId?: string;
  user: string;
  avatar?: string;
  text: string;
  ts: number;
  color?: string;
  role?: 'moderator' | string;
  fanBadge?: ChatFanBadge;
  userLevel?: number;
}

export interface SuperChatMessage {
  id: string;
  kind: 'super_chat';
  userId?: string;
  user: string;
  avatar?: string;
  amount: string;
  tier: SuperChatTier;
  text: string;
  userLevel?: number;
  ts: number;
  pending?: boolean;
  requestId?: string;
}

export interface SystemMessage {
  id: string;
  kind: 'system';
  text: string;
  ts: number;
}

export interface GiftMessage {
  id: string;
  kind: 'gift';
  requestId?: string;
  userId?: string;
  user: string;
  avatar?: string;
  giftName: string;
  giftIcon?: string;
  /** Set on gifts this viewer sent; server broadcasts carry only name and icon. */
  giftId?: string;
  count?: number;
  tier?: 0 | 1 | 2 | 3;
  userLevel?: number;
  totalCoin?: number;
  ts: number;
  self?: boolean;
}

export type Message = ChatMessage | SuperChatMessage | SystemMessage | GiftMessage;
