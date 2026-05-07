// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { generatedImagePlaceholderStyle, LoadableImage } from './LoadableImage';

describe('LoadableImage', () => {
  afterEach(() => {
    cleanup();
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
    expect(image.className).toContain('has-generated-placeholder');
    expect(image.getAttribute('style')).toContain('radial-gradient');

    act(() => {
      callbacks[0]?.(
        [{ isIntersecting: true, intersectionRatio: 1 } as IntersectionObserverEntry],
        {} as IntersectionObserver,
      );
    });

    expect(image.getAttribute('src')).toBe('/lazy-cover.jpg');
    expect(disconnect).toHaveBeenCalled();
  });

  it('generates a stable blurred placeholder when no low-res asset is provided', () => {
    const first = generatedImagePlaceholderStyle('/covers/live-a.jpg');
    const second = generatedImagePlaceholderStyle('/covers/live-a.jpg');
    const other = generatedImagePlaceholderStyle('/covers/live-b.jpg');

    expect(first).toBe(second);
    expect(first.backgroundImage).toContain('radial-gradient');
    expect(first.backgroundImage).not.toBe(other.backgroundImage);
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

  it('does not treat a low-res placeholder load as the final image load', () => {
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
    const onLoad = vi.fn();
    const onReady = vi.fn();

    const view = render(
      <LoadableImage
        src="/full-cover.jpg"
        placeholderSrc="/tiny-cover.jpg"
        alt="progressive cover"
        loading="lazy"
        onLoad={onLoad}
        onReady={onReady}
      />,
    );

    fireEvent.load(view.getByAltText('progressive cover'));

    expect(onLoad).not.toHaveBeenCalled();
    expect(onReady).not.toHaveBeenCalled();
  });

  it('falls back to the generated placeholder if the low-res asset fails', () => {
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
    const onError = vi.fn();

    const view = render(
      <LoadableImage
        src="/full-cover.jpg"
        placeholderSrc="/missing-tiny-cover.jpg"
        alt="progressive cover"
        loading="lazy"
        onError={onError}
      />,
    );

    const image = view.getByAltText('progressive cover');
    fireEvent.error(image);

    expect(onError).not.toHaveBeenCalled();
    expect(image.getAttribute('src')).toBeNull();
    expect(image.className).toContain('has-generated-placeholder');
    expect(image.className).not.toContain('is-error');
  });

  it('keeps the original image source while probing optimized formats', async () => {
    vi.stubGlobal('navigator', { userAgent: 'Chrome' });
    vi.stubGlobal(
      'Image',
      class {
        decoding = 'async';
        onload: (() => void) | null = null;
        onerror: (() => void) | null = null;
        src = '';
      },
    );

    render(<LoadableImage src="/avatar.jpg" alt="avatar" />);

    await act(async () => {
      await Promise.resolve();
    });

    const image = screen.getByAltText('avatar');
    expect(image.getAttribute('src')).toBe('/avatar.jpg');
    expect(image.className).toContain('is-loading');
  });
});
