import type { Stream } from '@/types/stream';

export const WATCH_HISTORY_KEY = 'golive-watch-history';
export const WATCH_LATER_KEY = 'golive-watch-later';
export const LIKED_STREAMS_KEY = 'golive-liked-streams';
export const LIBRARY_KEYS = [WATCH_HISTORY_KEY, WATCH_LATER_KEY, LIKED_STREAMS_KEY] as const;

export type LibraryStream = Stream & {
  savedAt?: string;
  watchedAt?: string;
};

export const MAX_LIBRARY_ITEMS = 60;

export function readLibrary(key: string): LibraryStream[] {
  if (typeof window === 'undefined') return [];
  try {
    const raw = window.localStorage.getItem(key);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(isLibraryStream);
  } catch {
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
  const next = filterLibraryItemsByLiveRooms(readLibrary(key), liveStreams);
  writeLibrary(key, next);
  return next;
}

export function isInLibrary(key: string, streamId: string): boolean {
  return readLibrary(key).some((item) => item.id === streamId);
}

function writeLibrary(key: string, items: LibraryStream[]): void {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(key, JSON.stringify(items));
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
