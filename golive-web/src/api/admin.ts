import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { http } from '@/lib/axios';
import type { User } from '@/types/user';
import type { AdminAuditLog } from '@/api/contentModeration';

export type AdminUserRole = 'user' | 'admin' | 'moderator';
export type AdminUserStatus =
  | 'all'
  | 'active'
  | 'banned'
  | 'appeal_pending'
  | 'frozen'
  | 'live_approved';
export type CoinAdjustAction = 'add' | 'deduct' | 'freeze' | 'unfreeze';
export type AdminUnbanAppealStatus = 'pending' | 'reviewing' | 'approved' | 'rejected';

export interface AdminHealthItem {
  key: string;
  label: string;
  status: 'ok' | 'down' | 'unknown' | string;
  detail?: string;
  checked: boolean;
}

export interface AdminOverview {
  onlineRooms: number;
  onlineViewers: number;
  todayNewUsers: number;
  todayRevenueCoins: number;
  health: AdminHealthItem[];
}

export interface AdminUserListItem extends User {
  email?: string;
  frozenCoins: number;
  banned?: boolean;
  banReason?: string;
  pendingAppeals?: number;
  createdAt: string;
  updatedAt: string;
}

export interface AdminUserStats {
  total: number;
  active: number;
  banned: number;
  admins: number;
  moderators: number;
  pendingAppeals: number;
}

export interface AdminUserListResp {
  items: AdminUserListItem[];
  total: number;
  page: number;
  size: number;
  stats: AdminUserStats;
}

export interface CoinTransaction {
  id: string;
  userId: string;
  type: string;
  amount: number;
  balanceAfter: number;
  title: string;
  description?: string;
  createdAt: string;
}

export interface AdminLiveRecord {
  id: string;
  title: string;
  status: string;
  viewers: number;
  peakViewers: number;
  startedAt: string;
  endedAt?: string;
}

export interface AdminReportRecord {
  id: string;
  targetType: string;
  targetId: string;
  reason: string;
  status: string;
  reporterId: string;
  reporterName: string;
  targetTitle?: string;
  targetText?: string;
  resolutionAction?: string;
  createdAt: string;
  resolvedAt?: string;
}

export interface AdminUnbanAppealRecord {
  id: string;
  userId: string;
  reason: string;
  status: AdminUnbanAppealStatus;
  reviewerId?: string;
  reviewer?: string;
  reviewNote?: string;
  reviewedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AdminUserDetail {
  user: AdminUserListItem;
  coinTransactions: CoinTransaction[];
  liveRecords: AdminLiveRecord[];
  reportRecords: AdminReportRecord[];
  appealRecords: AdminUnbanAppealRecord[];
}

export function useAdminOverview(enabled = true) {
  return useQuery<AdminOverview, Error>({
    queryKey: ['admin-overview'],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<AdminOverview>('/rooms/admin/dashboard', { signal });
      return data;
    },
    enabled,
    staleTime: 8_000,
    retry: 1,
  });
}

export function useAdminUsers(
  params: {
    q?: string;
    role?: string;
    status?: string;
    page?: number;
    size?: number;
  },
  enabled = true,
) {
  const page = params.page ?? 1;
  const size = params.size ?? 20;
  return useQuery<AdminUserListResp, Error>({
    queryKey: ['admin-users', { ...params, page, size }],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<AdminUserListResp>('/admin/users', {
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

export function useAdminUserDetail(id: string, enabled = true) {
  return useQuery<AdminUserDetail, Error>({
    queryKey: ['admin-user-detail', id],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<AdminUserDetail>(`/admin/users/${encodeURIComponent(id)}`, {
        signal,
      });
      return data;
    },
    enabled: enabled && !!id,
    staleTime: 8_000,
    retry: 1,
  });
}

function useAdminUserMutation<TPayload extends object>(
  fn: (payload: TPayload) => Promise<unknown>,
) {
  const qc = useQueryClient();
  return useMutation<unknown, Error, TPayload>({
    mutationFn: fn,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-users'] });
      void qc.invalidateQueries({ queryKey: ['admin-user-detail'] });
      void qc.invalidateQueries({ queryKey: ['admin-overview'] });
    },
  });
}

export function useAdminUpdateUserProfile(id: string) {
  return useAdminUserMutation<{ username?: string; displayName?: string }>(async (payload) => {
    const { data } = await http.patch(`/admin/users/${encodeURIComponent(id)}/profile`, payload);
    return data;
  });
}

export function useAdminUpdateUserRole(id: string) {
  return useAdminUserMutation<{ role: AdminUserRole }>(async (payload) => {
    const { data } = await http.patch(`/admin/users/${encodeURIComponent(id)}/role`, payload);
    return data;
  });
}

export function useAdminSetUserBan(id: string) {
  return useAdminUserMutation<{ banned: boolean; reason?: string }>(async (payload) => {
    const { data } = await http.patch(`/admin/users/${encodeURIComponent(id)}/ban`, payload);
    return data;
  });
}

export function useAdminReviewUnbanAppeal(id: string, appealId: string) {
  return useAdminUserMutation<{ status: 'reviewing' | 'approved' | 'rejected'; note?: string }>(
    async (payload) => {
      const { data } = await http.patch(
        `/admin/users/${encodeURIComponent(id)}/unban-appeals/${encodeURIComponent(appealId)}`,
        payload,
      );
      return data;
    },
  );
}

export function useAdminAdjustUserCoins(id: string) {
  return useAdminUserMutation<{
    action: CoinAdjustAction;
    amount: number;
    note?: string;
  }>(async (payload) => {
    const { data } = await http.post(`/admin/users/${encodeURIComponent(id)}/coins`, payload);
    return data;
  });
}

export type { AdminAuditLog };
