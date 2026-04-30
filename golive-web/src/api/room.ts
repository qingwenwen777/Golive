import {
  useMutation,
  useQuery,
  useQueryClient,
  keepPreviousData,
} from '@tanstack/react-query';
import { http } from '@/lib/axios';
import type { PaginatedRooms, RoomsQuery, Stream } from '@/types/stream';

function normalizeCategory(cat?: string): string {
  if (!cat || cat.toLowerCase() === 'all' || cat === 'すべて') return 'all';
  return cat;
}

export function useRooms(params: RoomsQuery = {}) {
  const category = normalizeCategory(params.category);
  const page = params.page ?? 1;
  const size = params.size ?? 24;

  return useQuery<PaginatedRooms, Error>({
    queryKey: ['rooms', { category, page, size }],
    queryFn: async ({ signal }) => {
      const search: Record<string, string | number> = { page, size };
      if (category !== 'all') search.category = category;
      const { data } = await http.get<PaginatedRooms>('/rooms', { params: search, signal });
      return data;
    },
    staleTime: 30_000,
    placeholderData: keepPreviousData,
    retry: 1,
  });
}

export function useRoom(id: string, enabled = true) {
  return useQuery<Stream, Error>({
    queryKey: ['room', id],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<Stream>(`/rooms/${id}`, { signal });
      return data;
    },
    enabled: enabled && !!id,
    staleTime: 30_000,
    retry: 1,
  });
}

export interface GoLivePayload {
  title: string;
  description?: string;
  category: string;
  cover?: string;
  channelName?: string;
  avatar?: string;
}

export function useGoLive() {
  const qc = useQueryClient();
  return useMutation<Stream, Error, GoLivePayload>({
    mutationFn: async (payload) => {
      const { data } = await http.post<Stream>('/rooms/live', payload);
      return data;
    },
    onSuccess: (stream) => {
      qc.setQueryData(['room', stream.id], stream);
      void qc.invalidateQueries({ queryKey: ['rooms'] });
    },
  });
}

export function useStopLive() {
  const qc = useQueryClient();
  return useMutation<{ ok: boolean }, Error, void>({
    mutationFn: async () => {
      const { data } = await http.delete<{ ok: boolean }>('/rooms/live');
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['rooms'] });
    },
  });
}

export function useUploadLiveCover() {
  return useMutation<{ url: string }, Error, File>({
    mutationFn: async (file) => {
      const form = new FormData();
      form.append('file', file);
      const { data } = await http.post<{ url: string }>('/rooms/live/cover', form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      return { url: normalizeUploadedCoverUrl(data.url) };
    },
  });
}

function normalizeUploadedCoverUrl(url: string): string {
  if (/^https?:\/\//i.test(url) || url.startsWith('/')) return url;

  const apiBase = String(import.meta.env.VITE_API_BASE || '');
  if (/^https?:\/\//i.test(apiBase)) return new URL(url, apiBase).href;

  return `/${url.replace(/^\/+/, '')}`;
}

export interface FollowState {
  channelId: string;
  following: boolean;
}

export function useFollowState(channelId: string, enabled: boolean) {
  return useQuery<FollowState, Error>({
    queryKey: ['follow', channelId],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<FollowState>(`/rooms/${channelId}/follow`, { signal });
      return data;
    },
    enabled: enabled && !!channelId,
    staleTime: 60_000,
    retry: 0,
  });
}

export function useFollow(channelId: string) {
  const qc = useQueryClient();
  return useMutation<FollowState, Error, void, { prev?: FollowState }>({
    mutationFn: async () => {
      const { data } = await http.post<FollowState>(`/rooms/${channelId}/follow`);
      return data;
    },
    onMutate: async () => {
      await qc.cancelQueries({ queryKey: ['follow', channelId] });
      const prev = qc.getQueryData<FollowState>(['follow', channelId]);
      qc.setQueryData<FollowState>(['follow', channelId], { channelId, following: true });
      return { prev };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(['follow', channelId], ctx.prev);
      else qc.setQueryData<FollowState>(['follow', channelId], { channelId, following: false });
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ['follow', channelId] });
    },
  });
}

export function useUnfollow(channelId: string) {
  const qc = useQueryClient();
  return useMutation<FollowState, Error, void, { prev?: FollowState }>({
    mutationFn: async () => {
      const { data } = await http.delete<FollowState>(`/rooms/${channelId}/follow`);
      return data;
    },
    onMutate: async () => {
      await qc.cancelQueries({ queryKey: ['follow', channelId] });
      const prev = qc.getQueryData<FollowState>(['follow', channelId]);
      qc.setQueryData<FollowState>(['follow', channelId], { channelId, following: false });
      return { prev };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(['follow', channelId], ctx.prev);
      else qc.setQueryData<FollowState>(['follow', channelId], { channelId, following: true });
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ['follow', channelId] });
    },
  });
}

export interface LikeState {
  streamId: string;
  liked: boolean;
  disliked: boolean;
  likes: number;
}

export function useLikeState(streamId: string, enabled: boolean) {
  return useQuery<LikeState, Error>({
    queryKey: ['like', streamId],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<LikeState>(`/rooms/${streamId}/like`, { signal });
      return data;
    },
    enabled: enabled && !!streamId,
    staleTime: 30_000,
    retry: 0,
  });
}

type LikeAction = 'like' | 'unlike' | 'dislike' | 'undislike';

export function useLike(streamId: string) {
  const qc = useQueryClient();
  return useMutation<LikeState, Error, LikeAction, { prev?: LikeState }>({
    mutationFn: async (action) => {
      const path =
        action === 'like' || action === 'unlike'
          ? `/rooms/${streamId}/like`
          : `/rooms/${streamId}/dislike`;
      const method = action === 'unlike' || action === 'undislike' ? 'delete' : 'post';
      const { data } = await http.request<LikeState>({ url: path, method });
      return data;
    },
    onMutate: async (action) => {
      await qc.cancelQueries({ queryKey: ['like', streamId] });
      const prev = qc.getQueryData<LikeState>(['like', streamId]);
      if (prev) {
        const next: LikeState = { ...prev };
        if (action === 'like') {
          next.liked = true;
          next.disliked = false;
          next.likes = prev.likes + (prev.liked ? 0 : 1);
        } else if (action === 'unlike') {
          next.liked = false;
          next.likes = Math.max(0, prev.likes - (prev.liked ? 1 : 0));
        } else if (action === 'dislike') {
          next.disliked = true;
          next.liked = false;
          next.likes = Math.max(0, prev.likes - (prev.liked ? 1 : 0));
        } else {
          next.disliked = false;
        }
        qc.setQueryData<LikeState>(['like', streamId], next);
      }
      return { prev };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(['like', streamId], ctx.prev);
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ['like', streamId] });
    },
  });
}
