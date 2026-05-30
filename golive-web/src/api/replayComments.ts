import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { http } from '@/lib/axios';
import type { PostAuthor } from '@/api/posts';

export interface ReplayComment {
  id: string;
  roomId: string;
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

export interface ReplayCommentListResp {
  items: ReplayComment[];
  total: number;
  canComment: boolean;
}

export interface CreateReplayCommentPayload {
  content: string;
  parentId?: string;
}

export interface ReplayCommentLikeState {
  commentId: string;
  liked: boolean;
  likes: number;
}

export function useReplayComments(roomId: string, enabled = true) {
  return useQuery<ReplayCommentListResp, Error>({
    queryKey: ['replay-comments', roomId],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<ReplayCommentListResp>(
        `/rooms/${encodeURIComponent(roomId)}/replay/comments`,
        { signal },
      );
      return data;
    },
    enabled: enabled && !!roomId,
    staleTime: 10_000,
    retry: 1,
  });
}

export function useCreateReplayComment(roomId: string) {
  const qc = useQueryClient();
  return useMutation<ReplayComment, Error, CreateReplayCommentPayload>({
    mutationFn: async (payload) => {
      const { data } = await http.post<ReplayComment>(
        `/rooms/${encodeURIComponent(roomId)}/replay/comments`,
        payload,
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['replay-comments', roomId] });
    },
  });
}

export function useDeleteReplayComment(roomId: string) {
  const qc = useQueryClient();
  return useMutation<{ ok: boolean }, Error, string>({
    mutationFn: async (commentId) => {
      const { data } = await http.delete<{ ok: boolean }>(
        `/rooms/${encodeURIComponent(roomId)}/replay/comments/${encodeURIComponent(commentId)}`,
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['replay-comments', roomId] });
    },
  });
}

export function useToggleReplayCommentLike(roomId: string, commentId: string) {
  const qc = useQueryClient();
  return useMutation<ReplayCommentLikeState, Error, 'like' | 'unlike'>({
    mutationFn: async (action) => {
      const method = action === 'like' ? 'post' : 'delete';
      const { data } = await http.request<ReplayCommentLikeState>({
        method,
        url: `/rooms/${encodeURIComponent(roomId)}/replay/comments/${encodeURIComponent(commentId)}/like`,
      });
      return data;
    },
    onSuccess: (state) => {
      qc.setQueryData<ReplayCommentListResp>(['replay-comments', roomId], (old) =>
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
      void qc.invalidateQueries({ queryKey: ['replay-comments', roomId] });
    },
  });
}
