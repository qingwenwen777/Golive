import { useQuery } from '@tanstack/react-query';
import { http } from '@/lib/axios';
import type {
  ChatFanBadge,
  ChatMessage,
  Message,
  SuperChatMessage,
  SuperChatTier,
} from '@/types/message';

interface ChatHistoryBase {
  type: 'chat' | 'super_chat';
  id: string;
  userId?: string;
  user: string;
  avatar?: string;
  text: string;
  ts: number;
}

interface DanmuHistoryItem extends ChatHistoryBase {
  type: 'chat';
  color?: string;
  role?: string;
  fanBadge?: ChatFanBadge;
}

interface SuperChatHistoryItem extends ChatHistoryBase {
  type: 'super_chat';
  amount: string;
  tier?: SuperChatTier;
  userLevel?: number;
}

type ChatHistoryItem = DanmuHistoryItem | SuperChatHistoryItem;

interface DanmuHistoryResponse {
  items: ChatHistoryItem[];
}

function toChatMessage(item: DanmuHistoryItem): ChatMessage {
  return {
    id: item.id,
    kind: 'chat',
    userId: item.userId,
    user: item.user,
    avatar: item.avatar,
    text: item.text,
    color: item.color,
    role: item.role,
    fanBadge: item.fanBadge,
    ts: item.ts,
  };
}

function toSuperChatMessage(item: SuperChatHistoryItem): SuperChatMessage {
  return {
    id: item.id,
    kind: 'super_chat',
    userId: item.userId,
    user: item.user,
    avatar: item.avatar,
    amount: item.amount,
    tier: item.tier ?? 0,
    text: item.text,
    userLevel: item.userLevel,
    ts: item.ts,
  };
}

function toMessage(item: ChatHistoryItem): Message {
  if (item.type === 'super_chat') return toSuperChatMessage(item);
  return toChatMessage(item);
}

export function useDanmuHistory(roomId: string, enabled = true, limit = 50) {
  return useQuery<Message[], Error>({
    queryKey: ['danmu-history', roomId, limit],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<DanmuHistoryResponse>(
        `/chat/rooms/${encodeURIComponent(roomId)}/danmus`,
        { params: { limit }, signal },
      );
      return data.items.slice().reverse().map(toMessage);
    },
    enabled: enabled && !!roomId,
    staleTime: 0,
    refetchOnMount: 'always',
    retry: 1,
  });
}

export function useReplayMessages(
  roomId: string,
  startedAt?: string,
  endedAt?: string,
  enabled = true,
) {
  return useQuery<Message[], Error>({
    queryKey: ['replay-messages', roomId, startedAt, endedAt],
    queryFn: async ({ signal }) => {
      const startMs = startedAt ? new Date(startedAt).getTime() : 0;
      const endMs = endedAt ? new Date(endedAt).getTime() : Number.POSITIVE_INFINITY;
      const items: ChatHistoryItem[] = [];
      let before = 0;

      for (let page = 0; page < 20; page += 1) {
        const { data } = await http.get<DanmuHistoryResponse>(
          `/chat/rooms/${encodeURIComponent(roomId)}/danmus`,
          { params: { limit: 200, ...(before > 0 ? { before } : {}) }, signal },
        );
        const pageItems = data.items ?? [];
        if (pageItems.length === 0) break;
        items.push(...pageItems);
        const oldest = Math.min(...pageItems.map((item) => item.ts).filter(Number.isFinite));
        if (!Number.isFinite(oldest) || oldest <= startMs) break;
        before = oldest;
      }

      return items
        .filter((item) => item.ts >= startMs && item.ts <= endMs)
        .sort((a, b) => a.ts - b.ts)
        .map(toMessage);
    },
    enabled: enabled && !!roomId && !!startedAt,
    staleTime: 60_000,
    retry: 1,
  });
}
