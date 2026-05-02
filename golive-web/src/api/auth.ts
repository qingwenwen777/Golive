import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';
import { http } from '@/lib/axios';
import { coinTransactionsKey } from '@/api/coins';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import type { LoginResp, User } from '@/types/user';

export interface LoginPayload {
  username: string;
  password: string;
}

export interface RegisterPayload {
  username: string;
  password: string;
  displayName: string;
}

export function useLoginMutation() {
  const login = useAuthStore((s) => s.login);
  const qc = useQueryClient();

  return useMutation<LoginResp, Error, LoginPayload>({
    mutationFn: async (payload) => {
      const { data } = await http.post<LoginResp>('/auth/login', payload);
      return data;
    },
    onSuccess: (data) => {
      login(data);
      void qc.invalidateQueries({ queryKey: ['me'] });
    },
  });
}

export function useRegisterMutation() {
  const login = useAuthStore((s) => s.login);
  const qc = useQueryClient();

  return useMutation<LoginResp, Error, RegisterPayload>({
    mutationFn: async (payload) => {
      const { data } = await http.post<LoginResp>('/auth/register', payload);
      return data;
    },
    onSuccess: (data) => {
      login(data);
      void qc.invalidateQueries({ queryKey: ['me'] });
    },
  });
}

export function useMe() {
  const isAuthed = useIsAuthed();
  const query = useQuery<User, Error>({
    queryKey: ['me'],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<User>('/users/me', { signal });
      return data;
    },
    enabled: isAuthed,
    staleTime: 60_000,
  });

  useEffect(() => {
    if (query.data) {
      useAuthStore.getState().setUser(query.data);
    }
  }, [query.data]);

  return query;
}

export function usePublicUser(id: string) {
  return useQuery<User, Error>({
    queryKey: ['public-user', id],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<User>(`/users/profile/${encodeURIComponent(id)}`, { signal });
      return data;
    },
    enabled: !!id,
    staleTime: 60_000,
    retry: 1,
  });
}

export function useTopupCoins() {
  const qc = useQueryClient();
  return useMutation<User, Error, { amount: number }>({
    mutationFn: async ({ amount }) => {
      const { data } = await http.post<User>('/users/me/coins/topup', { amount });
      return data;
    },
    onSuccess: (user) => {
      qc.setQueryData(['me'], user);
      useAuthStore.getState().setUser(user);
      void qc.invalidateQueries({ queryKey: coinTransactionsKey });
    },
  });
}

export interface AvatarUploadResp {
  url: string;
  user: User;
}

export function useUploadAvatar() {
  const qc = useQueryClient();
  return useMutation<AvatarUploadResp, Error, File>({
    mutationFn: async (file) => {
      const form = new FormData();
      form.append('file', file);
      const { data } = await http.post<AvatarUploadResp>('/users/me/avatar', form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      return data;
    },
    onSuccess: ({ user }) => {
      qc.setQueryData(['me'], user);
      useAuthStore.getState().setUser(user);
    },
  });
}

export interface ChannelCoverUploadResp {
  url: string;
  user: User;
}

export function useUploadChannelCover() {
  const qc = useQueryClient();
  return useMutation<ChannelCoverUploadResp, Error, File>({
    mutationFn: async (file) => {
      const form = new FormData();
      form.append('file', file);
      const { data } = await http.post<ChannelCoverUploadResp>('/users/me/cover', form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      return data;
    },
    onSuccess: ({ user }) => {
      qc.setQueryData(['me'], user);
      useAuthStore.getState().setUser(user);
      void qc.invalidateQueries({ queryKey: ['public-user', user.id] });
      void qc.invalidateQueries({ queryKey: ['public-user', user.username] });
    },
  });
}

export async function logout(): Promise<void> {
  try {
    await http.post('/auth/logout').catch(() => undefined);
  } finally {
    useAuthStore.getState().logout();
  }
}
