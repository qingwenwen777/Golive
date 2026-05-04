// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { ChatMessage, SuperChatMessage } from '@/types/message';
import { Chat } from './Chat';

const authMock = vi.hoisted(() => ({
  isAuthed: true,
  user: null as null | {
    id: string;
    username: string;
    displayName: string;
    avatar: string;
    coinBalance: number;
    role: 'user';
    livePermissionStatus: 'approved';
  },
  openLogin: vi.fn(),
}));

const toastMock = vi.hoisted(() => ({
  error: vi.fn(),
}));

vi.mock('sonner', () => ({
  toast: toastMock,
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    i18n: { resolvedLanguage: 'en-US', language: 'en-US' },
    t: (_key: string, options?: { defaultValue?: string; [key: string]: unknown }) => {
      if (!options?.defaultValue) return _key;
      return Object.entries(options).reduce((text, [name, value]) => {
        if (name === 'defaultValue') return text;
        return text.replace(`{{${name}}}`, String(value));
      }, options.defaultValue);
    },
  }),
}));

vi.mock('@/stores/useAuthStore', () => ({
  useIsAuthed: () => authMock.isAuthed,
  useAuthStore: <T,>(selector: (state: { user: typeof authMock.user }) => T) =>
    selector({ user: authMock.user }),
}));

vi.mock('@/stores/useAuthModalStore', () => ({
  useAuthModalStore: <T,>(
    selector: (state: { openLogin: (afterLogin?: () => void) => void }) => T,
  ) => selector({ openLogin: authMock.openLogin }),
}));

function makeUser() {
  return {
    id: 'me',
    username: 'me',
    displayName: 'Me',
    avatar: '',
    coinBalance: 0,
    role: 'user' as const,
    livePermissionStatus: 'approved' as const,
  };
}

function chatMessage(overrides: Partial<ChatMessage> = {}): ChatMessage {
  return {
    id: 'msg-1',
    kind: 'chat',
    userId: 'viewer-1',
    user: 'Viewer',
    text: 'hello',
    ts: 1,
    ...overrides,
  };
}

function superChatMessage(overrides: Partial<SuperChatMessage> = {}): SuperChatMessage {
  return {
    id: 'sc-1',
    kind: 'super_chat',
    userId: 'viewer-sc',
    user: 'Alice',
    text: 'Pinned hello',
    amount: '1000',
    tier: 2,
    ts: Date.now(),
    ...overrides,
  };
}

