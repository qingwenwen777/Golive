import { useQuery } from '@tanstack/react-query';
import { http } from '@/lib/axios';
import type { ChatMessage } from '@/types/message';

interface DanmuHistoryItem {
  type: 'chat';
  id: string;
  user: string;
  avatar?: string;
  text: string;
  color?: string;
  ts: number;
}

interface DanmuHistoryResponse {
  items: DanmuHistoryItem[];
}

function toChatMessage(item: DanmuHistoryItem): ChatMessage {
  return {
    id: item.id,
    kind: 'chat',
    user: item.user,
    avatar: item.avatar,
    text: item.text,
    color: item.color,
    ts: item.ts,
  };
}

export function useDanmuHistory(roomId: string, enabled = true, limit = 20) {
  return useQuery<ChatMessage[], Error>({
    queryKey: ['danmu-history', roomId, limit],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<DanmuHistoryResponse>(
        `/chat/rooms/${encodeURIComponent(roomId)}/danmus`,
        { params: { limit }, signal },
      );
      return data.items.slice().reverse().map(toChatMessage);
    },
    enabled: enabled && !!roomId,
    staleTime: 5_000,
    retry: 1,
  });
}
