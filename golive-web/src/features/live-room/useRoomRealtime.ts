import { useEffect, useMemo, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useWebSocket, type ReadyState } from '@/hooks/useWebSocket';
import { useRealtimeStore, useRoomSlice } from '@/stores/useRealtimeStore';
import { useDanmuStore } from '@/stores/useDanmuStore';
import { useDanmuHistory } from '@/api/chat';
import type { ChatFanBadge, ChatMessage, Message, SuperChatTier } from '@/types/message';
import { getAuthToken, refreshAuthToken } from '@/lib/authToken';
import { loadRecentChatMessages, saveRecentChatMessage } from '@/lib/recentChatCache';
import { useAuthStore } from '@/stores/useAuthStore';
import { userDisplayName } from '@/types/user';
import { betQueryKey } from '@/api/bet';
import {
  betOptionLabel,
  type BetOption,
  type BetOptionSummary,
  type BetRound,
  type BetRoundView,
} from '@/types/bet';

interface ServerChat {
  type: 'chat';
  id?: string;
  userId?: string;
  user: string;
  avatar?: string;
  text: string;
  color?: string;
  fanBadge?: ChatFanBadge;
  ts?: number;
}
interface ServerSuperChat {
  type: 'super_chat';
  id?: string;
  userId?: string;
  user: string;
  avatar?: string;
  amount: string;
  tier: SuperChatTier;
  text: string;
  ts?: number;
}
interface ServerGift {
  type: 'gift';
  id?: string;
  requestId?: string;
  userId?: string;
  user: string;
  avatar?: string;
  giftName: string;
  giftIcon?: string;
  count?: number;
  tier?: 0 | 1 | 2 | 3;
  totalCoin?: number;
  ts?: number;
}
interface ServerViewerCount {
  type: 'viewer_count';
  count: number;
}
interface ServerViewerList {
  type: 'viewer_list';
  total: number;
  viewers: Array<{
    userId?: string;
    user: string;
    avatar?: string;
    contribution?: number;
  }>;
  ts?: number;
}
interface ServerSystem {
  type: 'system';
  text: string;
  ts?: number;
}
interface ServerLiveStatus {
  type: 'live_status' | 'live_ended';
  status?: 'ended' | string;
  text?: string;
  ts?: number;
}
interface ServerBet {
  type: 'bet';
  event: 'opened' | 'wagered' | 'settled' | 'cancelled';
  round?: BetRound;
  summary?: BetOptionSummary[];
  option?: BetOption;
  ts?: number;
}
type ServerMessage =
  | ServerChat
  | ServerSuperChat
  | ServerGift
  | ServerViewerCount
  | ServerViewerList
  | ServerSystem
  | ServerLiveStatus
  | ServerBet;

