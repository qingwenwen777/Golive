import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AxiosError } from 'axios';
import { http } from '@/lib/axios';
import type { MicLinkEligibility, MicLinkView } from '@/types/micLink';

export function micLinkQueryKey(roomId: string) {
  return ['mic-link', roomId] as const;
}

export type MicLinkErrorReason =
  | 'mic_link_disabled'
  | 'mic_link_not_eligible'
  | 'mic_link_request_exists'
  | 'mic_link_slot_full'
  | 'mic_link_request_not_found'
  | 'mic_link_busy'
  | 'forbidden'
  | 'unknown'
  | 'network';

export interface MicLinkError {
  reason: MicLinkErrorReason;
  message: string;
}

function toMicLinkError(err: unknown): MicLinkError {
  if (err instanceof AxiosError) {
    const data = err.response?.data as
      | { reason?: MicLinkErrorReason; message?: string }
      | undefined;
    if (!err.response) return { reason: 'network', message: 'Network error' };
    return { reason: data?.reason ?? 'unknown', message: data?.message ?? err.message };
  }
  return { reason: 'unknown', message: String(err) };
}

export function useLatestMicLink(roomId: string, enabled = true) {
  return useQuery<MicLinkView, Error>({
    queryKey: micLinkQueryKey(roomId),
    queryFn: async ({ signal }) => {
      const { data } = await http.get<MicLinkView>('/mic-link/latest', {
        params: { roomId },
        signal,
      });
      return data;
    },
    enabled: enabled && !!roomId,
    staleTime: 0,
    refetchOnMount: 'always',
    refetchOnWindowFocus: true,
    refetchInterval: 3_000,
  });
}

export interface MicConfigPayload {
  enabled: boolean;
  eligibility: MicLinkEligibility;
  minFanLevel: number;
}

function useMicLinkMutation<TVars>(
  roomId: string,
  request: (vars: TVars) => Promise<MicLinkView>,
) {
  const qc = useQueryClient();
  return useMutation<MicLinkView, MicLinkError, TVars>({
    mutationFn: async (vars) => {
      try {
        return await request(vars);
      } catch (err) {
        throw toMicLinkError(err);
      }
    },
    onSuccess: (data) => {
      qc.setQueryData(micLinkQueryKey(roomId), data);
      void qc.invalidateQueries({ queryKey: micLinkQueryKey(roomId) });
    },
  });
}

export function useConfigMicLink(roomId: string) {
  return useMicLinkMutation<MicConfigPayload>(roomId, async (payload) => {
    const { data } = await http.post<MicLinkView>('/mic-link/config', { roomId, ...payload });
    return data;
  });
}

export function useRequestMicLink(roomId: string) {
  return useMicLinkMutation<void>(roomId, async () => {
    const { data } = await http.post<MicLinkView>('/mic-link/request', { roomId });
    return data;
  });
}

export function useCancelMicLink(roomId: string) {
  return useMicLinkMutation<void>(roomId, async () => {
    const { data } = await http.post<MicLinkView>('/mic-link/cancel', { roomId });
    return data;
  });
}

export function useLeaveMicLink(roomId: string) {
  return useMicLinkMutation<void>(roomId, async () => {
    const { data } = await http.post<MicLinkView>('/mic-link/leave', { roomId });
    return data;
  });
}

export function useMuteMicLink(roomId: string) {
  return useMicLinkMutation<{ muted: boolean }>(roomId, async ({ muted }) => {
    const { data } = await http.post<MicLinkView>('/mic-link/mute', { roomId, muted });
    return data;
  });
}

export function useApproveMicLink(roomId: string) {
  return useMicLinkMutation<{ targetId: string }>(roomId, async ({ targetId }) => {
    const { data } = await http.post<MicLinkView>('/mic-link/approve', { roomId, targetId });
    return data;
  });
}

export function useRejectMicLink(roomId: string) {
  return useMicLinkMutation<{ targetId: string }>(roomId, async ({ targetId }) => {
    const { data } = await http.post<MicLinkView>('/mic-link/reject', { roomId, targetId });
    return data;
  });
}

export function useRemoveMicLink(roomId: string) {
  return useMicLinkMutation<{ targetId: string }>(roomId, async ({ targetId }) => {
    const { data } = await http.post<MicLinkView>('/mic-link/remove', { roomId, targetId });
    return data;
  });
}
