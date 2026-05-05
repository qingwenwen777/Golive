import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { http } from '@/lib/axios';
import { useAuthStore } from '@/stores/useAuthStore';
import type { User } from '@/types/user';

export type LivePermissionStatus = User['livePermissionStatus'];

export interface CreatorApplication {
  id: string;
  userId: string;
  username?: string;
  displayName?: string;
  avatar?: string;
  reason?: string;
  status: LivePermissionStatus;
  reviewerId?: string;
  rejectReason?: string;
  reviewedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface CreatorApplicationResp {
  application?: CreatorApplication;
  livePermissionStatus: LivePermissionStatus;
  user: User;
  message: string;
}

export function useSubmitCreatorApplication() {
  const qc = useQueryClient();
  return useMutation<CreatorApplicationResp, Error, { reason: string }>({
    mutationFn: async (payload) => {
      const { data } = await http.post<CreatorApplicationResp>('/creator/applications', payload);
      return data;
    },
    onSuccess: ({ user }) => {
      qc.setQueryData(['me'], user);
      useAuthStore.getState().setUser(user);
      void qc.invalidateQueries({ queryKey: ['admin-creator-applications'] });
    },
  });
}

export interface AdminApplicationsResp {
  items: CreatorApplication[];
}

export function useAdminCreatorApplications(enabled = true) {
  return useQuery<AdminApplicationsResp, Error>({
    queryKey: ['admin-creator-applications'],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<AdminApplicationsResp>('/admin/creator-applications', {
        signal,
      });
      return data;
    },
    staleTime: 15_000,
    enabled,
  });
}

export function useReviewCreatorApplication(action: 'approve' | 'reject') {
  const qc = useQueryClient();
  return useMutation<{ application: CreatorApplication; user: User }, Error, { id: string; reason?: string }>({
    mutationFn: async ({ id, reason }) => {
      const { data } = await http.post<{ application: CreatorApplication; user: User }>(
        `/admin/creator-applications/${encodeURIComponent(id)}/${action}`,
        action === 'reject' ? { reason } : undefined,
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-creator-applications'] });
      void qc.invalidateQueries({ queryKey: ['admin-live-creators'] });
    },
  });
}

export interface LiveCreator {
  id: string;
  username: string;
  displayName?: string;
  avatar?: string;
  livePermissionStatus: LivePermissionStatus;
  updatedAt: string;
}

export interface AdminLiveCreatorsResp {
  items: LiveCreator[];
}

export function useAdminLiveCreators(enabled = true) {
  return useQuery<AdminLiveCreatorsResp, Error>({
    queryKey: ['admin-live-creators'],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<AdminLiveCreatorsResp>('/admin/live-creators', {
        signal,
      });
      return data;
    },
    staleTime: 15_000,
    enabled,
  });
}

export function useUpdateLivePermission() {
  const qc = useQueryClient();
  return useMutation<{ user: User }, Error, { id: string; status: Extract<LivePermissionStatus, 'approved' | 'rejected'> }>({
    mutationFn: async ({ id, status }) => {
      const { data } = await http.post<{ user: User }>(
        `/admin/users/${encodeURIComponent(id)}/live-permission`,
        { status },
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-live-creators'] });
      void qc.invalidateQueries({ queryKey: ['admin-creator-applications'] });
    },
  });
}

export interface AdminInviteCode {
  id: string;
  code: string;
  createdBy: string;
  used: boolean;
  usedBy?: string;
  usedUsername?: string;
  usedDisplayName?: string;
  usedEmail?: string;
  usedAt?: string;
  createdAt: string;
}

export interface AdminInviteCodesResp {
  items: AdminInviteCode[];
}

export function useAdminInviteCodes(enabled = true) {
  return useQuery<AdminInviteCodesResp, Error>({
    queryKey: ['admin-invite-codes'],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<AdminInviteCodesResp>('/admin/invite-codes', {
        signal,
      });
      return data;
    },
    staleTime: 15_000,
    enabled,
  });
}

export function useCreateInviteCode() {
  const qc = useQueryClient();
  return useMutation<{ inviteCode: AdminInviteCode }, Error, void>({
    mutationFn: async () => {
      const { data } = await http.post<{ inviteCode: AdminInviteCode }>('/admin/invite-codes');
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-invite-codes'] });
    },
  });
}

export function useDeleteInviteCode() {
  const qc = useQueryClient();
  return useMutation<{ inviteCode: AdminInviteCode }, Error, { id: string }>({
    mutationFn: async ({ id }) => {
      const { data } = await http.delete<{ inviteCode: AdminInviteCode }>(
        `/admin/invite-codes/${encodeURIComponent(id)}`,
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-invite-codes'] });
    },
  });
}
