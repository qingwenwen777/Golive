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
});
