export const COIN_ACTIVITY_KEY_PREFIX = 'golive-coin-activity';
const BEIJING_OFFSET_MS = 8 * 60 * 60 * 1000;
const WRITE_DEBOUNCE_MS = 1000;

export interface DailyCoinActivity {
  date: string;
  watchedRoomIds: string[];
  watchSeconds: number;
}

interface CachedActivity {
  activity: DailyCoinActivity;
  raw: string | null;
  dirty: boolean;
}

const activityCache = new Map<string, CachedActivity>();
let flushTimer: number | null = null;
let listenersInstalled = false;

export function coinTodayKey(date = new Date()): string {
  const beijingDate = new Date(date.getTime() + BEIJING_OFFSET_MS);
  const year = beijingDate.getUTCFullYear();
  const month = String(beijingDate.getUTCMonth() + 1).padStart(2, '0');
  const day = String(beijingDate.getUTCDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function readDailyCoinActivity(userId: string | null | undefined): DailyCoinActivity {
  if (typeof window === 'undefined' || !userId) return fallback();
  const key = activityKey(userId);
  const raw = window.localStorage.getItem(key);
  const cached = activityCache.get(userId);
  if (cached && (cached.dirty || cached.raw === raw)) return cloneActivity(cached.activity);

  try {
    if (!raw) {
      const activity = fallback();
      activityCache.set(userId, { activity, raw, dirty: false });
      return cloneActivity(activity);
    }
    const parsed = JSON.parse(raw) as Partial<DailyCoinActivity>;
    const base = fallback();
    if (parsed.date !== base.date) {
      activityCache.set(userId, { activity: base, raw, dirty: false });
      return cloneActivity(base);
    }
    const activity = {
      date: base.date,
      watchedRoomIds: Array.isArray(parsed.watchedRoomIds)
        ? parsed.watchedRoomIds.filter((id): id is string => typeof id === 'string')
        : [],
      watchSeconds: Number.isFinite(parsed.watchSeconds) ? Number(parsed.watchSeconds) : 0,
    };
    activityCache.set(userId, { activity, raw, dirty: false });
    return cloneActivity(activity);
  } catch {
    const activity = fallback();
    activityCache.set(userId, { activity, raw, dirty: false });
    return cloneActivity(activity);
  }
}

export function markDailyCoinRoomWatched(userId: string | null | undefined, roomId: string): void {
  if (typeof window === 'undefined' || !userId || !roomId) return;
  const next = readDailyCoinActivity(userId);
  if (!next.watchedRoomIds.includes(roomId)) next.watchedRoomIds.push(roomId);
  writeDailyCoinActivity(userId, next);
}

export function addDailyCoinWatchSeconds(userId: string | null | undefined, seconds: number): void {
  if (typeof window === 'undefined' || !userId || seconds <= 0) return;
  const next = readDailyCoinActivity(userId);
  next.watchSeconds = Math.min(24 * 60 * 60, next.watchSeconds + seconds);
  writeDailyCoinActivity(userId, next);
}

function writeDailyCoinActivity(userId: string, activity: DailyCoinActivity): void {
  activityCache.set(userId, {
    activity: cloneActivity(activity),
    raw: activityCache.get(userId)?.raw ?? window.localStorage.getItem(activityKey(userId)),
    dirty: true,
  });
  scheduleFlush();
}

function activityKey(userId: string): string {
  return `${COIN_ACTIVITY_KEY_PREFIX}:${userId}`;
}

function fallback(): DailyCoinActivity {
  return {
    date: coinTodayKey(),
    watchedRoomIds: [],
    watchSeconds: 0,
  };
}

function cloneActivity(activity: DailyCoinActivity): DailyCoinActivity {
  return {
    date: activity.date,
    watchedRoomIds: [...activity.watchedRoomIds],
    watchSeconds: activity.watchSeconds,
  };
}

function scheduleFlush() {
  installFlushListeners();
  if (typeof window === 'undefined') return;
  if (flushTimer !== null) return;
  flushTimer = window.setTimeout(() => {
    flushTimer = null;
    flushDailyCoinActivity();
  }, WRITE_DEBOUNCE_MS);
}

function installFlushListeners() {
  if (listenersInstalled || typeof window === 'undefined') return;
  listenersInstalled = true;
  window.addEventListener('pagehide', flushDailyCoinActivity);
  window.addEventListener('beforeunload', flushDailyCoinActivity);
}

export function flushDailyCoinActivity(): void {
  if (typeof window === 'undefined') return;
  for (const [userId, cached] of activityCache) {
    if (!cached.dirty) continue;
    const raw = JSON.stringify(cached.activity);
    try {
      window.localStorage.setItem(activityKey(userId), raw);
      activityCache.set(userId, { ...cached, raw, dirty: false });
    } catch {
      // localStorage can be unavailable or full; keep the in-memory copy.
    }
  }
}

export function resetDailyCoinActivityCacheForTest(): void {
  activityCache.clear();
  if (flushTimer !== null && typeof window !== 'undefined') {
    window.clearTimeout(flushTimer);
  }
  flushTimer = null;
}
