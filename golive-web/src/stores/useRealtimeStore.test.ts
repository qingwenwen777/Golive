import { beforeEach, describe, expect, it } from 'vitest';
import { useRealtimeStore } from './useRealtimeStore';
import type { ChatMessage } from '@/types/message';

function chat(id: string, patch: Partial<ChatMessage> = {}): ChatMessage {
  return {
    id,
    kind: 'chat',
    user: 'xhb',
    text: 'hello',
    ts: 1,
    ...patch,
  };
}

describe('useRealtimeStore', () => {
  beforeEach(() => {
    useRealtimeStore.setState({ rooms: {} });
  });

  it('keeps fan badge metadata when history returns the same chat without it', () => {
    const badge = { creatorId: 'creator-1', level: 3 };
    useRealtimeStore.getState().mergeMessages('room-1', [chat('m1', { fanBadge: badge })]);
    useRealtimeStore.getState().mergeMessages('room-1', [chat('m1')]);

    expect(useRealtimeStore.getState().rooms['room-1'].messages[0]).toMatchObject({
      id: 'm1',
      fanBadge: badge,
    });
  });

  it('adds fan badge metadata when the newer copy has it', () => {
    const badge = { creatorId: 'creator-1', level: 3 };
    useRealtimeStore.getState().mergeMessages('room-1', [chat('m1')]);
    useRealtimeStore.getState().mergeMessages('room-1', [chat('m1', { fanBadge: badge })]);

    expect(useRealtimeStore.getState().rooms['room-1'].messages[0]).toMatchObject({
      id: 'm1',
      fanBadge: badge,
    });
  });

  it('uses a message index to merge duplicate appends in place', () => {
    useRealtimeStore.getState().appendMessage('room-1', chat('m1', { text: 'first' }));
    useRealtimeStore.getState().appendMessage('room-1', chat('m1', { text: 'second' }));

    const slice = useRealtimeStore.getState().rooms['room-1'];
    expect(slice.messages).toHaveLength(1);
    expect(slice.messages[0]).toMatchObject({ id: 'm1', text: 'second' });
    expect(slice.messageIndex.m1).toBe(0);
  });

  it('does not let a message from another user rewrite one with the same id', () => {
    useRealtimeStore
      .getState()
      .appendMessage('room-1', chat('m1', { userId: 'u1', user: 'Luna', text: 'original' }));
    useRealtimeStore
      .getState()
      .appendMessage('room-1', chat('m1', { userId: 'u2', user: 'Luna', text: 'rewritten' }));
    useRealtimeStore
      .getState()
      .mergeMessages('room-1', [chat('m1', { userId: 'u3', user: 'Host', text: 'rewritten' })]);

    const slice = useRealtimeStore.getState().rooms['room-1'];
    expect(slice.messages).toHaveLength(1);
    expect(slice.messages[0]).toMatchObject({ id: 'm1', userId: 'u1', text: 'original' });
  });

  it('optimistically updates viewer contribution ranking', () => {
    useRealtimeStore.getState().setViewers('room-1', [
      { userId: 'u1', user: 'aaaa', contribution: 20 },
      { userId: 'u2', user: 'bbbb', contribution: 10 },
    ]);

    useRealtimeStore
      .getState()
      .incrementViewerContribution(
        'room-1',
        { userId: 'u1', user: 'aaaa', avatar: '/a.png', contribution: 0 },
        50,
      );

    expect(useRealtimeStore.getState().rooms['room-1'].viewers).toEqual([
      { userId: 'u1', user: 'aaaa', avatar: '/a.png', contribution: 70 },
      { userId: 'u2', user: 'bbbb', contribution: 10 },
    ]);
  });

  it('adds the sender to viewer ranking if the server list has not arrived yet', () => {
    useRealtimeStore
      .getState()
      .incrementViewerContribution('room-1', { userId: 'u1', user: 'aaaa', contribution: 0 }, 50);

    expect(useRealtimeStore.getState().rooms['room-1'].viewers).toMatchObject([
      { userId: 'u1', user: 'aaaa', contribution: 50 },
    ]);
  });
});
