// @vitest-environment jsdom
import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useInterval } from './useInterval';

describe('useInterval', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    cleanup();
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it('runs the latest callback without recreating the interval', () => {
    const first = vi.fn();
    const second = vi.fn();
    const { rerender } = renderHook(
      ({ callback, delay }: { callback: () => void; delay: number | null }) =>
        useInterval(callback, delay),
      { initialProps: { callback: first, delay: 1000 } },
    );

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(first).toHaveBeenCalledTimes(1);

    rerender({ callback: second, delay: 1000 });
    act(() => {
      vi.advanceTimersByTime(1000);
    });

    expect(first).toHaveBeenCalledTimes(1);
    expect(second).toHaveBeenCalledTimes(1);
  });

  it('does not schedule a timer when delay is null', () => {
    const callback = vi.fn();
    renderHook(() => useInterval(callback, null));

    act(() => {
      vi.advanceTimersByTime(5000);
    });

    expect(callback).not.toHaveBeenCalled();
  });
});
