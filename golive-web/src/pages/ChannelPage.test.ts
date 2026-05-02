import { describe, expect, it } from 'vitest';
import { resolveChannelCover } from './ChannelPage';
import type { User } from '@/types/user';

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
