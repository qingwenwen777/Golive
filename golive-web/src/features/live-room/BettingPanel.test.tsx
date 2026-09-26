// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { BetRoundView } from '@/types/bet';
import { BettingPanel } from './BettingPanel';

const toastMock = vi.hoisted(() => ({
  error: vi.fn(),
  success: vi.fn(),
}));

const authMock = vi.hoisted(() => ({
  isAuthed: true,
  openLogin: vi.fn(),
}));

const betApiMock = vi.hoisted(() => ({
  latest: { round: null, summary: [] } as BetRoundView,
  openMutate: vi.fn(),
  placeMutate: vi.fn(),
  settleMutate: vi.fn(),
  cancelMutate: vi.fn(),
}));

const meMock = vi.hoisted(() => ({
  balance: 0,
}));

vi.mock('sonner', () => ({
  toast: toastMock,
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
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
}));

vi.mock('@/stores/useAuthModalStore', () => ({
  useAuthModalStore: <T,>(selector: (state: { openLogin: () => void }) => T) =>
    selector({ openLogin: authMock.openLogin }),
}));

vi.mock('@/api/auth', () => ({
  useMe: () => ({ data: { coinBalance: meMock.balance } }),
}));

vi.mock('@/api/bet', () => ({
  useLatestBet: () => ({ data: betApiMock.latest }),
  useOpenBet: () => ({ mutate: betApiMock.openMutate, isPending: false }),
  usePlaceBet: () => ({ mutate: betApiMock.placeMutate, isPending: false }),
  useSettleBet: () => ({ mutate: betApiMock.settleMutate, isPending: false }),
  useCancelBet: () => ({ mutate: betApiMock.cancelMutate, isPending: false }),
}));

function openRound(): BetRoundView {
  return {
    round: {
      id: 'bet-1',
      roomId: 'room-1',
      ownerId: 'owner-1',
      question: 'Can blue win?',
      amount: 500,
      status: 'open',
      closeAt: new Date(Date.now() + 60_000).toISOString(),
      createdAt: '2026-05-02T00:00:00.000Z',
      updatedAt: '2026-05-02T00:00:00.000Z',
    },
    summary: [
      { option: 'win', count: 0, total: 0 },
      { option: 'lose', count: 0, total: 0 },
    ],
  };
}

describe('BettingPanel', () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    toastMock.error.mockReset();
    toastMock.success.mockReset();
    authMock.isAuthed = true;
    authMock.openLogin.mockReset();
    betApiMock.latest = { round: null, summary: [] };
    betApiMock.openMutate.mockReset();
    betApiMock.placeMutate.mockReset();
    betApiMock.settleMutate.mockReset();
    betApiMock.cancelMutate.mockReset();
    meMock.balance = 10_000;
  });

  it('asks anonymous viewers to log in before placing a wager', () => {
    authMock.isAuthed = false;
    betApiMock.latest = openRound();

    render(<BettingPanel roomId="room-1" ownsStream={false} />);

    fireEvent.click(screen.getByRole('button', { name: /Can win/ }));

    expect(authMock.openLogin).toHaveBeenCalledTimes(1);
    expect(betApiMock.placeMutate).not.toHaveBeenCalled();
  });

  it('blocks a wager locally when the viewer balance is too low', () => {
    authMock.isAuthed = true;
    meMock.balance = 100;
    betApiMock.latest = openRound();

    render(<BettingPanel roomId="room-1" ownsStream={false} />);

    fireEvent.click(screen.getByRole('button', { name: /Can win/ }));

    expect(toastMock.error).toHaveBeenCalledWith('Insufficient coin balance.');
    expect(betApiMock.placeMutate).not.toHaveBeenCalled();
  });

  it('keeps host settle buttons disabled and blocks host wagers while betting is open', () => {
    betApiMock.latest = openRound();

    render(<BettingPanel roomId="room-1" ownsStream />);

    const settle = screen.getByRole('button', { name: 'Can win' }) as HTMLButtonElement;
    expect(settle.disabled).toBe(true);
    expect(settle.title).toBe('You can settle once betting closes.');
    for (const button of screen.getAllByRole('button', { name: /Can win/ })) {
      expect((button as HTMLButtonElement).disabled).toBe(true);
    }
    fireEvent.click(settle);
    expect(betApiMock.settleMutate).not.toHaveBeenCalled();
    expect(betApiMock.placeMutate).not.toHaveBeenCalled();
  });

  it('lets the host settle once the close time has passed', () => {
    const view = openRound();
    view.round!.closeAt = new Date(Date.now() - 1_000).toISOString();
    betApiMock.latest = view;

    render(<BettingPanel roomId="room-1" ownsStream />);

    const settle = screen.getByRole('button', { name: 'Can win' }) as HTMLButtonElement;
    expect(settle.disabled).toBe(false);
    fireEvent.click(settle);
    expect(betApiMock.settleMutate).toHaveBeenCalledWith(
      { roundId: 'bet-1', option: 'win' },
      expect.objectContaining({ onSuccess: expect.any(Function), onError: expect.any(Function) }),
    );
  });

  it('opens a host round with trimmed question text and a floored positive amount', () => {
    render(<BettingPanel roomId="room-1" ownsStream />);

    fireEvent.change(screen.getByLabelText('Title'), {
      target: { value: '  Who takes game one?  ' },
    });
    fireEvent.change(screen.getByLabelText('Wager amount'), {
      target: { value: '250.9' },
    });
    fireEvent.click(screen.getByRole('button', { name: /Open bet/ }));

    expect(betApiMock.openMutate).toHaveBeenCalledWith(
      { amount: 250, question: 'Who takes game one?' },
      expect.objectContaining({ onSuccess: expect.any(Function), onError: expect.any(Function) }),
    );
  });
});
