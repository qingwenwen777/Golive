// @vitest-environment jsdom
import { type ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook } from '@testing-library/react';
import { AxiosError, type AxiosResponse } from 'axios';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { GiftOrder } from '@/types/gift';
import { useJoinFanClub } from './gift';

const httpMock = vi.hoisted(() => ({
  post: vi.fn(),
}));

vi.mock('@/lib/axios', () => ({
  http: httpMock,
}));

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
}

function wrapperFor(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

// An axios error with the given response, or none (a timeout or dropped connection).
function httpError(status?: number, data?: unknown) {
  const response =
    status === undefined ? undefined : ({ status, data, headers: {} } as unknown as AxiosResponse);
  return new AxiosError('request failed', 'ERR', undefined, undefined, response);
}

const joinedOrder: GiftOrder = {
  orderId: 'gift-1',
  requestId: 'unused',
  giftId: 'fan_light',
  count: 1,
  totalCoin: 1000,
  status: 'success',
  createdAt: '2026-09-26T00:00:00.000Z',
};

function postedRequestIds(): string[] {
  return httpMock.post.mock.calls.map(([, body, config]) => {
    expect(config.headers['X-Request-Id']).toBe(body.requestId);
    return body.requestId as string;
  });
}

describe('useJoinFanClub', () => {
  beforeEach(() => {
    httpMock.post.mockReset();
  });

  it('retries a join with the same request id until the server answers it', async () => {
    httpMock.post
      .mockRejectedValueOnce(httpError())
      .mockRejectedValueOnce(httpError(502))
      .mockResolvedValueOnce({ data: joinedOrder })
      .mockResolvedValueOnce({ data: joinedOrder });
    const { result } = renderHook(() => useJoinFanClub(), {
      wrapper: wrapperFor(makeQueryClient()),
    });

    await expect(result.current.mutateAsync({ creatorId: 'creator-1' })).rejects.toMatchObject({
      reason: 'network',
    });
    await expect(result.current.mutateAsync({ creatorId: 'creator-1' })).rejects.toMatchObject({
      reason: 'unknown',
    });
    await expect(result.current.mutateAsync({ creatorId: 'creator-1' })).resolves.toEqual(
      joinedOrder,
    );
    await result.current.mutateAsync({ creatorId: 'creator-1' });

    const [timedOut, badGateway, answered, next] = postedRequestIds();
    expect(badGateway).toBe(timedOut);
    expect(answered).toBe(timedOut);
    expect(next).not.toBe(answered);
    expect(httpMock.post).toHaveBeenCalledWith(
      '/gifts/fan-clubs/join',
      { creatorId: 'creator-1', requestId: timedOut },
      { headers: { 'X-Request-Id': timedOut } },
    );
  });

  it('reports an existing membership and refreshes the stale membership queries', async () => {
    httpMock.post
      .mockRejectedValueOnce(
        httpError(409, {
          message: 'Already a fan club member',
          reason: 'already_fan_club_member',
        }),
      )
      .mockResolvedValueOnce({ data: joinedOrder });
    const queryClient = makeQueryClient();
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');
    const { result } = renderHook(() => useJoinFanClub(), {
      wrapper: wrapperFor(queryClient),
    });

    await expect(result.current.mutateAsync({ creatorId: 'creator-1' })).rejects.toEqual({
      reason: 'already_fan_club_member',
      message: 'Already a fan club member',
    });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['fan-badges', 'me'] });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['fan-club-members'] });

    // The refusal was an answer, so a later join starts a new attempt.
    await result.current.mutateAsync({ creatorId: 'creator-1' });
    const [refused, next] = postedRequestIds();
    expect(next).not.toBe(refused);
  });
});
