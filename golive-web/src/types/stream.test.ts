import { describe, expect, it } from 'vitest';

import { isPlaceholderChannelName, streamChannelName } from './stream';

const uuid = '550e8400-e29b-41d4-a716-446655440000';

describe('stream helpers', () => {
  it('uses a real channel name before falling back to viewer or creator labels', () => {
    expect(streamChannelName({ channel: 'GoLive Studio', ownerId: uuid })).toBe('GoLive Studio');
    expect(
      streamChannelName(
        { channel: uuid, ownerId: 'owner-1' },
        { id: 'owner-1', displayName: 'Streamer', username: 'streamer' },
      ),
    ).toBe('Streamer');
    expect(streamChannelName({ channel: uuid, ownerId: 'owner-1' }, null)).toBe('Creator 550e8400');
    expect(streamChannelName({ channel: '', ownerId: undefined }, null)).toBe('Creator');
  });

  it.each([
    ['', true],
    ['   ', true],
    [uuid, true],
    ['creator', true],
    ['Creator abcdef', true],
    ['Creator Studio', false],
    ['GoLive Studio', false],
  ])('detects whether %j is a placeholder channel name', (name, expected) => {
    expect(isPlaceholderChannelName(name)).toBe(expected);
  });
});
