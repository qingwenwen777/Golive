import { describe, expect, it } from 'vitest';
import { matchesChannel, resolveChannelCover } from './ChannelPage';
import type { Stream } from '@/types/stream';
import type { User } from '@/types/user';

function makeStream(overrides: Partial<Stream> = {}): Stream {
  return {
    id: 'live-1',
    title: 'Live',
    channel: 'kabun',
    channelId: 'ch-owner-a',
    verified: false,
    avatar: '',
    cover: '',
    viewers: 0,
    duration: '',
    category: 'Gaming',
    startedAt: '2026-05-01T10:00:00Z',
    ownerId: 'owner-a',
    ...overrides,
  };
}

function makeUser(overrides: Partial<User> = {}): User {
  return {
    id: 'owner-a',
    username: 'kabun',
    displayName: 'kabun',
    avatar: '',
    coinBalance: 0,
    role: 'user',
    livePermissionStatus: 'approved',
    ...overrides,
  };
}

describe('resolveChannelCover', () => {
  it('uses the explicit channel cover from the user profile', () => {
    expect(resolveChannelCover(makeUser({ cover: '/api/uploads/covers/channel.webp' }))).toBe(
      '/api/uploads/covers/channel.webp',
    );
  });

  it('does not invent a channel cover when the profile has none', () => {
    expect(resolveChannelCover(makeUser({ cover: '' }))).toBe('');
  });
});

describe('matchesChannel', () => {
  const copycat = makeStream({
    id: 'live-x',
    channel: 'Kabun',
    channelId: 'ch-owner-x',
    ownerId: 'owner-x',
  });

  it("keeps a stream labelled with a user's name out of that user's channel", () => {
    const owner = makeUser();
    expect(matchesChannel(makeStream({ channel: 'Kabun Live' }), 'kabun', owner)).toBe(true);
    expect(matchesChannel(copycat, 'kabun', owner)).toBe(false);
    expect(matchesChannel(copycat, 'ch-owner-a', owner)).toBe(false);
  });

  it('matches labels only for keys that name no user', () => {
    expect(matchesChannel(copycat, 'kabun', null, true)).toBe(false);
    expect(matchesChannel(copycat, 'kabun', null)).toBe(true);
    expect(matchesChannel(copycat, 'ch-owner-x', null, true)).toBe(true);
  });
});
