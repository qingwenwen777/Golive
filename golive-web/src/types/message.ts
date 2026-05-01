export type MessageKind = 'chat' | 'super_chat' | 'system' | 'gift';

export type SuperChatTier = 0 | 1 | 2 | 3 | 4 | 5;

export interface ChatMessage {
  id: string;
  kind: 'chat';
  user: string;
  avatar?: string;
  text: string;
  ts: number;
  color?: string;
}

export interface SuperChatMessage {
  id: string;
  kind: 'super_chat';
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
  user: string;
  giftName: string;
  giftIcon?: string;
  count?: number;
  tier?: 0 | 1 | 2 | 3;
  ts: number;
  self?: boolean;
}

export type Message = ChatMessage | SuperChatMessage | SystemMessage | GiftMessage;
