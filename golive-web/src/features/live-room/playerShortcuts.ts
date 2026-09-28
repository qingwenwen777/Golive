export type PlayerShortcut =
  | 'togglePlay'
  | 'fullscreen'
  | 'mute'
  | 'danmu'
  | 'volumeUp'
  | 'volumeDown';

const EDITABLE = 'input, textarea, select, [contenteditable=""], [contenteditable="true"]';
const OVERLAY = '[role="dialog"], [role="alertdialog"], [role="menu"], [role="menubar"], [role="listbox"]';
const CONTROL =
  'button, a[href], summary, [role="button"], [role="link"], [role="menuitem"], [role="tab"], [role="option"], [role="switch"], [role="checkbox"], [role="radio"], [role="slider"]';

type ShortcutEvent = Pick<
  KeyboardEvent,
  'key' | 'altKey' | 'ctrlKey' | 'metaKey' | 'defaultPrevented' | 'repeat' | 'target'
>;

// playerShortcutFor decides whether a keydown belongs to the player. Shortcuts
// work while nothing in particular has focus (the page body) or while focus is
// inside the player. Anything else keeps its own keys: a focused button still
// activates on Space, arrows still scroll the page, and typing in a field,
// menu or dialog never mutes the stream.
export function playerShortcutFor(e: ShortcutEvent, player: Element): PlayerShortcut | null {
  if (e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey) return null;

  const target = e.target instanceof Element ? e.target : null;
  const onPage =
    !target || target === target.ownerDocument.body || target === target.ownerDocument.documentElement;
  const inPlayer = !!target && player.contains(target);
  if (!onPage && !inPlayer) return null;

  if (target && !onPage && (target.closest(EDITABLE) || target.closest(OVERLAY))) return null;
  const onControl = !!target && inPlayer && target !== player && !!target.closest(CONTROL);

  switch (e.key) {
    case ' ':
    case 'Spacebar':
      // A focused control (play, mute, danmu…) handles Space itself.
      return onControl || e.repeat ? null : 'togglePlay';
    case 'f':
    case 'F':
      return e.repeat ? null : 'fullscreen';
    case 'm':
    case 'M':
      return e.repeat ? null : 'mute';
    case 'd':
    case 'D':
      return e.repeat ? null : 'danmu';
    // Arrows adjust volume only inside the player; elsewhere they scroll.
    case 'ArrowUp':
      return inPlayer ? 'volumeUp' : null;
    case 'ArrowDown':
      return inPlayer ? 'volumeDown' : null;
    default:
      return null;
  }
}
