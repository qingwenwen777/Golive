import { isPlaceholderName, unknownCreatorName, userName, type User } from './user';

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
  fanClubOnly?: boolean;
  fanClubMember?: boolean;
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

// streamChannelName is the stream's channel name, the viewer's own name on
// their own stream, or a translated "Unknown creator" (a placeholder: see
// isPlaceholderName).
export function streamChannelName(
  stream: Pick<Stream, 'channel' | 'ownerId'>,
  viewer?: Pick<User, 'displayName' | 'username' | 'id'> | null,
): string {
  const raw = stream.channel?.trim();
  if (raw && !isPlaceholderName(raw)) return raw;
  if (viewer && stream.ownerId && viewer.id === stream.ownerId) {
    const own = userName(viewer);
    if (own) return own;
  }
  return unknownCreatorName();
}
