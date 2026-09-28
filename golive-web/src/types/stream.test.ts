import { describe, expect, it } from 'vitest';

import { streamChannelName } from './stream';

const uuid = '550e8400-e29b-41d4-a716-446655440000';

describe('stream helpers', () => {
  it('uses a real channel name before falling back to the viewer or a label', () => {
    expect(streamChannelName({ channel: 'GoLive Studio', ownerId: uuid })).toBe('GoLive Studio');
    expect(
      streamChannelName(
        { channel: uuid, ownerId: 'owner-1' },
        { id: 'owner-1', displayName: 'Streamer', username: 'streamer' },
      ),
    ).toBe('Streamer');
    // Never an id: not the channel's UUID, nor the label older servers stored.
    expect(streamChannelName({ channel: uuid, ownerId: 'owner-1' }, null)).toBe('Unknown creator');
    expect(streamChannelName({ channel: 'Creator 550e8400', ownerId: uuid })).toBe(
      'Unknown creator',
    );
    expect(streamChannelName({ channel: '', ownerId: undefined }, null)).toBe('Unknown creator');
    expect(
      streamChannelName({ channel: '', ownerId: 'owner-1' }, { id: 'owner-1', username: uuid }),
    ).toBe('Unknown creator');
  });
});
