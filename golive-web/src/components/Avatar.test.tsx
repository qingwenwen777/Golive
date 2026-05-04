// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { Avatar } from './Avatar';
import { LoadableImage } from './LoadableImage';

describe('Avatar', () => {
  it('reuses a previously loaded avatar without showing the loading shimmer again', () => {
    const src = `/avatar-${Date.now()}-${Math.random().toString(36).slice(2)}.png`;

    const preload = render(<LoadableImage src={src} alt="avatar preload" />);
    fireEvent.load(screen.getByAltText('avatar preload'));
    preload.unmount();

    render(<Avatar name="Luna" src={src} />);
    const avatar = screen.getByLabelText('Luna');
    expect(avatar.className).not.toContain('is-loading');

    const image = avatar.querySelector('img');
    expect(image?.className ?? '').toContain('is-loaded');
  });
});
