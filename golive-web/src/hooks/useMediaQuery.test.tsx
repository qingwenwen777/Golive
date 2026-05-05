// @vitest-environment jsdom
import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useMediaQuery } from './useMediaQuery';

type Listener = (event: MediaQueryListEvent) => void;

class MatchMediaController {
  matches = false;
  listeners = new Set<Listener>();

  matchMedia = vi.fn((query: string) => ({
    media: query,
    matches: this.matches,
    addEventListener: vi.fn((_type: 'change', listener: Listener) => {
      this.listeners.add(listener);
    }),
    removeEventListener: vi.fn((_type: 'change', listener: Listener) => {
      this.listeners.delete(listener);
    }),
  }));

  setMatches(matches: boolean) {
    this.matches = matches;
    for (const listener of this.listeners) {
      listener({ matches } as MediaQueryListEvent);
    }
  }
}

describe('useMediaQuery', () => {
  let media: MatchMediaController;

  beforeEach(() => {
    media = new MatchMediaController();
    vi.stubGlobal('matchMedia', media.matchMedia);
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it('returns the initial match and reacts to media query changes', () => {
    media.matches = true;
    const { result } = renderHook(() => useMediaQuery('(max-width: 767px)'));

    expect(result.current).toBe(true);

    act(() => {
      media.setMatches(false);
    });

    expect(result.current).toBe(false);
  });

  it('removes the change listener on unmount', () => {
    const { unmount } = renderHook(() => useMediaQuery('(max-width: 767px)'));
    expect(media.listeners.size).toBe(1);

    unmount();

    expect(media.listeners.size).toBe(0);
  });
});
