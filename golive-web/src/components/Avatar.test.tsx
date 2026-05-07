// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { Avatar } from './Avatar';
import { LoadableImage } from './LoadableImage';

describe('Avatar', () => {
  afterEach(() => cleanup());

  it('shows initials when no avatar image is available', () => {
    render(<Avatar name="Luna Nova" src="" />);

    const avatar = screen.getByLabelText('Luna Nova');
    expect(avatar.querySelector('img')).toBeNull();
    expect(screen.getByText('LN')).toBeTruthy();
  });

  it('uses the first visible character for non-latin names', () => {
    render(<Avatar name="小白" />);

    expect(screen.getByText('小')).toBeTruthy();
  });

  it('falls back to initials when the avatar image fails to load', () => {
    render(<Avatar name="Luna" src="/missing-avatar.png" />);

    const image = screen.getByLabelText('Luna').querySelector('img');
    expect(image).toBeTruthy();
    fireEvent.error(image as HTMLImageElement);

    expect(screen.getByText('L')).toBeTruthy();
  });

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
