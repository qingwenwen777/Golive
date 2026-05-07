// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { Avatar } from './Avatar';
import { LoadableImage } from './LoadableImage';

describe('Avatar', () => {
  afterEach(() => cleanup());

  it('uses the original DiceBear default avatar when no avatar image is available', () => {
    render(<Avatar name="Luna Nova" src="" />);

    const avatar = screen.getByLabelText('Luna Nova');
    const image = avatar.querySelector('img');
    expect(image?.src).toBe('https://api.dicebear.com/7.x/avataaars/svg?seed=Luna%20Nova');
  });

  it('falls back to the DiceBear default avatar when the primary avatar image fails to load', () => {
    render(<Avatar name="Luna" src="/missing-avatar.png" />);

    const image = screen.getByLabelText('Luna').querySelector('img');
    expect(image).toBeTruthy();
    fireEvent.error(image as HTMLImageElement);

    expect(screen.getByLabelText('Luna').querySelector('img')?.src).toBe(
      'https://api.dicebear.com/7.x/avataaars/svg?seed=Luna',
    );
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
