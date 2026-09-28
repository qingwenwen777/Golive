// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest';

import { playerShortcutFor } from './playerShortcuts';

function key(target: EventTarget | null, key: string, init: Partial<KeyboardEvent> = {}) {
  return {
    key,
    altKey: false,
    ctrlKey: false,
    metaKey: false,
    defaultPrevented: false,
    repeat: false,
    target,
    ...init,
  } as KeyboardEvent;
}

function setup() {
  document.body.innerHTML = `
    <div id="player" tabindex="0">
      <button id="mute">Mute</button>
      <input id="volume" type="range" />
    </div>
    <button id="share">Share</button>
    <input id="chat" />
    <div role="menu"><div role="menuitem" id="item" tabindex="-1">Watch later</div></div>
  `;
  const $ = (id: string) => document.getElementById(id)!;
  return { player: $('player'), $ };
}

afterEach(() => {
  document.body.innerHTML = '';
});

describe('playerShortcutFor', () => {
  it('handles shortcuts when nothing in particular has focus', () => {
    const { player } = setup();
    expect(playerShortcutFor(key(document.body, ' '), player)).toBe('togglePlay');
    expect(playerShortcutFor(key(document.body, 'm'), player)).toBe('mute');
    expect(playerShortcutFor(key(document.body, 'F'), player)).toBe('fullscreen');
    expect(playerShortcutFor(key(document.body, 'd'), player)).toBe('danmu');
  });

  it('leaves arrow keys to page scrolling unless the player has focus', () => {
    const { player, $ } = setup();
    expect(playerShortcutFor(key(document.body, 'ArrowDown'), player)).toBeNull();
    expect(playerShortcutFor(key(player, 'ArrowDown'), player)).toBe('volumeDown');
    expect(playerShortcutFor(key($('mute'), 'ArrowUp'), player)).toBe('volumeUp');
  });

  it('never takes Space from a focused button', () => {
    const { player, $ } = setup();
    expect(playerShortcutFor(key($('share'), ' '), player)).toBeNull();
    expect(playerShortcutFor(key($('mute'), ' '), player)).toBeNull();
  });

  it('ignores keys meant for other controls, fields and menus', () => {
    const { player, $ } = setup();
    expect(playerShortcutFor(key($('share'), 'm'), player)).toBeNull();
    expect(playerShortcutFor(key($('chat'), 'm'), player)).toBeNull();
    expect(playerShortcutFor(key($('item'), 'd'), player)).toBeNull();
    expect(playerShortcutFor(key($('volume'), 'ArrowUp'), player)).toBeNull();
  });

  it('ignores modified, repeated and already-handled keys', () => {
    const { player } = setup();
    expect(playerShortcutFor(key(document.body, 'f', { ctrlKey: true }), player)).toBeNull();
    expect(playerShortcutFor(key(document.body, 'm', { metaKey: true }), player)).toBeNull();
    expect(playerShortcutFor(key(document.body, ' ', { repeat: true }), player)).toBeNull();
    expect(playerShortcutFor(key(document.body, 'm', { defaultPrevented: true }), player)).toBeNull();
  });
});
