import i18next from 'i18next';

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
  email?: string;
  displayName?: string;
  usernameUpdatedAt?: string;
  usernameChangeAvailableAt?: string;
  avatar: string;
  cover?: string;
  coinBalance: number;
  frozenCoins?: number;
  levelInfo?: UserLevelInfo;
  verified?: boolean;
  banned?: boolean;
  banReason?: string;
  googleLinked?: boolean;
  role: 'user' | 'admin' | 'moderator';
  livePermissionStatus: 'none' | 'pending' | 'approved' | 'rejected';
  livePermissionRejectReason?: string;
  platformVerificationStatus?: 'none' | 'pending' | 'approved' | 'rejected';
  platformVerificationRejectReason?: string;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function isUuidLike(s: string | null | undefined): boolean {
  return !!s && UUID_RE.test(s.trim());
}

// Labels for someone without a name to show: no display name and only a
// UUID for a username, a profile that could not be loaded, or (guest) no
// account. They follow the UI language and are only for display; the server
// gets "" instead.
function nameLabel(key: string, defaultValue: string): string {
  return i18next.isInitialized ? i18next.t(key, { defaultValue }) : defaultValue;
}

export function unknownUserName(): string {
  return nameLabel('common:names.unknownUser', 'Unknown user');
}

export function unknownCreatorName(): string {
  return nameLabel('common:names.unknownCreator', 'Unknown creator');
}

export function guestName(): string {
  return nameLabel('common:names.guest', 'Guest');
}

// isPlaceholderName reports whether name is not a real name: blank, a bare
// UUID, one of the labels above, or the label older servers made up from an
// id ("Creator 1a2b3c4d").
export function isPlaceholderName(name: string | null | undefined): boolean {
  const trimmed = name?.trim() ?? '';
  if (!trimmed || isUuidLike(trimmed)) return true;
  if (/^creator\s+[0-9a-f-]{6,}$/i.test(trimmed)) return true;
  return trimmed === unknownUserName() || trimmed === unknownCreatorName();
}

// personName shows a person's name from the server, or "Unknown user" when
// it is a placeholder; creatorName does the same for creators and channels.
export function personName(name: string | null | undefined): string {
  return isPlaceholderName(name) ? unknownUserName() : (name ?? '').trim();
}

export function creatorName(name: string | null | undefined): string {
  return isPlaceholderName(name) ? unknownCreatorName() : (name ?? '').trim();
}

// userName is the name a user goes by: their display name, else their
// username unless it is only a UUID, else "". This, never a label, is what
// goes to the server.
export function userName(u: Pick<User, 'displayName' | 'username'> | null | undefined): string {
  const displayName = u?.displayName?.trim();
  if (displayName) return displayName;
  const username = u?.username?.trim();
  return username && !isUuidLike(username) ? username : '';
}

// userDisplayName is userName for display: "Unknown user" when there is no
// name, and "You" when there is no user.
export function userDisplayName(
  u: Pick<User, 'displayName' | 'username' | 'id'> | null | undefined,
): string {
  if (!u) return nameLabel('common:account.you', 'You');
  return userName(u) || unknownUserName();
}

export interface LoginResp {
  token: string;
  user: User;
}
