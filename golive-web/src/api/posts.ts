import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { http } from '@/lib/axios';

export type PostVisibility = 'public' | 'followers' | 'private';
export type PostCommentMode = 'everyone' | 'followers';

export interface PostAuthor {
  id: string;
  username?: string;
  displayName?: string;
  name: string;
  avatar?: string;
  verified: boolean;
}

export interface ChannelPost {
  id: string;
  ownerId: string;
  channelId: string;
  content: string;
  images: string[];
  visibility: PostVisibility;
  commentsEnabled: boolean;
  commentMode: PostCommentMode;
  likeCount: number;
  commentCount: number;
  liked: boolean;
  canComment: boolean;
  canDelete: boolean;
  author: PostAuthor;
  createdAt: string;
  updatedAt: string;
}

export interface PostListResp {
  items: ChannelPost[];
  total: number;
  page: number;
  size: number;
}

export interface PostComment {
  id: string;
  postId: string;
  userId: string;
  parentId?: string;
  rootId?: string;
  depth: number;
  content: string;
  likeCount: number;
  replyCount: number;
  liked: boolean;
  canDelete: boolean;
  author: PostAuthor;
  createdAt: string;
  updatedAt: string;
}

export interface PostCommentListResp {
  items: PostComment[];
  total: number;
}

export interface CreatePostPayload {
  content: string;
  images: string[];
  visibility: PostVisibility;
  commentsEnabled: boolean;
  commentMode: PostCommentMode;
}

export interface CreateCommentPayload {
  content: string;
  parentId?: string;
}

export interface PostLikeState {
  postId: string;
  liked: boolean;
  likes: number;
}

export interface CommentLikeState {
  commentId: string;
  liked: boolean;
  likes: number;
}

export function useStudioPosts(enabled = true, page = 1, size = 10) {
  return useQuery<PostListResp, Error>({
    queryKey: ['studio-posts', page, size],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<PostListResp>('/rooms/posts/mine', {
        params: { page, size },
        signal,
      });
      return data;
    },
    enabled,
    staleTime: 15_000,
    placeholderData: keepPreviousData,
    retry: 0,
  });
}

export function useChannelPosts(channelKey: string, enabled = true, page = 1, size = 6) {
  return useQuery<PostListResp, Error>({
    queryKey: ['channel-posts', channelKey, page, size],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<PostListResp>(
        `/rooms/channels/${encodeURIComponent(channelKey)}/posts`,
        { params: { page, size }, signal },
      );
      return data;
    },
    enabled: enabled && !!channelKey,
    staleTime: 20_000,
    placeholderData: keepPreviousData,
    retry: 1,
  });
}

export function useCreatePost() {
  const qc = useQueryClient();
  return useMutation<ChannelPost, Error, CreatePostPayload>({
    mutationFn: async (payload) => {
      const { data } = await http.post<ChannelPost>('/rooms/posts', payload);
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['studio-posts'] });
      void qc.invalidateQueries({ queryKey: ['channel-posts'] });
    },
  });
}

export function useDeletePost() {
  const qc = useQueryClient();
  return useMutation<{ ok: boolean }, Error, string>({
    mutationFn: async (postId) => {
      const { data } = await http.delete<{ ok: boolean }>(`/rooms/posts/${encodeURIComponent(postId)}`);
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['studio-posts'] });
      void qc.invalidateQueries({ queryKey: ['channel-posts'] });
      void qc.invalidateQueries({ queryKey: ['post-comments'] });
    },
  });
}

export function useUploadPostImage() {
  return useMutation<{ url: string }, Error, File>({
    mutationFn: async (file) => {
      const form = new FormData();
      form.append('file', file);
      const { data } = await http.post<{ url: string }>('/rooms/posts/images', form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      return { url: normalizeUploadedPostImageUrl(data.url) };
    },
  });
}

export function usePostComments(postId: string, enabled = true) {
  return useQuery<PostCommentListResp, Error>({
    queryKey: ['post-comments', postId],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<PostCommentListResp>(
        `/rooms/posts/${encodeURIComponent(postId)}/comments`,
        { signal },
      );
      return data;
    },
    enabled: enabled && !!postId,
    staleTime: 10_000,
    retry: 1,
  });
}

