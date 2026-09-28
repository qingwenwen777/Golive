// @vitest-environment jsdom
import { type ReactNode } from 'react';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { toast } from 'sonner';

import { SuperChatDialog } from './SuperChatDialog';

const scMock = vi.hoisted(() => ({
  mutate: vi.fn(),
}));

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
}));

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: Record<string, unknown>) => {
      let value = String(options?.defaultValue ?? key);
      for (const [name, replacement] of Object.entries(options ?? {})) {
        if (name === 'defaultValue') continue;
        value = value.replaceAll(`{{${name}}}`, String(replacement));
      }
      return value;
    },
    i18n: { language: 'en', resolvedLanguage: 'en' },
  }),
}));

vi.mock('@/hooks/useMediaQuery', () => ({
  useMediaQuery: () => false,
}));

vi.mock('@/api/auth', () => ({
  useMe: () => ({ data: { coinBalance: 100000 } }),
}));

vi.mock('@/api/gift', () => ({
  newRequestId: () => 'req-fixed',
  useSendSuperChat: () => ({ mutate: scMock.mutate, isPending: false }),
}));

vi.mock('@/stores/useAuthStore', () => ({
  useAuthStore: (select: (state: unknown) => unknown) =>
    select({ user: { id: 'u-1', username: 'viewer', displayName: 'Viewer', avatar: '' } }),
}));

vi.mock('@/stores/useRealtimeStore', () => ({
  useRealtimeStore: (select: (state: unknown) => unknown) =>
    select({ appendMessage: vi.fn(), replaceMessage: vi.fn(), removeMessage: vi.fn() }),
}));

vi.mock('@/components/ui/dialog', () => ({
  Dialog: ({ open, children }: { open: boolean; children: ReactNode }) =>
    open ? <div>{children}</div> : null,
  DialogContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
  DialogDescription: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

function renderDialog() {
  render(<SuperChatDialog open onOpenChange={vi.fn()} roomId="room-1" />);
  return {
    textarea: screen.getByPlaceholderText('Say something...') as HTMLTextAreaElement,
    send: screen.getByRole('button', { name: 'Send' }) as HTMLButtonElement,
  };
}

describe('SuperChatDialog', () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    scMock.mutate.mockReset();
    vi.mocked(toast.error).mockReset();
  });

  it('holds back text over the limit of a lowered tier until it is shortened', () => {
    const { textarea, send } = renderDialog();

    fireEvent.click(screen.getByRole('button', { name: '1,000 coins' }));
    fireEvent.change(textarea, { target: { value: 'a'.repeat(80) } });
    expect(textarea.value).toHaveLength(80);

    // 500 coins is tier 1, which allows 50 characters.
    fireEvent.click(screen.getByRole('button', { name: '500 coins' }));
    expect(send.disabled).toBe(true);
    expect(screen.getByText('Message is too long for this tier (max 50 characters).')).toBeTruthy();

    fireEvent.change(textarea, { target: { value: 'a'.repeat(81) } });
    expect(textarea.value).toHaveLength(80);
    fireEvent.change(textarea, { target: { value: 'a'.repeat(50) } });
    expect(send.disabled).toBe(false);

    fireEvent.click(send);
    expect(scMock.mutate).toHaveBeenCalledWith(
      expect.objectContaining({ amount: 500, text: 'a'.repeat(50), requestId: 'req-fixed' }),
      expect.anything(),
    );
  });

  it('counts an emoji as one character, as the server does', () => {
    const { textarea, send } = renderDialog();

    fireEvent.change(textarea, { target: { value: '😀'.repeat(50) } });

    expect(textarea.value).toBe('😀'.repeat(50));
    expect(screen.getByText('50 / 50')).toBeTruthy();
    expect(send.disabled).toBe(false);
  });

  it('explains the tier limit when the server rejects the text', () => {
    scMock.mutate.mockImplementation((_payload, options) => {
      options.onError({
        reason: 'super_chat_text_too_long',
        message: 'Text too long for this SuperChat tier',
      });
    });
    const { textarea, send } = renderDialog();

    fireEvent.change(textarea, { target: { value: 'hello' } });
    fireEvent.click(send);

    expect(toast.error).toHaveBeenCalledWith(
      'Message is too long for this tier (max 50 characters).',
    );
  });
});
