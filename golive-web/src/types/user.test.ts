import { describe, expect, it } from 'vitest';

import { isUuidLike, userDisplayName } from './user';

const uuid = '550e8400-e29b-41d4-a716-446655440000';

describe('user helpers', () => {
  it('recognizes UUID-like usernames with surrounding whitespace', () => {
    expect(isUuidLike(` ${uuid} `)).toBe(true);
    expect(isUuidLike('streamer')).toBe(false);
    expect(isUuidLike(null)).toBe(false);
  });

  it('chooses the most human-readable display name', () => {
    expect(userDisplayName(null)).toBe('You');
    expect(userDisplayName({ id: 'u1', username: 'streamer', displayName: 'Streamer' })).toBe(
      'Streamer',
    );
    expect(userDisplayName({ id: 'u1', username: 'streamer', displayName: '  ' })).toBe('streamer');
    expect(userDisplayName({ id: uuid, username: uuid, displayName: '' })).toBe('Creator 550e8400');
  });
});
