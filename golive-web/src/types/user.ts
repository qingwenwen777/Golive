export interface UserLevelInfo {
  level: number;
  maxLevel: number;
  totalTopupCoins: number;
  currentLevelMinCoins: number;
  nextLevelTargetCoins: number;
  coinsToNextLevel: number;
}

export interface User {
  id: string;
  username: string;
  displayName?: string;
  usernameUpdatedAt?: string;
  usernameChangeAvailableAt?: string;
  avatar: string;
  cover?: string;
  coinBalance: number;
  levelInfo?: UserLevelInfo;
  verified?: boolean;
  googleLinked?: boolean;
  role: 'user' | 'admin';
  livePermissionStatus: 'none' | 'pending' | 'approved' | 'rejected';
  livePermissionRejectReason?: string;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function isUuidLike(s: string | null | undefined): boolean {
  return !!s && UUID_RE.test(s.trim());
}

// userDisplayName picks the best human-readable label for a user, preferring
// displayName, then username, then "Creator xxxx" as a last resort if the
// only value we have is a UUID.
export function userDisplayName(
  u: Pick<User, 'displayName' | 'username' | 'id'> | null | undefined,
): string {
  if (!u) return 'You';
  if (u.displayName && u.displayName.trim()) return u.displayName;
  if (u.username && !isUuidLike(u.username)) return u.username;
  if (u.id) return `Creator ${u.id.slice(0, 8)}`;
  return 'You';
}

export interface LoginResp {
  token: string;
  refreshToken: string;
  user: User;
}
