// @vitest-environment jsdom
import { type ReactNode } from 'react';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { Gift } from '@/types/gift';
import { GiftPanel } from './GiftPanel';

const navigateMock = vi.hoisted(() => vi.fn());
const giftApiMock = vi.hoisted(() => ({
  gifts: [] as Gift[],
  balance: 0,
  mutate: vi.fn(),
  requestId: 'req-fixed',
}));

vi.mock('react-router-dom', () => ({
  useNavigate: () => navigateMock,
}));

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}));

vi.mock('@/hooks/useMediaQuery', () => ({
  useMediaQuery: () => false,
}));

vi.mock('@/api/auth', () => ({
  useMe: () => ({ data: { coinBalance: giftApiMock.balance } }),
}));

vi.mock('@/api/gift', () => ({
  newRequestId: () => giftApiMock.requestId,
  useGifts: () => ({ data: giftApiMock.gifts, isPending: false }),
  useSendGift: () => ({ mutate: giftApiMock.mutate, isPending: false }),
}));

vi.mock('@/components/ui/sheet', () => ({
  Sheet: ({ open, children }: { open: boolean; children: ReactNode }) =>
    open ? <div>{children}</div> : null,
  SheetContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  SheetHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  SheetTitle: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
  SheetDescription: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

function rocket(priceCoin: number, extra: Partial<Gift> = {}): Gift {
  return {
    id: 'rocket',
    name: 'Rocket',
    icon: 'R',
    priceCoin,
    category: 'premium',
    tier: 2,
    ...extra,
  };
}

describe('GiftPanel', () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    navigateMock.mockReset();
    giftApiMock.mutate.mockReset();
    giftApiMock.requestId = 'req-fixed';
    giftApiMock.gifts = [rocket(100)];
    giftApiMock.balance = 1000;
  });

  it('routes to recharge instead of mutating when balance is insufficient', () => {
    giftApiMock.gifts = [rocket(1200)];
    giftApiMock.balance = 100;
    const onOpenChange = vi.fn();

    render(
      <GiftPanel
        open
        onOpenChange={onOpenChange}
        roomId="room-1"
      />,
    );

    fireEvent.click(screen.getByText('Rocket'));
    fireEvent.click(screen.getByRole('button', { name: 'Top up' }));

    expect(giftApiMock.mutate).not.toHaveBeenCalled();
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(navigateMock).toHaveBeenCalledWith('/coins?focus=recharge');
  });

  it('routes to recharge instead of mutating when the gift level is locked', () => {
    giftApiMock.gifts = [rocket(100, { unlockLevel: 6 })];
    giftApiMock.balance = 1000;
    const onOpenChange = vi.fn();

    render(
      <GiftPanel
        open
        onOpenChange={onOpenChange}
        roomId="room-1"
      />,
    );

    fireEvent.click(screen.getByText('Rocket'));
    fireEvent.click(screen.getByRole('button', { name: 'Top up' }));

    expect(screen.getByText('Lv.6')).toBeTruthy();
    expect(giftApiMock.mutate).not.toHaveBeenCalled();
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(navigateMock).toHaveBeenCalledWith('/coins?focus=recharge');
  });

  it('sends the selected gift with a stable request id and reports the optimistic payload', () => {
    const onSent = vi.fn();
    giftApiMock.mutate.mockImplementation((_payload, options) => {
      options.onSuccess({
        orderId: 'order-1',
        requestId: giftApiMock.requestId,
        giftId: 'rocket',
        count: 1,
        totalCoin: 100,
        status: 'success',
        createdAt: '2026-05-02T00:00:00.000Z',
      });
    });

    render(
      <GiftPanel
        open
        onOpenChange={vi.fn()}
        roomId="room-1"
        onSent={onSent}
      />,
    );

    fireEvent.click(screen.getByText('Rocket'));
    fireEvent.click(screen.getByRole('button', { name: 'Send' }));

    expect(giftApiMock.mutate).toHaveBeenCalledWith(
      { roomId: 'room-1', giftId: 'rocket', count: 1, requestId: 'req-fixed' },
      expect.objectContaining({ onSuccess: expect.any(Function), onError: expect.any(Function) }),
    );
    expect(onSent).toHaveBeenCalledWith({
      gift: rocket(100),
      count: 1,
      requestId: 'req-fixed',
    });
  });
});
