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
  fanBadge?: ChatFanBadge;
}

interface SuperChatHistoryItem extends ChatHistoryBase {
  type: 'super_chat';
  amount: string;
  tier?: SuperChatTier;
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
    fanBadge: item.fanBadge,
    ts: item.ts,
  };
}

function toSuperChatMessage(item: SuperChatHistoryItem): SuperChatMessage {
  return {
    id: item.id,
    kind: 'super_chat',
    user: item.user,
    avatar: item.avatar,
    amount: item.amount,
    tier: item.tier ?? 0,
    text: item.text,
    ts: item.ts,
  };
}

function toMessage(item: ChatHistoryItem): Message {
  if (item.type === 'super_chat') return toSuperChatMessage(item);
  return toChatMessage(item);
}

export function useDanmuHistory(roomId: string, enabled = true, limit = 12) {
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
