export const COIN_ACTIVITY_KEY_PREFIX = 'golive-coin-activity';
const BEIJING_OFFSET_MS = 8 * 60 * 60 * 1000;

export interface DailyCoinActivity {
  date: string;
  watchedRoomIds: string[];
  watchSeconds: number;
}

export function coinTodayKey(date = new Date()): string {
  const beijingDate = new Date(date.getTime() + BEIJING_OFFSET_MS);
  const year = beijingDate.getUTCFullYear();
  const month = String(beijingDate.getUTCMonth() + 1).padStart(2, '0');
  const day = String(beijingDate.getUTCDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function readDailyCoinActivity(userId: string | null | undefined): DailyCoinActivity {
  const fallback: DailyCoinActivity = {
    date: coinTodayKey(),
    watchedRoomIds: [],
    watchSeconds: 0,
  };
  if (typeof window === 'undefined' || !userId) return fallback;
  try {
    const raw = window.localStorage.getItem(activityKey(userId));
    if (!raw) return fallback;
    const parsed = JSON.parse(raw) as Partial<DailyCoinActivity>;
    if (parsed.date !== fallback.date) return fallback;
    return {
      date: fallback.date,
      watchedRoomIds: Array.isArray(parsed.watchedRoomIds)
        ? parsed.watchedRoomIds.filter((id): id is string => typeof id === 'string')
        : [],
      watchSeconds: Number.isFinite(parsed.watchSeconds) ? Number(parsed.watchSeconds) : 0,
    };
  } catch {
    return fallback;
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
  window.localStorage.setItem(activityKey(userId), JSON.stringify(activity));
}

function activityKey(userId: string): string {
  return `${COIN_ACTIVITY_KEY_PREFIX}:${userId}`;
}
