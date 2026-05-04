import type { Stream } from '@/types/stream';

export const LIVE_SESSION_STORAGE_KEY = 'golive-live-session';

export interface PublisherSession {
  streamId: string;
  streamKey: string;
  ownerId?: string;
  playbackUrl?: string;
  rtmpServer: string;
  createdAt: string;
}

export function publisherSessionFromStream(stream: Stream): PublisherSession | null {
  if (!stream.streamKey) return null;
  const rtmpServer = (import.meta.env.VITE_RTMP_BASE || 'rtmp://localhost/live').replace(/\/$/, '');
  return {
    streamId: stream.id,
    streamKey: stream.streamKey,
    ownerId: stream.ownerId,
    playbackUrl: stream.playbackUrl,
    rtmpServer,
    createdAt: new Date().toISOString(),
  };
}

export function savePublisherSession(stream: Stream) {
  const session = publisherSessionFromStream(stream);
  if (!session) return;
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(LIVE_SESSION_STORAGE_KEY, JSON.stringify(session));
}

export function loadPublisherSession(): PublisherSession | null {
  if (typeof window === 'undefined') return null;
  try {
    const raw = window.localStorage.getItem(LIVE_SESSION_STORAGE_KEY);
    return raw ? (JSON.parse(raw) as PublisherSession) : null;
  } catch {
    return null;
  }
}

export function clearPublisherSession() {
  if (typeof window === 'undefined') return;
  window.localStorage.removeItem(LIVE_SESSION_STORAGE_KEY);
}

export function isPublisherSessionForUser(
  session: PublisherSession | null | undefined,
  userId?: string,
): session is PublisherSession & { ownerId: string } {
  return Boolean(session?.ownerId && userId && session.ownerId === userId);
}
