import type { ChatMessage } from '@/types/message';

const STORAGE_KEY = 'golive-recent-chat:v1';
const MAX_RECENT_PER_ROOM = 50;
const RECENT_TTL_MS = 24 * 60 * 60 * 1000;
const WRITE_DEBOUNCE_MS = 250;

type RecentChatStore = Record<string, ChatMessage[]>;

let cachedStore: RecentChatStore | null = null;
let cachedStorage: Storage | null = null;
let persistedRaw: string | null = null;
let flushTimer: number | null = null;
let dirty = false;
let listenersInstalled = false;

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

function getStorage(): Storage | null {
  if (typeof localStorage === 'undefined') return null;
  return localStorage;
}

function sanitizeStore(parsed: unknown, now = Date.now()): RecentChatStore {
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
}

function readStore(now = Date.now()): RecentChatStore {
  const storage = getStorage();
  if (!storage) return {};
  const raw = storage.getItem(STORAGE_KEY);
  if (cachedStore && cachedStorage === storage && (dirty || raw === persistedRaw)) {
    cachedStore = sanitizeStore(cachedStore, now);
    return cachedStore;
  }

  try {
    if (!raw) {
      cachedStore = {};
      cachedStorage = storage;
      persistedRaw = raw;
      dirty = false;
      return cachedStore;
    }
    const parsed = JSON.parse(raw) as unknown;
    cachedStore = sanitizeStore(parsed, now);
    cachedStorage = storage;
    persistedRaw = raw;
    dirty = false;
    return cachedStore;
  } catch {
    cachedStore = {};
    cachedStorage = storage;
    persistedRaw = raw;
    dirty = false;
    return cachedStore;
  }
}

function flushStore() {
  if (!dirty || !cachedStore || !cachedStorage) return;
  try {
    const raw = JSON.stringify(cachedStore);
    cachedStorage.setItem(STORAGE_KEY, raw);
    persistedRaw = raw;
    dirty = false;
  } catch {
    // Browsers can reject localStorage in private mode or when quota is full.
  }
}

function scheduleWrite() {
  dirty = true;
  installFlushListeners();
  if (typeof window === 'undefined') return;
  if (flushTimer !== null) return;
  flushTimer = window.setTimeout(() => {
    flushTimer = null;
    flushStore();
  }, WRITE_DEBOUNCE_MS);
}

function installFlushListeners() {
  if (listenersInstalled || typeof window === 'undefined') return;
  listenersInstalled = true;
  window.addEventListener('pagehide', flushStore);
  window.addEventListener('beforeunload', flushStore);
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
  cachedStore = store;
  scheduleWrite();
}

export function flushRecentChatCache(): void {
  if (flushTimer !== null && typeof window !== 'undefined') {
    window.clearTimeout(flushTimer);
    flushTimer = null;
  }
  flushStore();
}

export function resetRecentChatCacheForTest(): void {
  if (flushTimer !== null && typeof window !== 'undefined') {
    window.clearTimeout(flushTimer);
  }
  cachedStore = null;
  cachedStorage = null;
  persistedRaw = null;
  flushTimer = null;
  dirty = false;
}
