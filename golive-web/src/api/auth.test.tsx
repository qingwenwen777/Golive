// @vitest-environment jsdom
import { type ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useUploadAvatar, useUploadChannelCover } from './auth';
import type { User } from '@/types/user';

const httpMock = vi.hoisted(() => ({
  post: vi.fn(),
  patch: vi.fn(),
  get: vi.fn(),
}));

const authStoreMock = vi.hoisted(() => ({
  isAuthed: true,
  user: null as User | null,
  setUser: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
}));

vi.mock('@/lib/axios', () => ({
  http: httpMock,
}));

vi.mock('@/stores/useAuthStore', () => {
  const useAuthStore = Object.assign(
    (selector: (state: typeof authStoreMock) => unknown) => selector(authStoreMock),
    {
      getState: () => authStoreMock,
    },
  );

  return {
    useAuthStore,
    useIsAuthed: () => authStoreMock.isAuthed,
  };
});

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

function makeUser(overrides: Partial<User> = {}): User {
  return {
    id: 'user-1',
    username: 'streamer',
    displayName: 'Streamer',
    avatar: '',
    cover: '',
    coinBalance: 0,
    role: 'user',
    livePermissionStatus: 'approved',
    ...overrides,
  };
}

describe('auth upload hooks', () => {
  beforeEach(() => {
    httpMock.post.mockReset();
    httpMock.patch.mockReset();
    httpMock.get.mockReset();
    authStoreMock.user = null;
    authStoreMock.setUser.mockReset();
    authStoreMock.login.mockReset();
    authStoreMock.logout.mockReset();
  });

  it('uploads an avatar as multipart form data and refreshes the signed-in user cache', async () => {
    const queryClient = makeQueryClient();
    const uploadedUser = makeUser({ avatar: '/api/uploads/avatars/avatar.jpg' });
    const file = new File(['avatar'], 'avatar.jpg', { type: 'image/jpeg' });
    httpMock.post.mockResolvedValueOnce({
      data: { url: uploadedUser.avatar, user: uploadedUser },
    });

    const { result } = renderHook(() => useUploadAvatar(), {
      wrapper: wrapperFor(queryClient),
    });

    await expect(result.current.mutateAsync(file)).resolves.toEqual({
      url: uploadedUser.avatar,
      user: uploadedUser,
    });

    expect(httpMock.post).toHaveBeenCalledWith('/users/me/avatar', expect.any(FormData), {
      headers: { 'Content-Type': 'multipart/form-data' },
    });
    const form = httpMock.post.mock.calls[0][1] as FormData;
    expect(form.get('file')).toBe(file);
    expect(queryClient.getQueryData(['me'])).toEqual(uploadedUser);
    expect(authStoreMock.setUser).toHaveBeenCalledWith(uploadedUser);
  });

  it('uploads a channel cover and invalidates public profile aliases', async () => {
    const queryClient = makeQueryClient();
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');
    const uploadedUser = makeUser({ cover: '/api/uploads/covers/channel.jpg' });
    const file = new File(['cover'], 'channel.jpg', { type: 'image/jpeg' });
    httpMock.post.mockResolvedValueOnce({
      data: { url: uploadedUser.cover, user: uploadedUser },
    });

    const { result } = renderHook(() => useUploadChannelCover(), {
      wrapper: wrapperFor(queryClient),
    });

    await expect(result.current.mutateAsync(file)).resolves.toEqual({
      url: uploadedUser.cover,
      user: uploadedUser,
    });

    expect(httpMock.post).toHaveBeenCalledWith('/users/me/cover', expect.any(FormData), {
      headers: { 'Content-Type': 'multipart/form-data' },
    });
    const form = httpMock.post.mock.calls[0][1] as FormData;
    expect(form.get('file')).toBe(file);
    expect(queryClient.getQueryData(['me'])).toEqual(uploadedUser);
    expect(authStoreMock.setUser).toHaveBeenCalledWith(uploadedUser);
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['public-user', 'user-1'] });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['public-user', 'streamer'] });
  });
});
