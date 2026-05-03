import { isUuidLike, userDisplayName, type User } from './user';

export interface Stream {
  id: string;
  title: string;
  titleJa?: string;
  description?: string;
  channel: string;
  channelId: string;
  verified: boolean;
  avatar: string;
  cover: string;
  viewers: number;
  peakViewers?: number;
  duration: string;
  category: string;
  categoryJa?: string;
  startedAt: string;
  endedAt?: string;
  isLive?: boolean;
  ownerId?: string;
  playbackUrl?: string;
  streamKey?: string;
  status?:
    | 'scheduled'
    | 'publishing'
    | 'live'
    | 'ending'
    | 'ended'
    | 'expired'
    | 'canceled'
    | string;
  subscriberCount?: number;
  replay?: ReplayInfo;
}

export type ReplayStatus =
  | 'none'
  | 'pending'
  | 'uploading'
  | 'processing'
  | 'ready'
  | 'failed'
  | 'deleted'
  | string;

export type ReplayVisibility = 'public' | 'followers' | 'private' | string;

export interface ReplayInfo {
  roomId: string;
  status: ReplayStatus;
  visibility: ReplayVisibility;
  uploadAfterEnd: boolean;
  embedUrl?: string;
  bunnyVideoId?: string;
  uploadedAt?: string;
  deletedAt?: string;
  error?: string;
  canWatch: boolean;
  canManage: boolean;
}

export interface PaginatedRooms {
  items: Stream[];
  total: number;
  page: number;
  size: number;
}

export interface RoomsQuery {
  category?: string;
  page?: number;
  size?: number;
}

export function streamChannelName(
  stream: Pick<Stream, 'channel' | 'ownerId'>,
  viewer?: Pick<User, 'displayName' | 'username' | 'id'> | null,
): string {
  const raw = stream.channel?.trim();
  if (raw && !isUuidLike(raw)) return raw;
  if (viewer && stream.ownerId && viewer.id === stream.ownerId) return userDisplayName(viewer);
  const fallback = raw || stream.ownerId;
  if (fallback) return `Creator ${fallback.slice(0, 8)}`;
  return 'Creator';
}