function genId(prefix: string): string {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function giftMessageId(parsed: ServerGift): string {
  return parsed.requestId ? `gift:${parsed.requestId}` : (parsed.id ?? genId('gift'));
}

export interface UseRoomRealtimeReturn {
  readyState: ReadyState;
  retryCount: number;
  messages: Message[];
  viewerCount: number;
  viewers: ReturnType<typeof useRoomSlice>['viewers'];
  bullets: ReturnType<typeof useRoomSlice>['bullets'];
  sendChat: (text: string) => boolean;
  sendSuperChat: (payload: { amount: string; tier: SuperChatTier; text: string }) => boolean;
  clearBullet: (id: string) => void;
}

export function useRoomRealtime(
  roomId: string,
  enabled = true,
  opts: { onLiveEnded?: () => void; activeFanBadge?: ChatFanBadge | null } = {},
): UseRoomRealtimeReturn {
  const ensureRoom = useRealtimeStore((s) => s.ensureRoom);
  const resetRoom = useRealtimeStore((s) => s.resetRoom);
  const appendMessage = useRealtimeStore((s) => s.appendMessage);
  const mergeMessages = useRealtimeStore((s) => s.mergeMessages);
  const appendBullet = useRealtimeStore((s) => s.appendBullet);
  const clearBulletFn = useRealtimeStore((s) => s.clearBullet);
  const setViewerCount = useRealtimeStore((s) => s.setViewerCount);
  const setViewers = useRealtimeStore((s) => s.setViewers);
  const danmuOn = useDanmuStore((s) => s.on);
  const currentUser = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);
  const queryClient = useQueryClient();
  const danmuOnRef = useRef(danmuOn);
  const onLiveEndedRef = useRef(opts.onLiveEnded);
  const activeFanBadgeRef = useRef<ChatFanBadge | null | undefined>(opts.activeFanBadge);
  const refreshedTokenRef = useRef<string | null>(null);
  danmuOnRef.current = danmuOn;
  onLiveEndedRef.current = opts.onLiveEnded;
  activeFanBadgeRef.current = opts.activeFanBadge;

  useEffect(() => {
    ensureRoom(roomId);
    return () => resetRoom(roomId);
  }, [roomId, ensureRoom, resetRoom]);

  // Bounded history (YouTube-style): a small window of recent chat + SC
  // entries so newcomers see what just happened without flooding the panel.
  const history = useDanmuHistory(roomId, enabled, 12);

  // Fallback: replay the user's own recent chats from localStorage so a
  // refresh / re-entry does not blank out messages that the server-side
  // history endpoint did not return (chat persistence may be skipped in
  // local dev when Kafka is the no-op producer).
  useEffect(() => {
    if (!enabled || !roomId) return;
    const cached = loadRecentChatMessages(roomId);
    if (cached.length > 0) mergeMessages(roomId, cached);
  }, [enabled, mergeMessages, roomId]);

  useEffect(() => {
    if (!history.data) return;
    mergeMessages(roomId, history.data);
  }, [history.data, mergeMessages, roomId]);

  const url = useMemo(() => {
    if (!enabled || !roomId) return null;
    const raw = import.meta.env.VITE_WS_BASE;
    const base = raw.startsWith('/')
      ? `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}${raw}`
      : raw;
    return `${base}?roomId=${encodeURIComponent(roomId)}`;
  }, [enabled, roomId]);

  // Subscribe to auth token so a login/logout transition reconnects the WS
  // with the latest credentials. Without this, an anonymous handshake would
  // persist after sign-in and the gateway would reject normal chat with
  // `login required to chat`.
  const authToken = useAuthStore((s) => s.token);

  const { sendMessage, lastMessage, readyState, retryCount } = useWebSocket(url, {
    token: authToken,
    getToken: getAuthToken,
    heartbeatMs: 20000,
    reconnect: true,
    maxRetries: 10,
  });

  useEffect(() => {
    if (readyState !== 'open') return;
    sendMessage({
      type: 'viewer_profile',
      user: currentUser ? userDisplayName(currentUser) : 'Guest',
      avatar: currentUser?.avatar,
    });
  }, [readyState, sendMessage, currentUser]);

  useEffect(() => {
    if (readyState !== 'reconnecting' || !authToken) return;
    if (refreshedTokenRef.current === authToken) return;
    refreshedTokenRef.current = authToken;
    void refreshAuthToken().catch(() => {
      logout();
    });
  }, [readyState, authToken, logout]);

  // Dispatch last message into store.
  useEffect(() => {
    if (!lastMessage) return;
    let parsed: ServerMessage | null = null;
    try {
      parsed = JSON.parse(
        typeof lastMessage.data === 'string' ? lastMessage.data : String(lastMessage.data),
      ) as ServerMessage;
    } catch {
      return;
    }
    if (!parsed || typeof parsed !== 'object') return;

    const now = Date.now();
    switch (parsed.type) {
      case 'chat': {
        const msg: ChatMessage = {
          id: parsed.id ?? genId('m'),
          kind: 'chat',
          userId: parsed.userId,
          user: parsed.user,
          avatar: parsed.avatar,
          text: parsed.text,
          color: parsed.color,
          fanBadge: parsed.fanBadge,
          ts: parsed.ts ?? now,
        };
        const alreadySeen = useRealtimeStore
          .getState()
          .rooms[roomId]?.messages.some((x) => x.id === msg.id);
        appendMessage(roomId, msg);
        saveRecentChatMessage(roomId, msg);
        if (!alreadySeen && danmuOnRef.current) {
          appendBullet(roomId, {
            id: genId('b'),
            text: `${parsed.user}: ${parsed.text}`,
            color: parsed.color,
            user: parsed.user,
            ts: msg.ts,
          });
        }
        break;
      }
      case 'super_chat': {
        appendMessage(roomId, {
          id: parsed.id ?? genId('sc'),
          kind: 'super_chat',
          userId: parsed.userId,
          user: parsed.user,
          avatar: parsed.avatar,
          amount: parsed.amount,
          tier: parsed.tier,
          text: parsed.text,
          ts: parsed.ts ?? now,
        });
        break;
      }
      case 'gift': {
        const count = parsed.count ?? 1;
        const currentName = userDisplayName(currentUser);
        // Dedupe priority:
        //   1) Same requestId already present (the local optimistic copy).
        //      The server broadcast may use a different message id (orderID)
        //      than the local optimistic one, so id-only dedupe is not enough.
        //   2) Fallback: a recent self-flagged gift with matching name/count
        //      from the same user (handles missing requestId).
        const existing = useRealtimeStore.getState().rooms[roomId]?.messages ?? [];
        const duplicate = existing.some((message) => {
          if (message.kind !== 'gift') return false;
          if (parsed.requestId && message.requestId === parsed.requestId) return true;
          return (
            message.self === true &&
            message.giftName === parsed.giftName &&
            (message.count ?? 1) === count &&
            parsed.user === currentName &&
            now - message.ts < 15_000
          );
        });
        if (duplicate) break;

        appendMessage(roomId, {
          id: giftMessageId(parsed),
          kind: 'gift',
          requestId: parsed.requestId,
          userId: parsed.userId,
          user: parsed.user,
          avatar: parsed.avatar,
          giftName: parsed.giftName,
          giftIcon: parsed.giftIcon,
          count,
          tier: parsed.tier,
          totalCoin: parsed.totalCoin,
          ts: parsed.ts ?? now,
        });
        break;
      }
      case 'viewer_count': {
        setViewerCount(roomId, parsed.count);
        break;
      }
      case 'viewer_list': {
        setViewers(
          roomId,
          parsed.viewers.map((viewer) => ({
            userId: viewer.userId,
            user: viewer.user,
            avatar: viewer.avatar,
            contribution: viewer.contribution ?? 0,
          })),
          parsed.total,
        );
        break;
      }
      case 'system': {
        appendMessage(roomId, {
          id: genId('sys'),
          kind: 'system',
          text: parsed.text,
          ts: parsed.ts ?? now,
        });
        break;
      }
      case 'live_status':
      case 'live_ended': {
        if (parsed.type === 'live_ended' || parsed.status === 'ended') {
          appendMessage(roomId, {
            id: genId('sys'),
            kind: 'system',
            text: parsed.text || 'Live has ended.',
            ts: parsed.ts ?? now,
          });
          onLiveEndedRef.current?.();
        }
        break;
      }
      case 'bet': {
        if (parsed.round && isCompleteBetRound(parsed.round)) {
          const nextBetView = (prev?: BetRoundView): BetRoundView => ({
            round: parsed.round ?? null,
            summary: parsed.summary ?? prev?.summary ?? [],
            ...(prev?.myWager ? { myWager: prev.myWager } : {}),
          });
          queryClient.setQueryData<BetRoundView>(betQueryKey(roomId), nextBetView);
          if (parsed.round.roomId && parsed.round.roomId !== roomId) {
            queryClient.setQueryData<BetRoundView>(
              betQueryKey(parsed.round.roomId),
              nextBetView,
            );
            void queryClient.invalidateQueries({ queryKey: betQueryKey(parsed.round.roomId) });
          }
        }
        void queryClient.invalidateQueries({ queryKey: betQueryKey(roomId) });
        if (parsed.event === 'settled' || parsed.event === 'cancelled') {
          void queryClient.invalidateQueries({ queryKey: ['me'] });
        }
        if (parsed.event === 'wagered') break;
        const text =
          parsed.event === 'opened'
            ? '竞猜已开盘。'
            : parsed.event === 'settled'
              ? `竞猜已结算：${parsed.option ? betOptionLabel(parsed.option) : '结果已出'}。`
              : '竞猜已流盘，下注 coin 已退回。';
        appendMessage(roomId, {
          id: genId('bet'),
          kind: 'system',
          text,
          ts: parsed.ts ?? now,
        });
        break;
      }
    }
  }, [lastMessage, roomId, appendMessage, appendBullet, setViewerCount, setViewers, currentUser, queryClient]);

  // On reconnect (retryCount reset to 0 after open), send resume with lastTs.
  const prevReadyStateRef = useRef<ReadyState>('closed');
  useEffect(() => {
    const prev = prevReadyStateRef.current;
    prevReadyStateRef.current = readyState;
    if (readyState === 'open' && (prev === 'reconnecting' || prev === 'connecting')) {
      const slice = useRealtimeStore.getState().rooms[roomId];
      const lastTs = slice?.lastServerTs ?? 0;
      if (lastTs > 0) sendMessage({ type: 'resume', lastTs });
    }
  }, [readyState, roomId, sendMessage]);

  const slice = useRoomSlice(roomId);

  const sendChat = (text: string) => {
    const now = Date.now();
    const id = genId('chat');
    const user = userDisplayName(currentUser);
    const avatar = currentUser?.avatar;
    const fanBadge = activeFanBadgeRef.current ?? undefined;
    const ok = sendMessage({
      type: 'chat',
      roomId,
      text,
      user,
      avatar,
      fanBadge,
      clientId: id,
      ts: now,
    });
    if (!ok) return false;

    const msg: ChatMessage = {
      id,
      kind: 'chat',
      userId: currentUser?.id,
      user,
      avatar,
      text,
      fanBadge,
      ts: now,
    };
    appendMessage(roomId, msg);
    saveRecentChatMessage(roomId, msg);
    if (danmuOnRef.current) {
      appendBullet(roomId, {
        id: genId('b'),
        text: `${user}: ${text}`,
        user,
        ts: now,
      });
    }
    return true;
  };

  const sendSuperChat = (payload: { amount: string; tier: SuperChatTier; text: string }) =>
    sendMessage({ type: 'super_chat', roomId, ...payload, ts: Date.now() });

  const clearBullet = (id: string) => clearBulletFn(roomId, id);

  return {
    readyState,
    retryCount,
    messages: slice.messages,
    viewerCount: slice.viewerCount,
    viewers: slice.viewers,
    bullets: slice.bullets,
    sendChat,
    sendSuperChat,
    clearBullet,
  };
}

function isCompleteBetRound(round: Partial<BetRound>): round is BetRound {
  return Boolean(round.id && round.roomId && round.question && round.status && round.closeAt);
}
