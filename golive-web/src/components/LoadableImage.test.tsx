// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { LoadableImage } from './LoadableImage';

describe('LoadableImage', () => {
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
});
