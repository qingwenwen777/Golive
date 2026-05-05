// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Stream } from '@/types/stream';
import {
  LIKED_STREAMS_KEY,
  WATCH_HISTORY_KEY,
  WATCH_LATER_KEY,
  filterLibraryItemsByLiveRooms,
  isInLibrary,
  markStreamEndedInLibraries,
  readLibrary,
  removeStreamFromLibraries,
  saveToLibrary,
  syncLibraryWithLiveRooms,
} from './liveLibrary';

function stream(id: string, patch: Partial<Stream> = {}): Stream {
  return {
    id,
    title: `Live ${id}`,
    channel: 'Creator',
    channelId: 'channel-1',
    verified: false,
    avatar: '',
    cover: '',
    viewers: 10,
    duration: '0:10:00',
    category: 'Gaming',
    startedAt: '2026-05-05T00:00:00.000Z',
    isLive: true,
    status: 'live',
    playbackUrl: `/live/${id}.flv`,
    streamKey: `sk-${id}`,
    ...patch,
  };
}

describe('liveLibrary', () => {
  beforeEach(() => {
    vi.useRealTimers();
    window.localStorage.clear();
  });

  it('reads only valid stream-like entries from localStorage', () => {
    window.localStorage.setItem(
      WATCH_LATER_KEY,
      JSON.stringify([stream('a'), { id: 'bad' }, null, { id: 'b', title: 'Saved' }]),
    );

    expect(readLibrary(WATCH_LATER_KEY).map((item) => item.id)).toEqual(['a', 'b']);
  });

  it('saves newest streams first, updates duplicates, and caps the list', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-05-05T12:00:00.000Z'));

    for (let i = 0; i < 62; i += 1) {
      saveToLibrary(WATCH_HISTORY_KEY, stream(`room-${i}`), 'watchedAt');
    }
    vi.setSystemTime(new Date('2026-05-05T12:05:00.000Z'));
    const next = saveToLibrary(
      WATCH_HISTORY_KEY,
      stream('room-3', { title: 'Updated room' }),
      'watchedAt',
    );

    expect(next).toHaveLength(60);
    expect(next[0]).toMatchObject({
      id: 'room-3',
      title: 'Updated room',
      watchedAt: '2026-05-05T12:05:00.000Z',
    });
    expect(next.filter((item) => item.id === 'room-3')).toHaveLength(1);
    expect(isInLibrary(WATCH_HISTORY_KEY, 'room-3')).toBe(true);
  });

  it('removes a stream from all configured libraries', () => {
    saveToLibrary(WATCH_HISTORY_KEY, stream('room-1'));
    saveToLibrary(WATCH_LATER_KEY, stream('room-1'));
    saveToLibrary(LIKED_STREAMS_KEY, stream('room-1'));

    removeStreamFromLibraries('room-1');

    expect(isInLibrary(WATCH_HISTORY_KEY, 'room-1')).toBe(false);
    expect(isInLibrary(WATCH_LATER_KEY, 'room-1')).toBe(false);
    expect(isInLibrary(LIKED_STREAMS_KEY, 'room-1')).toBe(false);
  });

  it('marks ended streams as not live and strips playable secrets from libraries', () => {
    saveToLibrary(WATCH_HISTORY_KEY, stream('room-1'));
    saveToLibrary(WATCH_LATER_KEY, stream('room-1'));

    markStreamEndedInLibraries(stream('room-1', { title: 'Final title' }));

    for (const item of [readLibrary(WATCH_HISTORY_KEY)[0], readLibrary(WATCH_LATER_KEY)[0]]) {
      expect(item).toMatchObject({
        id: 'room-1',
        title: 'Final title',
        viewers: 0,
        isLive: false,
        status: 'ended',
      });
      expect(item.playbackUrl).toBeUndefined();
      expect(item.streamKey).toBeUndefined();
    }
  });

  it('merges current live room data while preserving saved timestamps', () => {
    const saved = saveToLibrary(WATCH_LATER_KEY, stream('room-1', { viewers: 1 }))[0];
    const synced = syncLibraryWithLiveRooms(WATCH_LATER_KEY, [
      stream('room-1', { viewers: 42, title: 'Live now' }),
    ]);

    expect(synced[0]).toMatchObject({
      id: 'room-1',
      title: 'Live now',
      viewers: 42,
      savedAt: saved.savedAt,
    });
  });

  it('marks library items ended when their live rooms disappear', () => {
    const [item] = filterLibraryItemsByLiveRooms([stream('room-1')], []);

    expect(item).toMatchObject({
      id: 'room-1',
      viewers: 0,
      isLive: false,
      status: 'ended',
    });
    expect(item.playbackUrl).toBeUndefined();
    expect(item.streamKey).toBeUndefined();
  });
});
