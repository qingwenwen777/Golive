import { create } from 'zustand';
import type { Message } from '@/types/message';

export interface Bullet {
  id: string;
  text: string;
  color?: string;
  user: string;
  ts: number;
}

export interface RoomViewer {
  userId?: string;
  user: string;
  avatar?: string;
  contribution: number;
  userLevel?: number;
}

interface RoomSlice {
  messages: Message[];
  messageIndex: Record<string, number>;
  bullets: Bullet[];
  viewerCount: number;
  viewers: RoomViewer[];
  lastServerTs: number;
}

const MESSAGE_CAP = 500;
const BULLET_CAP = 40;

function emptySlice(): RoomSlice {
  return {
    messages: [],
    messageIndex: {},
    bullets: [],
    viewerCount: 0,
    viewers: [],
    lastServerTs: 0,
  };
}

function indexMessages(messages: Message[]): Record<string, number> {
  const index: Record<string, number> = {};
  messages.forEach((message, position) => {
    index[message.id] = position;
  });
  return index;
}

function mergeMessageFields(current: Message | undefined, incoming: Message): Message {
  if (!current || current.kind !== incoming.kind) return incoming;
  if (current.kind !== 'chat' || incoming.kind !== 'chat') return incoming;
  // Same id from a different sender is never an update of this message; keep
  // what is on screen rather than let it be rewritten.
  if (current.userId && current.userId !== incoming.userId) return current;
  return {
    ...incoming,
    avatar: incoming.avatar ?? current.avatar,
    color: incoming.color ?? current.color,
    role: incoming.role ?? current.role,
    fanBadge: incoming.fanBadge ?? current.fanBadge,
    userLevel: incoming.userLevel ?? current.userLevel,
  };
}

interface RealtimeState {
  rooms: Record<string, RoomSlice>;
  ensureRoom: (roomId: string) => void;
  resetRoom: (roomId: string) => void;
  appendMessage: (roomId: string, m: Message) => void;
  mergeMessages: (roomId: string, messages: Message[]) => void;
  replaceMessage: (roomId: string, id: string, m: Message) => void;
  removeMessage: (roomId: string, id: string) => void;
  appendBullet: (roomId: string, b: Bullet) => void;
  clearBullet: (roomId: string, id: string) => void;
  setViewerCount: (roomId: string, n: number) => void;
  setViewers: (roomId: string, viewers: RoomViewer[], total?: number) => void;
  incrementViewerContribution: (roomId: string, viewer: RoomViewer, delta: number) => void;
}

function updateRoom(
  state: RealtimeState,
  roomId: string,
  patch: (s: RoomSlice) => RoomSlice,
): Partial<RealtimeState> {
  const current = state.rooms[roomId] ?? emptySlice();
  return { rooms: { ...state.rooms, [roomId]: patch(current) } };
}

export const useRealtimeStore = create<RealtimeState>((set) => ({
  rooms: {},
  ensureRoom: (roomId) =>
    set((state) => {
      if (state.rooms[roomId]) return state;
      return { rooms: { ...state.rooms, [roomId]: emptySlice() } };
    }),
  resetRoom: (roomId) => set((state) => ({ rooms: { ...state.rooms, [roomId]: emptySlice() } })),
  appendMessage: (roomId, m) =>
    set((state) =>
      updateRoom(state, roomId, (slice) => {
        const existingIndex = slice.messageIndex[m.id];
        if (existingIndex !== undefined) {
          const current = slice.messages[existingIndex];
          const messages = slice.messages.slice();
          messages[existingIndex] = mergeMessageFields(current, m);
          return {
            ...slice,
            messages,
            lastServerTs: Math.max(slice.lastServerTs, m.ts),
          };
        }
        const next =
          slice.messages.length >= MESSAGE_CAP
            ? [...slice.messages.slice(1), m]
            : [...slice.messages, m];
        return {
          ...slice,
          messages: next,
          messageIndex:
            next.length === slice.messages.length
              ? indexMessages(next)
              : { ...slice.messageIndex, [m.id]: next.length - 1 },
          lastServerTs: Math.max(slice.lastServerTs, m.ts),
        };
      }),
    ),
  mergeMessages: (roomId, messages) =>
    set((state) =>
      updateRoom(state, roomId, (slice) => {
        if (messages.length === 0) return slice;
        const byID = new Map<string, Message>();
        for (const message of slice.messages) {
          byID.set(message.id, message);
        }
        for (const message of messages) {
          byID.set(message.id, mergeMessageFields(byID.get(message.id), message));
        }
        const next = Array.from(byID.values()).sort((a, b) => a.ts - b.ts);
        const capped = next.length > MESSAGE_CAP ? next.slice(next.length - MESSAGE_CAP) : next;
        return {
          ...slice,
          messages: capped,
          messageIndex: indexMessages(capped),
          lastServerTs: capped.reduce(
            (max, message) => Math.max(max, message.ts),
            slice.lastServerTs,
          ),
        };
      }),
    ),
  replaceMessage: (roomId, id, m) =>
    set((state) =>
      updateRoom(state, roomId, (slice) => {
        const messages = slice.messages.map((x) => (x.id === id ? m : x));
        return {
          ...slice,
          messages,
          messageIndex: indexMessages(messages),
        };
      }),
    ),
  removeMessage: (roomId, id) =>
    set((state) =>
      updateRoom(state, roomId, (slice) => {
        const messages = slice.messages.filter((x) => x.id !== id);
        return {
          ...slice,
          messages,
          messageIndex: indexMessages(messages),
        };
      }),
    ),
  appendBullet: (roomId, b) =>
    set((state) =>
      updateRoom(state, roomId, (slice) => {
        const next = [...slice.bullets, b];
        return {
          ...slice,
          bullets: next.length > BULLET_CAP ? next.slice(next.length - BULLET_CAP) : next,
        };
      }),
    ),
  clearBullet: (roomId, id) =>
    set((state) =>
      updateRoom(state, roomId, (slice) => ({
        ...slice,
        bullets: slice.bullets.filter((b) => b.id !== id),
      })),
    ),
  setViewerCount: (roomId, n) =>
    set((state) => updateRoom(state, roomId, (slice) => ({ ...slice, viewerCount: n }))),
  setViewers: (roomId, viewers, total) =>
    set((state) =>
      updateRoom(state, roomId, (slice) => ({
        ...slice,
        viewers,
        viewerCount: total ?? slice.viewerCount,
      })),
    ),
  incrementViewerContribution: (roomId, viewer, delta) =>
    set((state) =>
      updateRoom(state, roomId, (slice) => {
        const contributionDelta = Math.max(0, Math.floor(delta));
        if (contributionDelta <= 0) return slice;

        let matched = false;
        const viewers = slice.viewers.map((item) => {
          const sameViewer = viewer.userId
            ? item.userId === viewer.userId
            : item.user === viewer.user;
          if (!sameViewer) return item;
          matched = true;
          return {
            ...item,
            userId: viewer.userId ?? item.userId,
            user: viewer.user || item.user,
            avatar: viewer.avatar ?? item.avatar,
            userLevel: viewer.userLevel ?? item.userLevel,
            contribution: item.contribution + contributionDelta,
          };
        });

        if (!matched) {
          viewers.push({
            ...viewer,
            contribution: contributionDelta,
          });
        }

        return {
          ...slice,
          viewers,
          viewerCount: Math.max(slice.viewerCount, viewers.length),
        };
      }),
    ),
}));

export function useRoomSlice(roomId: string): RoomSlice {
  return useRealtimeStore((s) => s.rooms[roomId] ?? emptySlice());
}
