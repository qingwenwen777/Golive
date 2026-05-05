import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { http } from '@/lib/axios';

export type ReportTargetType =
  | 'room'
  | 'channel'
  | 'danmu'
  | 'post'
  | 'post_comment'
  | 'super_chat';

export type ReportReason =
  | 'spam'
  | 'harassment'
  | 'sexual'
  | 'violence'
  | 'hate'
  | 'scam'
  | 'illegal'
  | 'other';

export type ReportStatus = 'pending' | 'reviewing' | 'resolved' | 'dismissed';

export interface CreateReportPayload {
  targetType: ReportTargetType;
  targetId: string;
  targetUrl?: string;
  roomId?: string;
  channelId?: string;
  targetOwnerId?: string;
  targetOwnerName?: string;
  targetUserId?: string;
  targetUserName?: string;
  targetTitle?: string;
  targetText?: string;
  reason: ReportReason;
  description?: string;
}

export interface ContentReport {
  id: string;
  reporterId: string;
  reporterName: string;
  reporterAvatar?: string;
  targetType: ReportTargetType | string;
  targetId: string;
  targetUrl?: string;
  roomId?: string;
  channelId?: string;
  targetOwnerId?: string;
  targetOwnerName?: string;
  targetUserId?: string;
  targetUserName?: string;
  targetTitle?: string;
  targetText?: string;
  reason: ReportReason | string;
  description?: string;
  status: ReportStatus | string;
  reviewerId?: string;
  resolutionNote?: string;
  resolvedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ReportStats {
  pending: number;
  reviewing: number;
  today: number;
  total: number;
}

export interface ContentReportListResp {
  items: ContentReport[];
  total: number;
  page: number;
  size: number;
  stats: ReportStats;
}

export interface BlockedWord {
  id: string;
  word: string;
  note?: string;
  enabled: boolean;
  createdBy?: string;
  updatedBy?: string;
  createdAt: string;
  updatedAt: string;
}

export interface BlockedWordListResp {
  items: BlockedWord[];
  total: number;
  page: number;
  size: number;
}

export function useSubmitReport() {
  return useMutation<ContentReport, Error, CreateReportPayload>({
    mutationFn: async (payload) => {
      const { data } = await http.post<ContentReport>('/rooms/reports', payload);
      return data;
    },
  });
}

export function useAdminReports(
  params: {
    status?: string;
    targetType?: string;
    reason?: string;
    q?: string;
    page?: number;
    size?: number;
  },
  enabled = true,
) {
  const page = params.page ?? 1;
  const size = params.size ?? 20;
  return useQuery<ContentReportListResp, Error>({
    queryKey: ['admin-reports', { ...params, page, size }],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<ContentReportListResp>('/rooms/admin/reports', {
        params: { ...params, page, size },
        signal,
      });
      return data;
    },
    enabled,
    staleTime: 10_000,
    placeholderData: keepPreviousData,
    retry: 1,
  });
}

export function useUpdateAdminReport() {
  const qc = useQueryClient();
  return useMutation<ContentReport, Error, { id: string; status: ReportStatus; note?: string }>({
    mutationFn: async ({ id, status, note }) => {
      const { data } = await http.patch<ContentReport>(
        `/rooms/admin/reports/${encodeURIComponent(id)}`,
        { status, note },
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-reports'] });
    },
  });
}

export function useAdminBlockedWords(page = 1, size = 50, enabled = true) {
  return useQuery<BlockedWordListResp, Error>({
    queryKey: ['admin-blocked-words', page, size],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<BlockedWordListResp>('/rooms/admin/blocked-words', {
        params: { page, size },
        signal,
      });
      return data;
    },
    enabled,
    staleTime: 10_000,
    placeholderData: keepPreviousData,
    retry: 1,
  });
}

export function useCreateBlockedWord() {
  const qc = useQueryClient();
  return useMutation<BlockedWord, Error, { word: string; note?: string; enabled?: boolean }>({
    mutationFn: async (payload) => {
      const { data } = await http.post<BlockedWord>('/rooms/admin/blocked-words', payload);
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-blocked-words'] });
    },
  });
}

export function useUpdateBlockedWord() {
  const qc = useQueryClient();
  return useMutation<
    BlockedWord,
    Error,
    { id: string; word?: string; note?: string; enabled?: boolean }
  >({
    mutationFn: async ({ id, ...payload }) => {
      const { data } = await http.patch<BlockedWord>(
        `/rooms/admin/blocked-words/${encodeURIComponent(id)}`,
        payload,
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-blocked-words'] });
    },
  });
}

export function useDeleteBlockedWord() {
  const qc = useQueryClient();
  return useMutation<{ ok: boolean }, Error, string>({
    mutationFn: async (id) => {
      const { data } = await http.delete<{ ok: boolean }>(
        `/rooms/admin/blocked-words/${encodeURIComponent(id)}`,
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-blocked-words'] });
    },
  });
}