describe('Chat', () => {
  beforeEach(() => {
    authMock.isAuthed = true;
    authMock.user = makeUser();
    authMock.openLogin.mockReset();
    toastMock.error.mockReset();
    Object.defineProperty(HTMLDivElement.prototype, 'scrollTo', {
      configurable: true,
      value: vi.fn(),
    });
    Object.defineProperty(window, 'requestAnimationFrame', {
      configurable: true,
      value: (callback: FrameRequestCallback) => {
        callback(0);
        return 1;
      },
    });
  });

  afterEach(() => {
    cleanup();
  });

  it('opens the login modal instead of sending for anonymous viewers', () => {
    authMock.isAuthed = false;
    authMock.user = null;
    const onSendChat = vi.fn();

    render(<Chat messages={[]} onSendChat={onSendChat} />);

    fireEvent.focus(screen.getByLabelText('liveRoom.chatInput'));
    fireEvent.click(screen.getByRole('button', { name: 'liveRoom.send' }));

    expect(authMock.openLogin).toHaveBeenCalledTimes(2);
    expect(onSendChat).not.toHaveBeenCalled();
  });

  it('trims and sends chat text with Enter, then clears the composer', async () => {
    const onSendChat = vi.fn(() => true);
    render(<Chat messages={[]} onSendChat={onSendChat} />);
    const input = screen.getByLabelText('liveRoom.chatInput') as HTMLInputElement;

    fireEvent.change(input, { target: { value: '  hello room  ' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(onSendChat).toHaveBeenCalledWith('hello room');
    await waitFor(() => expect(input.value).toBe(''));
  });

  it('does not submit while an IME composition is active', () => {
    const onSendChat = vi.fn(() => true);
    render(<Chat messages={[]} onSendChat={onSendChat} />);
    const input = screen.getByLabelText('liveRoom.chatInput');

    fireEvent.change(input, { target: { value: 'ni hao' } });
    fireEvent.compositionStart(input);
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(onSendChat).not.toHaveBeenCalled();

    fireEvent.compositionEnd(input);
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(onSendChat).toHaveBeenCalledWith('ni hao');
  });

  it('keeps the composer read-only and send button disabled while the viewer is muted', () => {
    const onSendChat = vi.fn();
    render(<Chat messages={[]} chatMuted onSendChat={onSendChat} />);
    const input = screen.getByLabelText('liveRoom.chatInput') as HTMLInputElement;
    const sendButton = screen.getByRole('button', { name: 'liveRoom.send' }) as HTMLButtonElement;

    expect(input.readOnly).toBe(true);
    expect(input.placeholder).toBe('你当前已被禁言');
    expect(sendButton.disabled).toBe(true);

    fireEvent.change(input, { target: { value: 'blocked' } });
    fireEvent.click(sendButton);

    expect(input.value).toBe('');
    expect(onSendChat).not.toHaveBeenCalled();
  });

  it('opens moderation targets from chat rows with owner and moderator roles preserved', () => {
    const onOpenModeration = vi.fn();
    render(
      <Chat
        messages={[
          chatMessage({ id: 'owner-msg', userId: 'owner-1', user: 'Host' }),
          chatMessage({
            id: 'mod-msg',
            userId: 'mod-1',
            user: 'Mod',
            role: 'moderator',
            avatar: '/mod.jpg',
          }),
        ]}
        ownerId="owner-1"
        canModerate
        onOpenModeration={onOpenModeration}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Moderate Host' }));
    fireEvent.click(screen.getByRole('button', { name: 'Moderate Mod' }));

    expect(onOpenModeration).toHaveBeenNthCalledWith(1, {
      userId: 'owner-1',
      user: 'Host',
      avatar: undefined,
      role: 'owner',
    });
    expect(onOpenModeration).toHaveBeenNthCalledWith(2, {
      userId: 'mod-1',
      user: 'Mod',
      avatar: '/mod.jpg',
      role: 'moderator',
    });
  });

  it('keeps moderation avatar buttons disabled when the viewer cannot moderate', () => {
    const onOpenModeration = vi.fn();
    render(
      <Chat
        messages={[chatMessage({ userId: 'viewer-1', user: 'Viewer' })]}
        canModerate={false}
        onOpenModeration={onOpenModeration}
      />,
    );

    const button = screen.getByRole('button', { name: 'Moderate Viewer' }) as HTMLButtonElement;

    expect(button.disabled).toBe(true);
    fireEvent.click(button);
    expect(onOpenModeration).not.toHaveBeenCalled();
  });

  it('keeps pinned SuperChats as compact capsules and reveals details in a popover', () => {
    render(
      <Chat
        messages={[
          superChatMessage(),
          superChatMessage({
            id: 'sc-2',
            user: 'Bob',
            text: 'Second pinned message',
            amount: '2000',
            tier: 3,
          }),
        ]}
      />,
    );

    const pinned = screen.getByLabelText('liveRoom.chatPanel.pinnedSuperChats');
    expect(pinned.querySelector('.gl-sc-pin-row')).toBeTruthy();
    expect(within(pinned).getByRole('button', { name: 'Open SuperChat from Alice' })).toBeTruthy();
    expect(within(pinned).getByRole('button', { name: 'Open SuperChat from Bob' })).toBeTruthy();
    expect(within(pinned).queryByText('\u00a51,000')).toBeNull();
    expect(within(pinned).queryByText('Pinned hello')).toBeNull();

    fireEvent.click(within(pinned).getByRole('button', { name: 'Open SuperChat from Alice' }));

    expect(within(pinned).getByText('1,000 coins')).toBeTruthy();
    expect(within(pinned).getByText('Pinned hello')).toBeTruthy();
  });

  it('orders pinned SuperChats by tier, amount, then latest send time', () => {
    const now = Date.now();
    render(
      <Chat
        messages={[
          superChatMessage({
            id: 'sc-low-tier',
            user: 'Lower tier',
            amount: '5000',
            tier: 4,
            ts: now + 4,
          }),
          superChatMessage({
            id: 'sc-same-early',
            user: 'Earlier same amount',
            amount: '10000',
            tier: 5,
            ts: now + 2,
          }),
          superChatMessage({
            id: 'sc-high',
            user: 'Highest amount',
            amount: '20000',
            tier: 5,
            ts: now + 1,
          }),
          superChatMessage({
            id: 'sc-same-late',
            user: 'Later same amount',
            amount: '10000',
            tier: 5,
            ts: now + 3,
          }),
        ]}
      />,
    );

    const pinned = screen.getByLabelText('liveRoom.chatPanel.pinnedSuperChats');
    const names = Array.from(pinned.querySelectorAll('.gl-sc-pin-user')).map(
      (node) => node.textContent,
    );

    expect(names).toEqual([
      'Highest amount',
      'Later same amount',
      'Earlier same amount',
      'Lower tier',
    ]);
  });
});
