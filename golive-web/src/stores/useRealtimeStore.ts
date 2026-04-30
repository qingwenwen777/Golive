import { create } from 'zustand';
import type { Message } from '@/types/message';

export interface Bullet {
  id: string;
  text: string;
  color?: string;
  user: string;
  ts: number;
}

interface RoomSlice {
  messages: Message[];
  bullets: Bullet[];
  viewerCount: number;
  lastServerTs: number;
}

const MESSAGE_CAP = 500;
const BULLET_CAP = 40;

function emptySlice(): RoomSlice {
  return { messages: [], bullets: [], viewerCount: 0, lastServerTs: 0 };
}

interface RealtimeState {
  rooms: Record<string, RoomSlice>;
  ensureRoom: (roomId: string) => void;
  resetRoom: (roomId: string) => void;
  appendMessage: (roomId: string, m: Message) => void;
  replaceMessage: (roomId: string, id: string, m: Message) => void;
  removeMessage: (roomId: string, id: string) => void;
  appendBullet: (roomId: string, b: Bullet) => void;
  clearBullet: (roomId: string, id: string) => void;
  setViewerCount: (roomId: string, n: number) => void;
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
  resetRoom: (roomId) =>
    set((state) => ({ rooms: { ...state.rooms, [roomId]: emptySlice() } })),
  appendMessage: (roomId, m) =>
    set((state) =>
      updateRoom(state, roomId, (slice) => {
        if (slice.messages.some((x) => x.id === m.id)) {
          return {
            ...slice,
            lastServerTs: Math.max(slice.lastServerTs, m.ts),
          };
        }
        const next = [...slice.messages, m];
        return {
          ...slice,
          messages: next.length > MESSAGE_CAP ? next.slice(next.length - MESSAGE_CAP) : next,
          lastServerTs: m.ts,
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
}));

export function useRoomSlice(roomId: string): RoomSlice {
  return useRealtimeStore((s) => s.rooms[roomId] ?? emptySlice());
}
