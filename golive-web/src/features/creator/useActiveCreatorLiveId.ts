import { useRoom, useRooms } from '@/api/room';
import type { Stream } from '@/types/stream';
import {
  isPublisherSessionForUser,
  loadPublisherSession,
  type PublisherSession,
} from './publisherSession';

function isLiveStream(stream: Pick<Stream, 'isLive' | 'status'>): boolean {
  return stream.isLive === true || stream.status === 'live';
}

function isActivePublisherRoom(stream: Pick<Stream, 'isLive' | 'status'>): boolean {
  return isLiveStream(stream) || stream.status === 'publishing';
}

export function resolveActiveCreatorLiveId(
  items: Stream[],
  userId?: string,
  session: PublisherSession | null = loadPublisherSession(),
  sessionRoom: Stream | null = null,
): string {
  const liveByUser = userId
    ? items.find((item) => item.ownerId === userId && isLiveStream(item))
    : undefined;
  if (liveByUser) return liveByUser.id;

  if (!isPublisherSessionForUser(session, userId)) return '';

  const liveBySession = session?.streamId
    ? items.find((item) => item.id === session.streamId && item.ownerId === userId && isLiveStream(item))
    : undefined;
  if (liveBySession) return liveBySession.id;

  if (
    sessionRoom &&
    sessionRoom.id === session.streamId &&
    sessionRoom.ownerId === userId &&
    isActivePublisherRoom(sessionRoom)
  ) {
    return sessionRoom.id;
  }

  return '';
}

export function useActiveCreatorLiveId(userId?: string): string {
  const liveRooms = useRooms({ size: 100 });
  const session = loadPublisherSession();
  const sessionRoom = useRoom(session?.streamId ?? '', isPublisherSessionForUser(session, userId));

  return resolveActiveCreatorLiveId(
    liveRooms.data?.items ?? [],
    userId,
    session,
    sessionRoom.data ?? null,
  );
}
