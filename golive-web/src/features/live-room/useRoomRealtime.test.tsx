// @vitest-environment jsdom
import { type ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { betQueryKey } from '@/api/bet';
import { loadRecentChatMessages } from '@/lib/recentChatCache';
import { useAuthStore } from '@/stores/useAuthStore';
import { useDanmuStore } from '@/stores/useDanmuStore';
import { useRealtimeStore } from '@/stores/useRealtimeStore';
import type { BetRoundView } from '@/types/bet';
import type { GiftMessage } from '@/types/message';
import { useRoomRealtime } from './useRoomRealtime';

const wsMock = vi.hoisted(() => ({
  sendMessage: vi.fn<(payload: unknown) => boolean>(),
  lastMessage: null as { data: string } | null,
  readyState: 'open',
  retryCount: 0,
}));

const chatMock = vi.hoisted(() => ({
  historyData: undefined as unknown,
}));

vi.mock('@/hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    sendMessage: wsMock.sendMessage,
    lastMessage: wsMock.lastMessage,
    readyState: wsMock.readyState,
    retryCount: wsMock.retryCount,
  }),
}));

vi.mock('@/api/chat', () => ({
  useDanmuHistory: () => ({ data: chatMock.historyData }),
}));

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
}

function wrapperFor(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

function seedAuthedUser() {
  useAuthStore.setState({
    token: 'access-token',
    user: {
      id: 'u-self',
      username: 'xhb',
      displayName: 'XHB',
      avatar: '/avatar.png',
      coinBalance: 5000,
      role: 'user',
      livePermissionStatus: 'approved',
    },
  });
}

function renderRealtime(queryClient = makeQueryClient()) {
  const rendered = renderHook(
    () =>
      useRoomRealtime('room-1', true, {
        activeFanBadge: { creatorId: 'creator-1', level: 3 },
      }),
    { wrapper: wrapperFor(queryClient) },
  );
  return { ...rendered, queryClient };
}

describe('useRoomRealtime', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.stubEnv('VITE_WS_BASE', '/ws');
    wsMock.sendMessage.mockReset();
    wsMock.sendMessage.mockReturnValue(true);
    wsMock.lastMessage = null;
    wsMock.readyState = 'open';
    wsMock.retryCount = 0;
    chatMock.historyData = undefined;
    window.localStorage.clear();
    useRealtimeStore.setState({ rooms: {} });
    useDanmuStore.setState({ on: true, opacity: 1, fontSize: 'md', density: 'med' });
    useAuthStore.setState({
      token: null,
      user: null,
      hasHydrated: true,
    });
    vi.spyOn(Date, 'now').mockReturnValue(1_700_000_000_000);
  });

  it('sends chat with identity without optimistic local echo', () => {
    seedAuthedUser();
    const { result } = renderRealtime();

    act(() => {
      expect(result.current.sendChat('hello room')).toBe(true);
    });

    expect(wsMock.sendMessage).toHaveBeenLastCalledWith(
      expect.objectContaining({
        type: 'chat',
        roomId: 'room-1',
        text: 'hello room',
        user: 'XHB',
        avatar: '/avatar.png',
        fanBadge: { creatorId: 'creator-1', level: 3 },
      }),
    );

    const slice = useRealtimeStore.getState().rooms['room-1'];
    expect(slice.messages).toHaveLength(0);
    expect(slice.bullets).toHaveLength(0);
    expect(loadRecentChatMessages('room-1', 1_700_000_000_000)).toHaveLength(0);
  });

  it('turns server chat into one chat message and one danmu bullet', async () => {
    const { rerender } = renderRealtime();

    wsMock.lastMessage = {
      data: JSON.stringify({
        type: 'chat',
        id: 'server-chat-1',
        userId: 'u-2',
        user: 'Luna',
        text: 'from server',
        color: '#f43f5e',
        ts: 10,
      }),
    };
    rerender();

    await waitFor(() => {
      const slice = useRealtimeStore.getState().rooms['room-1'];
      expect(slice.messages).toHaveLength(1);
      expect(slice.messages[0]).toMatchObject({
        id: 'server-chat-1',
        kind: 'chat',
        user: 'Luna',
        text: 'from server',
      });
      expect(slice.bullets).toHaveLength(1);
      expect(slice.bullets[0]).toMatchObject({ text: 'from server', color: '#ffffff' });
    });
  });

  it('deduplicates a gift broadcast that matches a local optimistic request id', async () => {
    seedAuthedUser();
    const optimistic: GiftMessage = {
      id: 'local-gift',
      kind: 'gift',
      requestId: 'req-1',
      user: 'XHB',
      giftName: 'Rocket',
      count: 1,
      ts: 1_700_000_000_000,
      self: true,
    };
    useRealtimeStore.getState().appendMessage('room-1', optimistic);

    wsMock.lastMessage = {
      data: JSON.stringify({
        type: 'gift',
        id: 'server-order-1',
        requestId: 'req-1',
        user: 'XHB',
        giftName: 'Rocket',
        count: 1,
        ts: 1_700_000_000_100,
      }),
    };

    const { rerender } = renderRealtime();
    rerender();

    await waitFor(() => {
      const gifts = useRealtimeStore
        .getState()
        .rooms['room-1'].messages.filter((message) => message.kind === 'gift');
      expect(gifts).toHaveLength(1);
      expect(gifts[0].id).toBe('local-gift');
    });
  });

  it('updates the bet query cache and posts a system message for bet events', async () => {
    const { queryClient, rerender } = renderRealtime();
    const round = {
      id: 'bet-1',
      roomId: 'room-1',
      ownerId: 'owner-1',
      question: 'Will blue team win?',
      amount: 100,
      status: 'open',
      closeAt: '2026-05-02T12:00:00.000Z',
      createdAt: '2026-05-02T11:59:00.000Z',
      updatedAt: '2026-05-02T11:59:00.000Z',
    };

    wsMock.lastMessage = {
      data: JSON.stringify({
        type: 'bet',
        event: 'opened',
        round,
        summary: [
          { option: 'win', count: 0, total: 0 },
          { option: 'lose', count: 0, total: 0 },
        ],
        ts: 1_700_000_000_000,
      }),
    };
    rerender();

    await waitFor(() => {
      const cached = queryClient.getQueryData<BetRoundView>(betQueryKey('room-1'));
      expect(cached?.round?.id).toBe('bet-1');
      expect(cached?.summary).toHaveLength(2);
      const systemMessages = useRealtimeStore
        .getState()
        .rooms['room-1'].messages.filter((message) => message.kind === 'system');
      expect(systemMessages.at(-1)?.text).toBe('Betting is open.');
    });
  });
});
