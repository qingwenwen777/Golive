// @vitest-environment jsdom
import { type ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  useDirectMessages,
  useSendThreadMessage,
  useFanGroupMessages,
  useSendFanGroupMessage,
  useUpdateMessagePreference,
  type DirectThread,
  type DirectThreadListResp,
  type FanGroup,
  type FanGroupListResp,
  type MessagePreference,
} from './messages';

const httpMock = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  patch: vi.fn(),
  delete: vi.fn(),
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

function thread(patch: Partial<DirectThread> = {}): DirectThread {
  return {
    id: 'thread-1',
    viewerId: 'viewer-1',
    creatorId: 'creator-1',
    channelId: 'ch-creator-1',
    peer: {
      id: 'creator-1',
      username: 'creator',
      name: 'Creator',
      avatar: '',
      verified: false,
    },
    lastMessagePreview: 'hello',
    lastSenderId: 'creator-1',
    lastMessageAt: '2026-05-07T00:00:00.000Z',
    unread: 3,
    pinned: false,
    muted: false,
    pushDisabled: false,
    canSend: true,
    awaitingReply: false,
    blocked: false,
    ...patch,
  };
}

function directList(items: DirectThread[]): DirectThreadListResp {
  return { items, total: items.length, page: 1, size: 50 };
}

function fanGroup(patch: Partial<FanGroup> = {}): FanGroup {
  return {
    id: 'group-1',
    creatorId: 'creator-1',
    groupNo: 1,
    name: 'Creator Fans',
    memberCount: 20,
    unread: 4,
    members: [],
    createdAt: '2026-05-07T00:00:00.000Z',
    updatedAt: '2026-05-07T00:05:00.000Z',
    ...patch,
  };
}

function fanGroupList(items: FanGroup[]): FanGroupListResp {
  return { items, total: items.length };
}

