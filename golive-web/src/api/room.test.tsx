// @vitest-environment jsdom
import { type ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  type FollowState,
  type LikeState,
  useFollow,
  useLike,
  useUnfollow,
  useUpdateLiveMetadata,
  useUploadLiveCover,
} from './room';

const httpMock = vi.hoisted(() => ({
  post: vi.fn(),
  patch: vi.fn(),
  delete: vi.fn(),
  request: vi.fn(),
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

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe('room api hooks', () => {
  beforeEach(() => {
    vi.stubEnv('VITE_API_BASE', '/api');
    httpMock.post.mockReset();
    httpMock.patch.mockReset();
    httpMock.delete.mockReset();
    httpMock.request.mockReset();
  });

  it('optimistically follows a channel and rolls back when the mutation fails', async () => {
    const queryClient = makeQueryClient();
    queryClient.setQueryData<FollowState>(['follow', 'channel-1'], {
      channelId: 'channel-1',
      following: false,
      subscriberCount: 7,
    });
    const request = deferred<{ data: FollowState }>();
    httpMock.post.mockReturnValueOnce(request.promise);
    const { result } = renderHook(() => useFollow('channel-1'), {
      wrapper: wrapperFor(queryClient),
    });

    let pending: Promise<unknown>;
    act(() => {
      pending = result.current.mutateAsync();
    });

    await waitFor(() => {
      expect(queryClient.getQueryData<FollowState>(['follow', 'channel-1'])).toEqual({
        channelId: 'channel-1',
        following: true,
        subscriberCount: 8,
      });
    });

    const assertion = expect(pending!).rejects.toThrow('network down');
    request.reject(new Error('network down'));
    await assertion;
    expect(queryClient.getQueryData<FollowState>(['follow', 'channel-1'])).toEqual({
      channelId: 'channel-1',
      following: false,
      subscriberCount: 7,
    });
  });

  it('optimistically unfollows without underflowing subscribers and restores on failure', async () => {
    const queryClient = makeQueryClient();
    queryClient.setQueryData<FollowState>(['follow', 'channel-1'], {
      channelId: 'channel-1',
      following: true,
      subscriberCount: 0,
    });
    const request = deferred<{ data: FollowState }>();
    httpMock.delete.mockReturnValueOnce(request.promise);
    const { result } = renderHook(() => useUnfollow('channel-1'), {
      wrapper: wrapperFor(queryClient),
    });

    let pending: Promise<unknown>;
    act(() => {
      pending = result.current.mutateAsync();
    });

    await waitFor(() => {
      expect(queryClient.getQueryData<FollowState>(['follow', 'channel-1'])).toEqual({
        channelId: 'channel-1',
        following: false,
        subscriberCount: 0,
      });
    });

    const assertion = expect(pending!).rejects.toThrow('server rejected');
    request.reject(new Error('server rejected'));
    await assertion;
    expect(queryClient.getQueryData<FollowState>(['follow', 'channel-1'])).toEqual({
      channelId: 'channel-1',
      following: true,
      subscriberCount: 0,
    });
  });

  it('applies local like state immediately and rolls back the cache on error', async () => {
    const queryClient = makeQueryClient();
    queryClient.setQueryData<LikeState>(['like', 'stream-1'], {
      streamId: 'stream-1',
      liked: true,
      disliked: false,
      likes: 12,
    });
    const request = deferred<{ data: LikeState }>();
    httpMock.request.mockReturnValueOnce(request.promise);
    const { result } = renderHook(() => useLike('stream-1'), {
      wrapper: wrapperFor(queryClient),
    });

    let pending: Promise<unknown>;
    act(() => {
      pending = result.current.mutateAsync('dislike');
    });

    await waitFor(() => {
      expect(queryClient.getQueryData<LikeState>(['like', 'stream-1'])).toEqual({
        streamId: 'stream-1',
        liked: false,
        disliked: true,
        likes: 11,
      });
    });

    const assertion = expect(pending!).rejects.toThrow('like failed');
    request.reject(new Error('like failed'));
    await assertion;
    expect(queryClient.getQueryData<LikeState>(['like', 'stream-1'])).toEqual({
      streamId: 'stream-1',
      liked: true,
      disliked: false,
      likes: 12,
    });
  });

  it('normalizes relative uploaded cover urls against the api base contract', async () => {
    const queryClient = makeQueryClient();
    httpMock.post.mockResolvedValueOnce({ data: { url: 'uploads/covers/live.webp' } });
    const { result } = renderHook(() => useUploadLiveCover(), {
      wrapper: wrapperFor(queryClient),
    });
    const file = new File(['cover'], 'cover.webp', { type: 'image/webp' });

    await expect(result.current.mutateAsync(file)).resolves.toEqual({
      url: '/uploads/covers/live.webp',
    });
    expect(httpMock.post).toHaveBeenCalledWith(
      '/rooms/live/cover',
      expect.any(FormData),
      { headers: { 'Content-Type': 'multipart/form-data' } },
    );
  });

  it('updates live metadata and refreshes the room cache', async () => {
    const queryClient = makeQueryClient();
    const stream = {
      id: 'stream-1',
      title: 'New title',
      description: 'New description',
      channel: 'Creator',
      channelId: 'ch-owner',
      verified: false,
      avatar: '',
      cover: '/uploads/covers/new.webp',
      viewers: 0,
      duration: '0:00:00',
      category: 'Just Chatting',
      startedAt: new Date().toISOString(),
      isLive: true,
    };
    httpMock.patch.mockResolvedValueOnce({ data: stream });
    const { result } = renderHook(() => useUpdateLiveMetadata(), {
      wrapper: wrapperFor(queryClient),
    });

    await expect(
      result.current.mutateAsync({
        title: 'New title',
        description: 'New description',
        cover: '/uploads/covers/new.webp',
      }),
    ).resolves.toEqual(stream);
    expect(httpMock.patch).toHaveBeenCalledWith('/rooms/live', {
      title: 'New title',
      description: 'New description',
      cover: '/uploads/covers/new.webp',
    });
    expect(queryClient.getQueryData(['room', 'stream-1'])).toEqual(stream);
  });
});
