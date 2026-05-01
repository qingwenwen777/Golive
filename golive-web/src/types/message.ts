export type MessageKind = 'chat' | 'super_chat' | 'system' | 'gift';

export type SuperChatTier = 0 | 1 | 2 | 3 | 4 | 5;

export interface ChatMessage {
  id: string;
  kind: 'chat';
  userId?: string;
  user: string;
  avatar?: string;
  text: string;
  ts: number;
  color?: string;
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
  count?: number;
  tier?: 0 | 1 | 2 | 3;
  totalCoin?: number;
  ts: number;
  self?: boolean;
}

export type Message = ChatMessage | SuperChatMessage | SystemMessage | GiftMessage;
