import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';
import { http } from '@/lib/axios';
import { coinTransactionsKey } from '@/api/coins';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import type { LoginResp, User } from '@/types/user';

export interface LoginPayload {
  username: string;
  password: string;
  captchaId: string;
  captchaCode: string;
}

export interface RegisterPayload {
  username: string;
  password: string;
  displayName: string;
  email: string;
  inviteCode: string;
  captchaId: string;
  captchaCode: string;
}

export interface CaptchaChallenge {
  id: string;
  image: string;
  expiresIn: number;
}

export interface ResetPasswordPayload {
  email: string;
  newPassword: string;
}

export interface GoogleCredentialPayload {
  credential: string;
}

export interface GoogleRegisterPayload extends GoogleCredentialPayload {
  username: string;
  displayName: string;
  inviteCode: string;
}

export interface GoogleLinkExistingPayload extends GoogleCredentialPayload {
  password: string;
}

export interface GoogleUnbindPayload {
  password: string;
}

export interface UpdateProfilePayload {
  username: string;
  displayName: string;
}

export interface ChangePasswordPayload {
  currentPassword: string;
  newPassword: string;
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
      scheduleAuthPageRefresh();
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
      scheduleAuthPageRefresh();
    },
  });
}

export function useGoogleLoginMutation() {
  const login = useAuthStore((s) => s.login);
  const qc = useQueryClient();

  return useMutation<LoginResp, Error, GoogleCredentialPayload>({
    mutationFn: async (payload) => {
      const { data } = await http.post<LoginResp>('/auth/google/login', payload);
      return data;
    },
    onSuccess: (data) => {
      login(data);
      void qc.invalidateQueries({ queryKey: ['me'] });
    },
  });
}

export function useGoogleRegisterMutation() {
  const login = useAuthStore((s) => s.login);
  const qc = useQueryClient();

  return useMutation<LoginResp, Error, GoogleRegisterPayload>({
    mutationFn: async (payload) => {
      const { data } = await http.post<LoginResp>('/auth/google/register', payload);
      return data;
    },
    onSuccess: (data) => {
      login(data);
      void qc.invalidateQueries({ queryKey: ['me'] });
    },
  });
}

export function useGoogleLinkExistingMutation() {
  const login = useAuthStore((s) => s.login);
  const qc = useQueryClient();

  return useMutation<LoginResp, Error, GoogleLinkExistingPayload>({
    mutationFn: async (payload) => {
      const { data } = await http.post<LoginResp>('/auth/google/link-existing', payload);
      return data;
    },
    onSuccess: (data) => {
      login(data);
      void qc.invalidateQueries({ queryKey: ['me'] });
    },
  });
}

export async function fetchCaptcha(): Promise<CaptchaChallenge> {
  const { data } = await http.get<CaptchaChallenge>('/auth/captcha');
  return data;
}

export function useResetPasswordMutation() {
  return useMutation<{ ok: boolean }, Error, ResetPasswordPayload>({
    mutationFn: async (payload) => {
      const { data } = await http.post<{ ok: boolean }>('/auth/password/reset', payload);
      return data;
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
    if (isAuthed && query.data) {
      useAuthStore.getState().setUser(query.data);
    }
  }, [isAuthed, query.data]);

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

export function useUpdateProfile() {
  const qc = useQueryClient();
  return useMutation<User, Error, UpdateProfilePayload>({
    mutationFn: async (payload) => {
      const { data } = await http.patch<User>('/users/me/profile', payload);
      return data;
    },
    onSuccess: (user) => {
      const prev = useAuthStore.getState().user;
      qc.setQueryData(['me'], user);
      useAuthStore.getState().setUser(user);
      void qc.invalidateQueries({ queryKey: ['public-user', user.id] });
      void qc.invalidateQueries({ queryKey: ['public-user', user.username] });
      if (prev?.username && prev.username !== user.username) {
        void qc.invalidateQueries({ queryKey: ['public-user', prev.username] });
      }
    },
  });
}

export function useChangePassword() {
  return useMutation<{ ok: boolean }, Error, ChangePasswordPayload>({
    mutationFn: async (payload) => {
      const { data } = await http.post<{ ok: boolean }>('/users/me/password', payload);
      return data;
    },
  });
}

export function useBindGoogleAccount() {
  const qc = useQueryClient();
  return useMutation<User, Error, GoogleCredentialPayload>({
    mutationFn: async (payload) => {
      const { data } = await http.post<User>('/auth/google/bind', payload);
      return data;
    },
    onSuccess: (user) => {
      qc.setQueryData(['me'], user);
      useAuthStore.getState().setUser(user);
    },
  });
}

export function useUnbindGoogleAccount() {
  const qc = useQueryClient();
  return useMutation<User, Error, GoogleUnbindPayload>({
    mutationFn: async (payload) => {
      const { data } = await http.post<User>('/auth/google/unbind', payload);
      return data;
    },
    onSuccess: (user) => {
      qc.setQueryData(['me'], user);
      useAuthStore.getState().setUser(user);
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
      void qc.invalidateQueries({ queryKey: ['public-user', user.id] });
      void qc.invalidateQueries({ queryKey: ['public-user', user.username] });
      void qc.invalidateQueries({ queryKey: ['room'] });
      void qc.invalidateQueries({ queryKey: ['rooms'] });
      void qc.invalidateQueries({ queryKey: ['fan-badges'] });
      void qc.invalidateQueries({ queryKey: ['channel-appointments'] });
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
    scheduleAuthPageRefresh();
  }
}

function scheduleAuthPageRefresh(): void {
  if (typeof window === 'undefined') return;
  window.setTimeout(() => {
    window.location.reload();
  }, 0);
}
