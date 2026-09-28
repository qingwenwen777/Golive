import { useEffect, useMemo, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useWebSocket, type ReadyState } from '@/hooks/useWebSocket';
import { useRealtimeStore, useRoomSlice } from '@/stores/useRealtimeStore';
import { useDanmuStore } from '@/stores/useDanmuStore';
import { useDanmuHistory } from '@/api/chat';
import type { ChatFanBadge, ChatMessage, Message, SuperChatTier } from '@/types/message';
import {
  getAuthToken,
  isSessionInvalidAfterRefreshFailure,
  refreshAuthToken,
} from '@/lib/authToken';
import { loadRecentChatMessages, saveRecentChatMessage } from '@/lib/recentChatCache';
import { useAuthStore } from '@/stores/useAuthStore';
import { userName } from '@/types/user';
import type { Stream } from '@/types/stream';
import { betQueryKey } from '@/api/bet';
import { luckyBagQueryKey } from '@/api/luckyBag';
import { micLinkQueryKey } from '@/api/micLink';
import i18n from '@/i18n';
import {
  type BetOption,
  type BetOptionSummary,
  type BetRound,
  type BetRoundView,
} from '@/types/bet';
import { formatNumber } from '@/lib/format';

interface ServerChat {
  type: 'chat';
  id?: string;
  userId?: string;
  user: string;
  avatar?: string;
  text: string;
  color?: string;
  role?: string;
  fanBadge?: ChatFanBadge;
  userLevel?: number;
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
  userLevel?: number;
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
  userLevel?: number;
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
    userLevel?: number;
  }>;
  ts?: number;
}
// Sent only to the chat's sender: the server id assigned to its clientId.
interface ServerChatAck {
  type: 'chat_ack';
  clientId: string;
  id: string;
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
interface ServerRoomUpdated {
  type: 'room_updated';
  title: string;
  description?: string;
  cover?: string;
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
interface ServerLuckyBag {
  type: 'lucky_bag';
  event: 'opened' | 'joined' | 'drawn' | 'cancelled';
  bag?: { id?: string; roomId?: string };
  participantCount?: number;
  winnerCount?: number;
  ts?: number;
}
interface ServerMicLink {
  type: 'mic_link';
  event:
    | 'feature_enabled'
    | 'feature_disabled'
    | 'requested'
    | 'approved'
    | 'rejected'
    | 'cancelled'
    | 'on_air'
    | 'left'
    | 'removed'
    | 'guest_muted'
    | 'guest_unmuted';
  roomId?: string;
  userId?: string;
  ts?: number;
}
type ServerMessage =
  | ServerChat
  | ServerSuperChat
  | ServerGift
  | ServerViewerCount
  | ServerViewerList
  | ServerChatAck
  | ServerSystem
  | ServerLiveStatus
  | ServerRoomUpdated
  | ServerBet
  | ServerLuckyBag
  | ServerMicLink;

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
  sendSuperChat: (payload: {
    amount: string;
    tier: SuperChatTier;
    text: string;
    userLevel?: number;
  }) => boolean;
  clearBullet: (id: string) => void;
}

export function useRoomRealtime(
  roomId: string,
  enabled = true,
  opts: { onLiveEnded?: () => void; ownerId?: string } = {},
): UseRoomRealtimeReturn {
  const ensureRoom = useRealtimeStore((s) => s.ensureRoom);
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
  const refreshedTokenRef = useRef<string | null>(null);
  danmuOnRef.current = danmuOn;
  onLiveEndedRef.current = opts.onLiveEnded;

  useEffect(() => {
    ensureRoom(roomId);
  }, [roomId, ensureRoom]);

  // Bounded history (YouTube-style): a small window of recent chat + SC
  // entries so newcomers see what just happened without flooding the panel.
  const history = useDanmuHistory(roomId, enabled);

  // Fallback: replay recent chats seen by this browser so a refresh / re-entry
  // does not blank out messages if the server-side history endpoint lags.
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
    const params = new URLSearchParams({ roomId });
    if (opts.ownerId) params.set('ownerId', opts.ownerId);
    return `${base}?${params.toString()}`;
  }, [enabled, opts.ownerId, roomId]);

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
    // Never give up: while the gateway restarts or refuses a crowded NAT
    // address, a closed socket would leave the room without chat and the
    // viewer count until a reload. Delays top out at 30s, jittered.
    maxRetries: Infinity,
  });

  // The gateway resolves name/avatar/level server-side from the token's user;
  // this frame only asks it to pick up a profile change.
  useEffect(() => {
    if (readyState !== 'open') return;
    sendMessage({ type: 'viewer_profile' });
  }, [readyState, sendMessage, currentUser]);

  useEffect(() => {
    if (readyState !== 'reconnecting' || !authToken) return;
    if (refreshedTokenRef.current === authToken) return;
    refreshedTokenRef.current = authToken;
    void refreshAuthToken().catch((err: unknown) => {
      if (isSessionInvalidAfterRefreshFailure(err)) {
        logout();
      }
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
          role: parsed.role,
          fanBadge: parsed.fanBadge,
          userLevel: parsed.userLevel,
          ts: parsed.ts ?? now,
        };
        const alreadySeen =
          useRealtimeStore.getState().rooms[roomId]?.messageIndex[msg.id] !== undefined;
        appendMessage(roomId, msg);
        if (!alreadySeen) saveRecentChatMessage(roomId, msg);
        if (!alreadySeen && danmuOnRef.current) {
          appendBullet(roomId, {
            id: genId('b'),
            text: parsed.text,
            color: '#ffffff',
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
          userLevel: parsed.userLevel,
          ts: parsed.ts ?? now,
        });
        break;
      }
      case 'gift': {
        const count = parsed.count ?? 1;
        const currentName = userName(currentUser);
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
          userLevel: parsed.userLevel,
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
            userLevel: viewer.userLevel,
          })),
          parsed.total,
        );
        break;
      }
      case 'chat_ack': {
        // No optimistic local echo to reconcile: the sender's own chat
        // arrives through the room broadcast like everyone else's.
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
      case 'room_updated': {
        queryClient.setQueryData<Stream>(['room', roomId], (prev) =>
          prev
            ? {
                ...prev,
                title: parsed.title,
                description: parsed.description ?? '',
                cover: parsed.cover ?? '',
              }
            : prev,
        );
        void queryClient.invalidateQueries({ queryKey: ['rooms'] });
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
            queryClient.setQueryData<BetRoundView>(betQueryKey(parsed.round.roomId), nextBetView);
            void queryClient.invalidateQueries({ queryKey: betQueryKey(parsed.round.roomId) });
          }
        }
        void queryClient.invalidateQueries({ queryKey: betQueryKey(roomId) });
        if (parsed.event === 'settled' || parsed.event === 'cancelled') {
          void queryClient.invalidateQueries({ queryKey: ['me'] });
        }
        if (parsed.event === 'wagered') break;
        const optionLabel = parsed.option
          ? i18n.t(`betting.option.${parsed.option}`, {
              ns: 'pages',
              defaultValue: parsed.option === 'win' ? 'Can win' : 'Cannot win',
            })
          : i18n.t('betting.systemSettledDefault', {
              ns: 'pages',
              defaultValue: 'Result is in',
            });
        const text =
          parsed.event === 'opened'
            ? i18n.t('betting.systemOpened', {
                ns: 'pages',
                defaultValue: 'Betting is open.',
              })
            : parsed.event === 'settled'
              ? i18n.t('betting.systemSettled', {
                  ns: 'pages',
                  option: optionLabel,
                  defaultValue: 'Bet settled: {{option}}.',
                })
              : i18n.t('betting.systemCancelled', {
                  ns: 'pages',
                  defaultValue: 'Bet cancelled. Wagered coins were refunded.',
                });
        appendMessage(roomId, {
          id: genId('bet'),
          kind: 'system',
          text,
          ts: parsed.ts ?? now,
        });
        break;
      }
      case 'lucky_bag': {
        void queryClient.invalidateQueries({ queryKey: luckyBagQueryKey(roomId) });
        const bagRoomId = parsed.bag?.roomId;
        if (bagRoomId && bagRoomId !== roomId) {
          void queryClient.invalidateQueries({ queryKey: luckyBagQueryKey(bagRoomId) });
        }
        if (parsed.event === 'drawn' || parsed.event === 'cancelled') {
          void queryClient.invalidateQueries({ queryKey: ['me'] });
        }
        if (parsed.event === 'joined') break;
        const bagText =
          parsed.event === 'opened'
            ? i18n.t('luckyBag.systemOpened', {
                ns: 'pages',
                defaultValue: 'A lucky bag is up for grabs!',
              })
            : parsed.event === 'drawn'
              ? i18n.t('luckyBag.systemDrawn', {
                  ns: 'pages',
                  count: parsed.winnerCount ?? 0,
                  formattedCount: formatNumber(parsed.winnerCount ?? 0),
                  defaultValue: 'Lucky bag drawn: {{formattedCount}} winners.',
                })
              : i18n.t('luckyBag.systemCancelled', {
                  ns: 'pages',
                  defaultValue: 'Lucky bag cancelled. Coins were refunded.',
                });
        appendMessage(roomId, {
          id: genId('bag'),
          kind: 'system',
          text: bagText,
          ts: parsed.ts ?? now,
        });
        break;
      }
      case 'mic_link': {
        const targetRoom = parsed.roomId || roomId;
        void queryClient.invalidateQueries({ queryKey: micLinkQueryKey(targetRoom) });
        if (targetRoom !== roomId) {
          void queryClient.invalidateQueries({ queryKey: micLinkQueryKey(roomId) });
        }
        break;
      }
    }
  }, [
    lastMessage,
    roomId,
    appendMessage,
    appendBullet,
    setViewerCount,
    setViewers,
    currentUser,
    queryClient,
  ]);

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

  // Only the text is ours to send: the gateway derives the message id, name,
  // avatar, level and fan badge from the authenticated user. clientId comes
  // back in a chat_ack (to this socket only) with the server id.
  const sendChat = (text: string) => {
    const now = Date.now();
    const ok = sendMessage({
      type: 'chat',
      roomId,
      text,
      clientId: genId('chat'),
      ts: now,
    });
    if (!ok) return false;
    return true;
  };

  const sendSuperChat = (payload: {
    amount: string;
    tier: SuperChatTier;
    text: string;
    userLevel?: number;
  }) => sendMessage({ type: 'super_chat', roomId, ...payload, ts: Date.now() });

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
