// @vitest-environment jsdom
import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useWebSocket } from './useWebSocket';

let sockets: FakeWebSocket[] = [];

class FakeWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;

  readonly url: string;
  readonly protocols?: string | string[];
  readyState = FakeWebSocket.CONNECTING;
  sent: string[] = [];
  closeCalls: Array<{ code?: number; reason?: string }> = [];
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;

  constructor(url: string, protocols?: string | string[]) {
    this.url = url;
    this.protocols = protocols;
    sockets.push(this);
  }

  open() {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen?.(new Event('open'));
  }

  receive(data: string) {
    this.onmessage?.({ data } as MessageEvent);
  }

  send(data: string) {
    if (this.readyState !== FakeWebSocket.OPEN) throw new Error('socket not open');
    this.sent.push(data);
  }

  close(code?: number, reason?: string) {
    this.readyState = FakeWebSocket.CLOSED;
    this.closeCalls.push({ code, reason });
    this.onclose?.({ code: code ?? 1000, reason: reason ?? '' } as CloseEvent);
  }
}

describe('useWebSocket', () => {
  beforeEach(() => {
    sockets = [];
    vi.useFakeTimers();
    vi.stubGlobal('WebSocket', FakeWebSocket);
    // No reconnect jitter unless a test asks for it.
    vi.spyOn(Math, 'random').mockReturnValue(0);
  });

  afterEach(() => {
    cleanup();
    vi.clearAllTimers();
    vi.useRealTimers();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('offers the token as a subprotocol, receives messages, and serializes outgoing payloads', () => {
    const token = 'eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1LTEifQ.c2lnbmF0dXJl';
    const onMessage = vi.fn();
    const { result } = renderHook(() =>
      useWebSocket('ws://example.test/ws?room=1', {
        heartbeatMs: 0,
        onMessage,
        token,
      }),
    );

    // Never in the URL: that ends up in access logs.
    expect(sockets[0].url).toBe('ws://example.test/ws?room=1');
    expect(sockets[0].protocols).toEqual(['golive.v1', `auth.${token}`]);
    act(() => {
      sockets[0].open();
    });

    expect(result.current.readyState).toBe('open');
    act(() => {
      sockets[0].receive('hello');
    });

    expect(result.current.lastMessage?.data).toBe('hello');
    expect(onMessage).toHaveBeenCalledWith(expect.objectContaining({ data: 'hello' }));
    expect(result.current.sendMessage({ type: 'chat', text: 'hi' })).toBe(true);
    expect(result.current.sendMessage('raw')).toBe(true);
    expect(sockets[0].sent).toEqual([JSON.stringify({ type: 'chat', text: 'hi' }), 'raw']);
  });

  it('offers only the gateway protocol without a token', () => {
    renderHook(() => useWebSocket('ws://example.test/ws?room=1', { heartbeatMs: 0, token: null }));

    expect(sockets[0].url).toBe('ws://example.test/ws?room=1');
    expect(sockets[0].protocols).toEqual(['golive.v1']);
  });

  it('reconnects unexpected closes with backoff and resets retries after a successful open', () => {
    const { result } = renderHook(() =>
      useWebSocket('ws://example.test/ws', { heartbeatMs: 0, maxRetries: 2 }),
    );

    act(() => {
      sockets[0].open();
    });
    act(() => {
      sockets[0].close(1006, 'network-drop');
    });

    expect(result.current.readyState).toBe('reconnecting');
    expect(result.current.retryCount).toBe(1);

    act(() => {
      vi.advanceTimersByTime(999);
    });
    expect(sockets).toHaveLength(1);

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(sockets).toHaveLength(2);
    expect(result.current.readyState).toBe('connecting');

    act(() => {
      sockets[1].open();
    });
    expect(result.current.retryCount).toBe(0);
    expect(result.current.readyState).toBe('open');
  });

  // Every viewer of a restarted gateway used to come back on the same
  // 1s, 2s, 4s... schedule; a random cut of up to half of each step spreads
  // them out.
  it('jitters reconnect delays and caps them at 30s, however many attempts fail', () => {
    vi.mocked(Math.random).mockReturnValue(0.5);
    const { result } = renderHook(() =>
      useWebSocket('ws://example.test/ws', { heartbeatMs: 0, maxRetries: Infinity }),
    );

    // A quarter off each step: 1s, 2s, 4s, 8s, 16s, then 30s for good.
    const delays = [750, 1500, 3000, 6000, 12000, 22500, 22500, 22500, 22500, 22500, 22500, 22500];
    delays.forEach((delay, attempt) => {
      act(() => {
        sockets[attempt].close(1006, 'handshake refused');
      });
      expect(result.current.readyState).toBe('reconnecting');
      act(() => {
        vi.advanceTimersByTime(delay - 1);
      });
      expect(sockets).toHaveLength(attempt + 1);
      act(() => {
        vi.advanceTimersByTime(1);
      });
      expect(sockets).toHaveLength(attempt + 2);
    });
    expect(result.current.retryCount).toBe(delays.length);
  });

  it('does not reconnect after a manual disconnect', () => {
    const { result } = renderHook(() =>
      useWebSocket('ws://example.test/ws', { heartbeatMs: 0, maxRetries: 2 }),
    );
    act(() => {
      sockets[0].open();
    });

    act(() => {
      result.current.disconnect(3001, 'manual');
    });

    expect(sockets[0].closeCalls[0]).toEqual({ code: 3001, reason: 'manual' });
    expect(result.current.readyState).toBe('closed');

    act(() => {
      vi.advanceTimersByTime(10_000);
    });
    expect(sockets).toHaveLength(1);
  });

  it('opens a fresh socket when the auth token changes and ignores the stale close', () => {
    const { rerender, result } = renderHook(
      ({ token }: { token: string }) =>
        useWebSocket('ws://example.test/ws', { heartbeatMs: 0, token }),
      { initialProps: { token: 'old-token' } },
    );
    const first = sockets[0];
    act(() => {
      first.open();
    });

    rerender({ token: 'new-token' });

    expect(first.closeCalls[0]).toEqual({ code: 1000, reason: 'unmount' });
    expect(sockets).toHaveLength(2);
    expect(sockets[1].url).toBe('ws://example.test/ws');
    expect(sockets[1].protocols).toEqual(['golive.v1', 'auth.new-token']);
    expect(result.current.readyState).toBe('connecting');
  });
});