export function useCreatePostComment(postId: string) {
  const qc = useQueryClient();
  return useMutation<PostComment, Error, CreateCommentPayload>({
    mutationFn: async (payload) => {
      const { data } = await http.post<PostComment>(
        `/rooms/posts/${encodeURIComponent(postId)}/comments`,
        payload,
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['post-comments', postId] });
      void qc.invalidateQueries({ queryKey: ['studio-posts'] });
      void qc.invalidateQueries({ queryKey: ['channel-posts'] });
    },
  });
}

export function useDeletePostComment(postId: string) {
  const qc = useQueryClient();
  return useMutation<{ ok: boolean }, Error, string>({
    mutationFn: async (commentId) => {
      const { data } = await http.delete<{ ok: boolean }>(
        `/rooms/posts/${encodeURIComponent(postId)}/comments/${encodeURIComponent(commentId)}`,
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['post-comments', postId] });
      void qc.invalidateQueries({ queryKey: ['studio-posts'] });
      void qc.invalidateQueries({ queryKey: ['channel-posts'] });
    },
  });
}

export function useTogglePostLike(postId: string) {
  const qc = useQueryClient();
  return useMutation<PostLikeState, Error, 'like' | 'unlike'>({
    mutationFn: async (action) => {
      const method = action === 'like' ? 'post' : 'delete';
      const { data } = await http.request<PostLikeState>({
        method,
        url: `/rooms/posts/${encodeURIComponent(postId)}/like`,
      });
      return data;
    },
    onSuccess: (state) => {
      qc.setQueriesData<PostListResp>({ queryKey: ['studio-posts'] }, (old) =>
        patchPostLike(old, postId, state.liked, state.likes),
      );
      qc.setQueriesData<PostListResp>({ queryKey: ['channel-posts'] }, (old) =>
        patchPostLike(old, postId, state.liked, state.likes),
      );
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ['studio-posts'] });
      void qc.invalidateQueries({ queryKey: ['channel-posts'] });
    },
  });
}

export function useToggleCommentLike(postId: string, commentId: string) {
  const qc = useQueryClient();
  return useMutation<CommentLikeState, Error, 'like' | 'unlike'>({
    mutationFn: async (action) => {
      const method = action === 'like' ? 'post' : 'delete';
      const { data } = await http.request<CommentLikeState>({
        method,
        url: `/rooms/posts/${encodeURIComponent(postId)}/comments/${encodeURIComponent(commentId)}/like`,
      });
      return data;
    },
    onSuccess: (state) => {
      qc.setQueryData<PostCommentListResp>(['post-comments', postId], (old) =>
        old
          ? {
              ...old,
              items: old.items.map((item) =>
                item.id === commentId
                  ? { ...item, liked: state.liked, likeCount: state.likes }
                  : item,
              ),
            }
          : old,
      );
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ['post-comments', postId] });
    },
  });
}

function patchPostLike(
  old: PostListResp | undefined,
  postId: string,
  liked: boolean,
  likes: number,
): PostListResp | undefined {
  if (!old) return old;
  return {
    ...old,
    items: old.items.map((item) =>
      item.id === postId ? { ...item, liked, likeCount: likes } : item,
    ),
  };
}

function normalizeUploadedPostImageUrl(url: string): string {
  if (/^https?:\/\//i.test(url) || url.startsWith('/')) return url;

  const apiBase = String(import.meta.env.VITE_API_BASE || '');
  if (/^https?:\/\//i.test(apiBase)) return new URL(url, apiBase).href;

  return `/${url.replace(/^\/+/, '')}`;
}
