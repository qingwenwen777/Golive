// @vitest-environment jsdom
import { type ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  useAddModerator,
  useMuteRoomUser,
  useRoomMuteState,
  useRoomModerationState,
  useUnmuteRoomUser,
  type ModerationUser,
  type RoomModerationState,
  type RoomMuteState,
} from './moderation';

const httpMock = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
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

describe('moderation api hooks', () => {
  beforeEach(() => {
    httpMock.get.mockReset();
    httpMock.post.mockReset();
    httpMock.delete.mockReset();
  });

  it('fetches room moderation state with encoded room ids', async () => {
    const state: RoomModerationState = {
      roomId: 'room/1',
      ownerId: 'owner-1',
      role: 'moderator',
      canModerate: true,
      muted: false,
    };
    httpMock.get.mockResolvedValueOnce({ data: state });
    const queryClient = makeQueryClient();

    const { result } = renderHook(() => useRoomModerationState('room/1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.data).toEqual(state));

    expect(httpMock.get).toHaveBeenCalledWith(
      '/rooms/room%2F1/moderation/state',
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
  });

  it('adds a moderator and refreshes moderation-related query groups', async () => {
    const moderator: ModerationUser = {
      id: 'user/2',
      name: 'Mod',
      verified: false,
      moderator: true,
    };
    httpMock.post.mockResolvedValueOnce({ data: moderator });
    const queryClient = makeQueryClient();
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');

    const { result } = renderHook(() => useAddModerator(), {
      wrapper: wrapperFor(queryClient),
    });

    await expect(result.current.mutateAsync('user/2')).resolves.toEqual(moderator);

    expect(httpMock.post).toHaveBeenCalledWith('/rooms/moderation/moderators/user%2F2');
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['moderation-followers'] });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['room-moderators'] });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['moderation-logs'] });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['room-moderation-state'] });
  });

  it('mutes a room user and invalidates the target mute and room state caches', async () => {
    const payload = {
      targetUserId: 'target/1',
      targetName: 'Target',
      targetAvatar: '/avatar.jpg',
      durationMinutes: 10 as const,
    };
    const response = {
      roomId: 'room/1',
      targetUserId: 'target/1',
      targetName: 'Target',
      durationMinutes: 10,
      expiresAt: '2026-05-04T00:00:00.000Z',
    };
    httpMock.post.mockResolvedValueOnce({ data: response });
    const queryClient = makeQueryClient();
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');

    const { result } = renderHook(() => useMuteRoomUser('room/1'), {
      wrapper: wrapperFor(queryClient),
    });

    await expect(result.current.mutateAsync(payload)).resolves.toEqual(response);

    expect(httpMock.post).toHaveBeenCalledWith('/rooms/room%2F1/moderation/mutes', payload);
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['moderation-logs'] });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['room-mute-state', 'room/1', 'target/1'],
    });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['room-moderation-state', 'room/1'] });
  });

  it('fetches and clears a specific room mute state with encoded identifiers', async () => {
    const muteState: RoomMuteState = {
      roomId: 'room/1',
      targetUserId: 'target/1',
      targetName: 'Target',
      muted: true,
      muteRemainingSeconds: 300,
    };
    httpMock.get.mockResolvedValueOnce({ data: muteState });
    httpMock.delete.mockResolvedValueOnce({ data: { ...muteState, muted: false } });
    const queryClient = makeQueryClient();
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');

    const muteStateHook = renderHook(() => useRoomMuteState('room/1', 'target/1'), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(muteStateHook.result.current.data).toEqual(muteState));

    expect(httpMock.get).toHaveBeenCalledWith(
      '/rooms/room%2F1/moderation/mutes/target%2F1',
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );

    const unmuteHook = renderHook(() => useUnmuteRoomUser('room/1'), {
      wrapper: wrapperFor(queryClient),
    });

    await expect(unmuteHook.result.current.mutateAsync('target/1')).resolves.toEqual({
      ...muteState,
      muted: false,
    });

    expect(httpMock.delete).toHaveBeenCalledWith('/rooms/room%2F1/moderation/mutes/target%2F1');
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['moderation-logs'] });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['room-mute-state', 'room/1', 'target/1'],
    });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['room-moderation-state', 'room/1'] });
  });
});
