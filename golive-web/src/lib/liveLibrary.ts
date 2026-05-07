import type { Stream } from '@/types/stream';

export const WATCH_HISTORY_KEY = 'golive-watch-history';
export const WATCH_LATER_KEY = 'golive-watch-later';
export const LIKED_STREAMS_KEY = 'golive-liked-streams';
export const LIBRARY_KEYS = [WATCH_HISTORY_KEY, WATCH_LATER_KEY, LIKED_STREAMS_KEY] as const;
const WRITE_DEBOUNCE_MS = 500;

export type LibraryStream = Stream & {
  savedAt?: string;
  watchedAt?: string;
};

export const MAX_LIBRARY_ITEMS = 60;

interface CachedLibrary {
  items: LibraryStream[];
  raw: string | null;
  dirty: boolean;
}

const libraryCache = new Map<string, CachedLibrary>();
let flushTimer: number | null = null;
let listenersInstalled = false;

export function readLibrary(key: string): LibraryStream[] {
  if (typeof window === 'undefined') return [];
  const raw = window.localStorage.getItem(key);
  const cached = libraryCache.get(key);
  if (cached && (cached.dirty || cached.raw === raw)) return cached.items;

  try {
    if (!raw) {
      libraryCache.set(key, { items: [], raw, dirty: false });
      return [];
    }
    const parsed = JSON.parse(raw);
    const items = Array.isArray(parsed) ? parsed.filter(isLibraryStream) : [];
    libraryCache.set(key, { items, raw, dirty: false });
    return items;
  } catch {
    libraryCache.set(key, { items: [], raw, dirty: false });
    return [];
  }
}

export function saveToLibrary(
  key: string,
  stream: Stream,
  stamp: 'savedAt' | 'watchedAt' = 'savedAt',
): LibraryStream[] {
  const now = new Date().toISOString();
  const current = readLibrary(key).filter((item) => item.id !== stream.id);
  const next = [{ ...stream, [stamp]: now }, ...current].slice(0, MAX_LIBRARY_ITEMS);
  writeLibrary(key, next);
  return next;
}

export function removeFromLibrary(key: string, streamId: string): LibraryStream[] {
  const next = readLibrary(key).filter((item) => item.id !== streamId);
  writeLibrary(key, next);
  return next;
}

export function clearLibrary(key: string): void {
  writeLibrary(key, []);
}

export function removeStreamFromLibraries(
  streamId: string,
  keys: readonly string[] = LIBRARY_KEYS,
): void {
  for (const key of keys) removeFromLibrary(key, streamId);
}

export function markStreamEndedInLibraries(
  stream: Stream,
  keys: readonly string[] = LIBRARY_KEYS,
): void {
  for (const key of keys) {
    const next = readLibrary(key).map((item) =>
      item.id === stream.id ? markStreamNotLive({ ...item, ...stream }) : item,
    );
    writeLibrary(key, next);
  }
}

export function filterLibraryItemsByLiveRooms(
  items: LibraryStream[],
  liveStreams: Stream[],
): LibraryStream[] {
  const liveById = new Map(liveStreams.map((stream) => [stream.id, stream]));
  return items.map((item) => {
    const live = liveById.get(item.id);
    if (!live) return markStreamNotLive(item);
    return { ...item, ...live, savedAt: item.savedAt, watchedAt: item.watchedAt };
  });
}

export function syncLibraryWithLiveRooms(key: string, liveStreams: Stream[]): LibraryStream[] {
  const current = readLibrary(key);
  const next = filterLibraryItemsByLiveRooms(current, liveStreams);
  if (libraryItemsEqual(current, next)) return current;
  writeLibrary(key, next, { defer: true });
  return next;
}

export function isInLibrary(key: string, streamId: string): boolean {
  return readLibrary(key).some((item) => item.id === streamId);
}

function writeLibrary(
  key: string,
  items: LibraryStream[],
  options: { defer?: boolean } = {},
): void {
  if (typeof window === 'undefined') return;
  if (!options.defer) {
    const raw = JSON.stringify(items);
    window.localStorage.setItem(key, raw);
    libraryCache.set(key, { items, raw, dirty: false });
    return;
  }

  libraryCache.set(key, {
    items,
    raw: libraryCache.get(key)?.raw ?? window.localStorage.getItem(key),
    dirty: true,
  });
  scheduleFlush();
}

function markStreamNotLive(stream: LibraryStream): LibraryStream {
  const rest = { ...stream };
  delete rest.playbackUrl;
  delete rest.streamKey;
  return {
    ...rest,
    viewers: 0,
    isLive: false,
    status: 'ended',
  };
}

function isLibraryStream(value: unknown): value is LibraryStream {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<LibraryStream>;
  return typeof candidate.id === 'string' && typeof candidate.title === 'string';
}

function libraryItemsEqual(a: LibraryStream[], b: LibraryStream[]): boolean {
  if (a.length !== b.length) return false;
  return a.every((item, index) => JSON.stringify(item) === JSON.stringify(b[index]));
}

function scheduleFlush() {
  installFlushListeners();
  if (typeof window === 'undefined') return;
  if (flushTimer !== null) return;
  flushTimer = window.setTimeout(() => {
    flushTimer = null;
    flushLiveLibrary();
  }, WRITE_DEBOUNCE_MS);
}

function installFlushListeners() {
  if (listenersInstalled || typeof window === 'undefined') return;
  listenersInstalled = true;
  window.addEventListener('pagehide', flushLiveLibrary);
  window.addEventListener('beforeunload', flushLiveLibrary);
}

export function flushLiveLibrary(): void {
  if (typeof window === 'undefined') return;
  for (const [key, cached] of libraryCache) {
    if (!cached.dirty) continue;
    const raw = JSON.stringify(cached.items);
    try {
      window.localStorage.setItem(key, raw);
      libraryCache.set(key, { ...cached, raw, dirty: false });
    } catch {
      // localStorage can be unavailable or full; keep the in-memory copy.
    }
  }
}

export function resetLiveLibraryCacheForTest(): void {
  libraryCache.clear();
  if (flushTimer !== null && typeof window !== 'undefined') {
    window.clearTimeout(flushTimer);
  }
  flushTimer = null;
}