describe('messages api hooks', () => {
  beforeEach(() => {
    httpMock.get.mockReset();
    httpMock.post.mockReset();
    httpMock.patch.mockReset();
    httpMock.delete.mockReset();
  });

  it('loads direct message pages and marks matching thread caches as read', async () => {
    const queryClient = makeQueryClient();
    const selected = thread({ id: 'thread 1', unread: 5 });
    const other = thread({ id: 'thread-2', creatorId: 'creator-2', unread: 7 });
    queryClient.setQueryData(['direct-threads', 1, 50], directList([selected, other]));
    queryClient.setQueryData(['direct-draft', 'creator-1'], selected);
    httpMock.get.mockImplementation(
      async (_url: string, config: { params?: { page?: number } }) => {
        const page = config.params?.page ?? 1;
        return {
          data: {
            items: [
              {
                id: `m-${page}`,
                threadId: 'thread 1',
                senderId: 'creator-1',
                receiverId: 'viewer-1',
                body: `page ${page}`,
                createdAt: '2026-05-07T00:00:00.000Z',
              },
            ],
            total: 3,
            page,
            size: 2,
          },
        };
      },
    );

    const { result } = renderHook(() => useDirectMessages('thread 1', true, 2), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(httpMock.get).toHaveBeenCalledWith(
      '/messages/direct/thread%201',
      expect.objectContaining({ params: { page: 1, size: 2 }, signal: expect.anything() }),
    );
    expect(
      queryClient.getQueryData<DirectThreadListResp>(['direct-threads', 1, 50])?.items,
    ).toEqual([{ ...selected, unread: 0 }, other]);
    expect(queryClient.getQueryData<DirectThread>(['direct-draft', 'creator-1'])?.unread).toBe(0);

    await act(async () => {
      await result.current.fetchNextPage();
    });

    expect(httpMock.get).toHaveBeenLastCalledWith(
      '/messages/direct/thread%201',
      expect.objectContaining({ params: { page: 2, size: 2 }, signal: expect.anything() }),
    );
  });

  it('updates thread caches and clears unread after sending into an existing thread', async () => {
    const queryClient = makeQueryClient();
    const before = thread({ id: 'thread 1', unread: 3, lastMessagePreview: 'old' });
    const after = thread({
      id: 'thread 1',
      unread: 8,
      lastMessagePreview: 'new message',
      lastSenderId: 'viewer-1',
    });
    queryClient.setQueryData(['direct-threads', 1, 50], directList([before]));
    queryClient.setQueryData(['direct-draft', 'creator-1'], before);
    httpMock.post.mockResolvedValueOnce({ data: after });

    const { result } = renderHook(() => useSendThreadMessage('thread 1'), {
      wrapper: wrapperFor(queryClient),
    });

    await act(async () => {
      await result.current.mutateAsync('new message');
    });

    expect(httpMock.post).toHaveBeenCalledWith('/messages/direct/thread%201', {
      content: 'new message',
    });
    const expected = { ...after, unread: 0 };
    expect(
      queryClient.getQueryData<DirectThreadListResp>(['direct-threads', 1, 50])?.items,
    ).toEqual([expected]);
    expect(queryClient.getQueryData<DirectThread>(['direct-draft', 'creator-1'])).toEqual(expected);
  });

  it('loads fan group pages and marks the joined group cache as read', async () => {
    const queryClient = makeQueryClient();
    const selected = fanGroup({ id: 'group 1', unread: 6 });
    const other = fanGroup({ id: 'group-2', unread: 2 });
    queryClient.setQueryData(['joined-fan-groups'], fanGroupList([selected, other]));
    httpMock.get.mockImplementation(
      async (_url: string, config: { params?: { page?: number } }) => {
        const page = config.params?.page ?? 1;
        return {
          data: {
            items: [
              {
                id: `gm-${page}`,
                groupId: 'group 1',
                sender: { id: 'fan-1', name: 'Fan', verified: false },
                body: `group page ${page}`,
                createdAt: '2026-05-07T00:00:00.000Z',
              },
            ],
            total: 3,
            page,
            size: 2,
          },
        };
      },
    );

    const { result } = renderHook(() => useFanGroupMessages('group 1', true, 2), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(httpMock.get).toHaveBeenCalledWith(
      '/messages/fan-groups/group%201/messages',
      expect.objectContaining({ params: { page: 1, size: 2 }, signal: expect.anything() }),
    );
    expect(queryClient.getQueryData<FanGroupListResp>(['joined-fan-groups'])?.items).toEqual([
      { ...selected, unread: 0 },
      other,
    ]);

    await act(async () => {
      await result.current.fetchNextPage();
    });

    expect(httpMock.get).toHaveBeenLastCalledWith(
      '/messages/fan-groups/group%201/messages',
      expect.objectContaining({ params: { page: 2, size: 2 }, signal: expect.anything() }),
    );
  });

  it('marks fan group unread as read after sending a group message', async () => {
    const queryClient = makeQueryClient();
    queryClient.setQueryData(
      ['joined-fan-groups'],
      fanGroupList([fanGroup({ id: 'group 1', unread: 9 })]),
    );
    httpMock.post.mockResolvedValueOnce({
      data: {
        id: 'gm-new',
        groupId: 'group 1',
        sender: { id: 'viewer-1', name: 'Viewer', verified: false },
        body: 'hello group',
        createdAt: '2026-05-07T00:00:00.000Z',
      },
    });

    const { result } = renderHook(() => useSendFanGroupMessage('group 1'), {
      wrapper: wrapperFor(queryClient),
    });

    await act(async () => {
      await result.current.mutateAsync('hello group');
    });

    expect(httpMock.post).toHaveBeenCalledWith('/messages/fan-groups/group%201/messages', {
      content: 'hello group',
    });
    expect(queryClient.getQueryData<FanGroupListResp>(['joined-fan-groups'])?.items[0].unread).toBe(
      0,
    );
  });

  it('writes updated message preferences into the cache', async () => {
    const queryClient = makeQueryClient();
    const next: MessagePreference = {
      messageReminderEnabled: false,
      replyReminderScope: 'following',
      mentionReminderScope: 'none',
      likeReminderEnabled: true,
      foldUnfollowedMessages: true,
    };
    httpMock.patch.mockResolvedValueOnce({ data: next });

    const { result } = renderHook(() => useUpdateMessagePreference(), {
      wrapper: wrapperFor(queryClient),
    });

    await act(async () => {
      await result.current.mutateAsync(next);
    });

    expect(httpMock.patch).toHaveBeenCalledWith('/messages/settings', next);
    expect(queryClient.getQueryData(['message-preference'])).toEqual(next);
  });
});
