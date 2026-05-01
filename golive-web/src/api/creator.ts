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
  status: LivePermissionStatus;
  reviewerId?: string;
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
  return useMutation<CreatorApplicationResp, Error, void>({
    mutationFn: async () => {
      const { data } = await http.post<CreatorApplicationResp>('/creator/applications');
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
  return useMutation<{ application: CreatorApplication; user: User }, Error, string>({
    mutationFn: async (id) => {
      const { data } = await http.post<{ application: CreatorApplication; user: User }>(
        `/admin/creator-applications/${encodeURIComponent(id)}/${action}`,
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-creator-applications'] });
    },
  });
}
