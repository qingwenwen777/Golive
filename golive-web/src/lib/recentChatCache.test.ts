import { beforeEach, describe, expect, it, vi } from 'vitest';
import { loadRecentChatMessages, saveRecentChatMessage } from './recentChatCache';
import type { ChatMessage } from '@/types/message';

function message(id: string, ts: number): ChatMessage {
  return {
    id,
    kind: 'chat',
    user: 'xhb',
    text: `message ${id}`,
    ts,
  };
}

describe('recentChatCache', () => {
  let store: Record<string, string>;

  beforeEach(() => {
    store = {};
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => store[key] ?? null,
      setItem: (key: string, value: string) => {
        store[key] = value;
      },
    });
  });

  it('keeps the newest fifty messages per room in timestamp order', () => {
    for (let i = 1; i <= 55; i += 1) {
      saveRecentChatMessage('room-1', message(String(i), i), 55);
    }

    const loaded = loadRecentChatMessages('room-1', 55);

    expect(loaded).toHaveLength(50);
    expect(loaded[0].id).toBe('6');
    expect(loaded.at(-1)?.id).toBe('55');
  });

  it('deduplicates by id and drops messages older than 24 hours', () => {
    const now = 100 * 60 * 60 * 1000;
    saveRecentChatMessage('room-1', message('old', now - 25 * 60 * 60 * 1000), now);
    saveRecentChatMessage('room-1', message('same', now - 1000), now);
    saveRecentChatMessage('room-1', { ...message('same', now), text: 'new text' }, now);

    const loaded = loadRecentChatMessages('room-1', now);

    expect(loaded).toHaveLength(1);
    expect(loaded[0]).toMatchObject({ id: 'same', text: 'new text' });
  });
});
