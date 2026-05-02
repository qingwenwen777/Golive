import { describe, expect, it } from 'vitest';
import type { Stream } from '@/types/stream';
import { resolveActiveCreatorLiveId } from './useActiveCreatorLiveId';
import type { PublisherSession } from './publisherSession';

function makeStream(overrides: Partial<Stream> & Pick<Stream, 'id' | 'ownerId' | 'status' | 'isLive'>): Stream {
  return {
    title: 'Test live',
    channel: 'Test Creator',
    channelId: 'ch-test',
    verified: false,
    avatar: '',
    cover: '',
    viewers: 0,
    duration: '0:00:00',
    category: 'Just Chatting',
    startedAt: '2026-05-02T00:00:00Z',
    ...overrides,
  };
}

describe('resolveActiveCreatorLiveId', () => {
  it('prefers the current owner live room over a stale foreign session', () => {
    const items = [
      makeStream({ id: 'room-a', ownerId: 'owner-a', status: 'live', isLive: true }),
      makeStream({ id: 'room-b', ownerId: 'owner-b', status: 'live', isLive: true }),
    ];
    const session: PublisherSession = {
      streamId: 'room-b',
      streamKey: 'lk_b',
      ownerId: 'owner-b',
      rtmpServer: 'rtmp://localhost/live',
      createdAt: '2026-05-02T08:00:00.000Z',
    };

    expect(resolveActiveCreatorLiveId(items, 'owner-a', session)).toBe('room-a');
  });

  it('ignores sessions without an owner binding', () => {
    const session = {
      streamId: 'room-z',
      streamKey: 'lk_z',
      rtmpServer: 'rtmp://localhost/live',
      createdAt: '2026-05-02T08:00:00.000Z',
    } as PublisherSession;

    expect(resolveActiveCreatorLiveId([], 'owner-a', session)).toBe('');
  });

  it('accepts an owner publishing room before it appears in the live list', () => {
    const session: PublisherSession = {
      streamId: 'room-fresh',
      streamKey: 'lk_fresh',
      ownerId: 'owner-a',
      rtmpServer: 'rtmp://localhost/live',
      createdAt: '2026-05-02T08:00:00.000Z',
    };
    const sessionRoom = makeStream({
      id: 'room-fresh',
      ownerId: 'owner-a',
      status: 'publishing',
      isLive: false,
    });

    expect(resolveActiveCreatorLiveId([], 'owner-a', session, sessionRoom)).toBe('room-fresh');
  });
});
