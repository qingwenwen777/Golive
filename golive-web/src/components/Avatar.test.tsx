// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { avatarColor, avatarInitial } from '@/lib/avatar';
import { Avatar } from './Avatar';
import { LoadableImage } from './LoadableImage';

describe('Avatar', () => {
  afterEach(() => cleanup());

  it('draws the first letter when there is no avatar image', () => {
    render(<Avatar name="Luna Nova" src="" />);

    const avatar = screen.getByRole('img', { name: 'Luna Nova' });
    expect(avatar.querySelector('img')).toBeNull();
    expect(avatar.textContent).toBe('L');
    expect(avatar.style.background).not.toBe('');
  });

  it('falls back to the letter when the avatar image fails to load', () => {
    render(<Avatar name="Luna" src="/missing-avatar.png" />);

    const image = screen.getByRole('img', { name: 'Luna' }).querySelector('img');
    expect(image).toBeTruthy();
    fireEvent.error(image as HTMLImageElement);

    const avatar = screen.getByRole('img', { name: 'Luna' });
    expect(avatar.querySelector('img')).toBeNull();
    expect(avatar.textContent).toBe('L');
  });

  it('never loads generated DiceBear avatars stored by older accounts', () => {
    render(<Avatar name="Luna" src="https://api.dicebear.com/7.x/avataaars/svg?seed=Luna" />);

    const avatar = screen.getByRole('img', { name: 'Luna' });
    expect(avatar.querySelector('img')).toBeNull();
    expect(avatar.textContent).toBe('L');
  });

  it('reuses a previously loaded avatar without showing the loading shimmer again', () => {
    const src = `/avatar-${Date.now()}-${Math.random().toString(36).slice(2)}.png`;

    const preload = render(<LoadableImage src={src} alt="avatar preload" />);
    fireEvent.load(screen.getByAltText('avatar preload'));
    preload.unmount();

    render(<Avatar name="Luna" src={src} />);
    const avatar = screen.getByRole('img', { name: 'Luna' });
    expect(avatar.className).not.toContain('is-loading');

    const image = avatar.querySelector('img');
    expect(image?.className ?? '').toContain('is-loaded');
  });
});

describe('avatarInitial', () => {
  it('takes the first letter or digit of any script', () => {
    expect(avatarInitial('luna')).toBe('L');
    expect(avatarInitial('星野るな')).toBe('星');
    expect(avatarInitial('🌸 Sakura')).toBe('S');
    expect(avatarInitial('  #1 fan')).toBe('1');
    expect(avatarInitial('')).toBe('');
  });
});

describe('avatarColor', () => {
  it('gives a name the same colour every time', () => {
    expect(avatarColor('Luna')).toBe(avatarColor(' luna '));
    const colours = new Set(
      ['Luna', 'Kuroneko', 'Mika', 'devlogdan', 'Zeph', 'Aki'].map(avatarColor),
    );
    expect(colours.size).toBeGreaterThan(1);
  });
});
