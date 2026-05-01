import type { ChatMessage } from '@/types/message';

const STORAGE_KEY = 'golive-recent-chat:v1';
const MAX_RECENT_PER_ROOM = 8;
const RECENT_TTL_MS = 24 * 60 * 60 * 1000;

type RecentChatStore = Record<string, ChatMessage[]>;

function isChatMessage(value: unknown): value is ChatMessage {
  if (!value || typeof value !== 'object') return false;
  const item = value as Partial<ChatMessage>;
  return (
    typeof item.id === 'string' &&
    item.kind === 'chat' &&
    typeof item.user === 'string' &&
    typeof item.text === 'string' &&
    typeof item.ts === 'number'
  );
}

function readStore(now = Date.now()): RecentChatStore {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== 'object') return {};

    const cutoff = now - RECENT_TTL_MS;
    const next: RecentChatStore = {};
    for (const [roomId, value] of Object.entries(parsed)) {
      if (!Array.isArray(value)) continue;
      const messages = value
        .filter(isChatMessage)
        .filter((message) => message.ts >= cutoff)
        .sort((a, b) => a.ts - b.ts)
        .slice(-MAX_RECENT_PER_ROOM);
      if (messages.length > 0) next[roomId] = messages;
    }
    return next;
  } catch {
    return {};
  }
}

function writeStore(store: RecentChatStore) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(store));
  } catch {
    // Browsers can reject localStorage in private mode or when quota is full.
  }
}

export function loadRecentChatMessages(roomId: string, now = Date.now()): ChatMessage[] {
  if (!roomId) return [];
  return readStore(now)[roomId] ?? [];
}

export function saveRecentChatMessage(roomId: string, message: ChatMessage, now = Date.now()) {
  if (!roomId) return;
  const store = readStore(now);
  const byId = new Map<string, ChatMessage>();

  for (const item of store[roomId] ?? []) {
    byId.set(item.id, item);
  }
  byId.set(message.id, message);

  const cutoff = now - RECENT_TTL_MS;
  const messages = Array.from(byId.values())
    .filter((item) => item.ts >= cutoff)
    .sort((a, b) => a.ts - b.ts)
    .slice(-MAX_RECENT_PER_ROOM);

  if (messages.length > 0) {
    store[roomId] = messages;
  } else {
    delete store[roomId];
  }
  writeStore(store);
}
