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
}

interface RoomSlice {
  messages: Message[];
  bullets: Bullet[];
  viewerCount: number;
  viewers: RoomViewer[];
  lastServerTs: number;
}

const MESSAGE_CAP = 500;
const BULLET_CAP = 40;

function emptySlice(): RoomSlice {
  return { messages: [], bullets: [], viewerCount: 0, viewers: [], lastServerTs: 0 };
}

function mergeMessageFields(current: Message | undefined, incoming: Message): Message {
  if (!current || current.kind !== incoming.kind) return incoming;
  if (current.kind !== 'chat' || incoming.kind !== 'chat') return incoming;
  return {
    ...incoming,
    avatar: incoming.avatar ?? current.avatar,
    color: incoming.color ?? current.color,
    role: incoming.role ?? current.role,
    fanBadge: incoming.fanBadge ?? current.fanBadge,
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
        if (slice.messages.some((x) => x.id === m.id)) {
          const messages = slice.messages.map((x) =>
            x.id === m.id ? mergeMessageFields(x, m) : x,
          );
          return {
            ...slice,
            messages,
            lastServerTs: Math.max(slice.lastServerTs, m.ts),
          };
        }
        const next = [...slice.messages, m];
        return {
          ...slice,
          messages: next.length > MESSAGE_CAP ? next.slice(next.length - MESSAGE_CAP) : next,
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
          lastServerTs: capped.reduce(
            (max, message) => Math.max(max, message.ts),
            slice.lastServerTs,
          ),
        };
      }),
    ),
  replaceMessage: (roomId, id, m) =>
    set((state) =>
      updateRoom(state, roomId, (slice) => ({
        ...slice,
        messages: slice.messages.map((x) => (x.id === id ? m : x)),
      })),
    ),
  removeMessage: (roomId, id) =>
    set((state) =>
      updateRoom(state, roomId, (slice) => ({
        ...slice,
        messages: slice.messages.filter((x) => x.id !== id),
      })),
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
