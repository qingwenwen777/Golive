// @vitest-environment jsdom
import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { LoadableImage } from './LoadableImage';

describe('LoadableImage', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('keeps a previously loaded image visible on remount', () => {
    const src = `/cached-${Date.now()}-${Math.random().toString(36).slice(2)}.jpg`;

    const first = render(<LoadableImage src={src} alt="cover" />);
    const image = screen.getByAltText('cover');
    expect(image.className).not.toContain('is-loaded');

    fireEvent.load(image);
    expect(image.className).toContain('is-loaded');

    first.unmount();

    render(<LoadableImage src={src} alt="cover" />);
    expect(screen.getByAltText('cover').className).toContain('is-loaded');
  });

  it('defers lazy images until they are near the viewport', () => {
    const callbacks: IntersectionObserverCallback[] = [];
    const observe = vi.fn();
    const disconnect = vi.fn();
    class MockIntersectionObserver {
      readonly root = null;
      readonly rootMargin = '';
      readonly thresholds = [];
      observe = observe;
      disconnect = disconnect;
      unobserve = vi.fn();
      takeRecords = vi.fn(() => []);

      constructor(callback: IntersectionObserverCallback) {
        callbacks.push(callback);
      }
    }
    vi.stubGlobal('IntersectionObserver', MockIntersectionObserver);

    render(<LoadableImage src="/lazy-cover.jpg" alt="lazy cover" loading="lazy" />);
    const image = screen.getByAltText('lazy cover');
    expect(image.getAttribute('src')).toBeNull();
    expect(image.className).toContain('is-lazy-pending');

    act(() => {
      callbacks[0]?.(
        [{ isIntersecting: true, intersectionRatio: 1 } as IntersectionObserverEntry],
        {} as IntersectionObserver,
      );
    });

    expect(image.getAttribute('src')).toBe('/lazy-cover.jpg');
    expect(disconnect).toHaveBeenCalled();
  });

  it('shows a blurred placeholder before a lazy image is requested', () => {
    class MockIntersectionObserver {
      readonly root = null;
      readonly rootMargin = '';
      readonly thresholds = [];
      observe = vi.fn();
      disconnect = vi.fn();
      unobserve = vi.fn();
      takeRecords = vi.fn(() => []);
    }
    vi.stubGlobal('IntersectionObserver', MockIntersectionObserver);

    render(
      <LoadableImage
        src="/full-cover.jpg"
        placeholderSrc="/tiny-cover.jpg"
        alt="progressive cover"
        loading="lazy"
      />,
    );

    const image = screen.getByAltText('progressive cover');
    expect(image.getAttribute('src')).toBe('/tiny-cover.jpg');
    expect(image.className).toContain('has-placeholder');
  });
});
