// @vitest-environment jsdom
import { type ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useDanmuHistory, useReplayMessages } from './chat';

const httpMock = vi.hoisted(() => ({
  get: vi.fn(),
}));

vi.mock('@/lib/axios', () => ({
  http: httpMock,
}));

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
}

function wrapperFor(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

describe('chat api hooks', () => {
  beforeEach(() => {
    httpMock.get.mockReset();
  });

  it('loads danmu history newest-first from the server and returns chronological messages', async () => {
    const queryClient = makeQueryClient();
    httpMock.get.mockResolvedValueOnce({
      data: {
        items: [
          {
            type: 'super_chat',
            id: 'sc-1',
            userId: 'u2',
            user: 'Bob',
            amount: '500',
            text: 'Pinned support',
            ts: 3000,
          },
          {
            type: 'chat',
            id: 'chat-1',
            userId: 'u1',
            user: 'Alice',
            text: 'hello',
            color: '#fff',
            role: 'moderator',
            fanBadge: { creatorId: 'creator-1', level: 3 },
            userLevel: 12,
            ts: 1000,
          },
        ],
      },
    });

    const { result } = renderHook(() => useDanmuHistory('room 1', true, 2), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(httpMock.get).toHaveBeenCalledWith(
      '/chat/rooms/room%201/danmus',
      expect.objectContaining({ params: { limit: 2 }, signal: expect.anything() }),
    );
    expect(result.current.data).toEqual([
      {
        id: 'chat-1',
        kind: 'chat',
        userId: 'u1',
        user: 'Alice',
        text: 'hello',
        color: '#fff',
        role: 'moderator',
        fanBadge: { creatorId: 'creator-1', level: 3 },
        userLevel: 12,
        ts: 1000,
      },
      {
        id: 'sc-1',
        kind: 'super_chat',
        userId: 'u2',
        user: 'Bob',
        amount: '500',
        tier: 0,
        text: 'Pinned support',
        ts: 3000,
      },
    ]);
  });

  it('paginates replay messages until the stream start and filters outside the replay window', async () => {
    const queryClient = makeQueryClient();
    httpMock.get.mockImplementation(
      async (_url: string, config: { params?: { before?: number } }) => {
        if (config.params?.before === 3000) {
          return {
            data: {
              items: [
                { type: 'chat', id: 'inside-early', user: 'Alice', text: 'start', ts: 1000 },
                { type: 'chat', id: 'too-old', user: 'Eve', text: 'old', ts: 500 },
              ],
            },
          };
        }
        return {
          data: {
            items: [
              { type: 'chat', id: 'too-late', user: 'Mallory', text: 'late', ts: 5000 },
              {
                type: 'super_chat',
                id: 'inside-late',
                user: 'Bob',
                amount: '200',
                tier: 1,
                text: 'nice',
                ts: 3000,
              },
            ],
          },
        };
      },
    );

    const { result } = renderHook(
      () =>
        useReplayMessages('room 1', '1970-01-01T00:00:01.000Z', '1970-01-01T00:00:04.000Z', true),
      { wrapper: wrapperFor(queryClient) },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(httpMock.get).toHaveBeenNthCalledWith(
      1,
      '/chat/rooms/room%201/danmus',
      expect.objectContaining({ params: { limit: 200 }, signal: expect.anything() }),
    );
    expect(httpMock.get).toHaveBeenNthCalledWith(
      2,
      '/chat/rooms/room%201/danmus',
      expect.objectContaining({ params: { limit: 200, before: 3000 }, signal: expect.anything() }),
    );
    expect(result.current.data?.map((message) => message.id)).toEqual([
      'inside-early',
      'inside-late',
    ]);
  });
});
