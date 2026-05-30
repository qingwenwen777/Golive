import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AxiosError } from 'axios';
import { http } from '@/lib/axios';
import type { LuckyBagAmountMode, LuckyBagEligibility, LuckyBagView } from '@/types/luckyBag';

export function luckyBagQueryKey(roomId: string) {
  return ['lucky-bag', roomId] as const;
}

export type LuckyBagErrorReason =
  | 'insufficient_coin'
  | 'active_lucky_bag_exists'
  | 'lucky_bag_closed'
  | 'lucky_bag_already_joined'
  | 'lucky_bag_not_eligible'
  | 'lucky_bag_not_found'
  | 'bad_lucky_bag'
  | 'forbidden'
  | 'unknown'
  | 'network';

export interface LuckyBagError {
  reason: LuckyBagErrorReason;
  message: string;
}

function toLuckyBagError(err: unknown): LuckyBagError {
  if (err instanceof AxiosError) {
    const data = err.response?.data as
      | { reason?: LuckyBagErrorReason; message?: string }
      | undefined;
    if (!err.response) return { reason: 'network', message: 'Network error' };
    return {
      reason: data?.reason ?? 'unknown',
      message: data?.message ?? err.message,
    };
  }
  return { reason: 'unknown', message: String(err) };
}

export interface OpenLuckyBagPayload {
  totalCoin: number;
  count: number;
  amountMode: LuckyBagAmountMode;
  eligibility: LuckyBagEligibility;
  minFanLevel: number;
  durationSeconds: number;
  message?: string;
}

export function useLatestLuckyBag(roomId: string, enabled = true) {
  return useQuery<LuckyBagView, Error>({
    queryKey: luckyBagQueryKey(roomId),
    queryFn: async ({ signal }) => {
      const { data } = await http.get<LuckyBagView>('/lucky-bags/latest', {
        params: { roomId },
        signal,
      });
      return data;
    },
    enabled: enabled && !!roomId,
    staleTime: 0,
    refetchOnMount: 'always',
    refetchOnWindowFocus: true,
    refetchInterval: 2_000,
    structuralSharing: (oldData, newData) => {
      const oldView = oldData as LuckyBagView | undefined;
      const newView = newData as LuckyBagView;
      if (
        newView.bag === null &&
        oldView?.bag &&
        oldView.bag.status !== 'drawn' &&
        oldView.bag.status !== 'cancelled'
      ) {
        return oldView;
      }
      return newView;
    },
  });
}

export function useOpenLuckyBag(roomId: string) {
  const qc = useQueryClient();
  return useMutation<LuckyBagView, LuckyBagError, OpenLuckyBagPayload>({
    mutationFn: async (payload) => {
      try {
        const { data } = await http.post<LuckyBagView>('/lucky-bags', { roomId, ...payload });
        return data;
      } catch (err) {
        throw toLuckyBagError(err);
      }
    },
    onSuccess: (data) => {
      qc.setQueryData(luckyBagQueryKey(roomId), data);
      void qc.invalidateQueries({ queryKey: luckyBagQueryKey(roomId) });
      void qc.invalidateQueries({ queryKey: ['me'] });
    },
  });
}

export function useJoinLuckyBag(roomId: string) {
  const qc = useQueryClient();
  return useMutation<LuckyBagView, LuckyBagError, { bagId: string }>({
    mutationFn: async ({ bagId }) => {
      try {
        const { data } = await http.post<LuckyBagView>(
          `/lucky-bags/${encodeURIComponent(bagId)}/join`,
          { roomId },
        );
        return data;
      } catch (err) {
        throw toLuckyBagError(err);
      }
    },
    onSuccess: (data) => {
      qc.setQueryData(luckyBagQueryKey(roomId), data);
      void qc.invalidateQueries({ queryKey: luckyBagQueryKey(roomId) });
    },
  });
}

export function useCancelLuckyBag(roomId: string) {
  const qc = useQueryClient();
  return useMutation<LuckyBagView, LuckyBagError, { bagId: string }>({
    mutationFn: async ({ bagId }) => {
      try {
        const { data } = await http.post<LuckyBagView>(
          `/lucky-bags/${encodeURIComponent(bagId)}/cancel`,
        );
        return data;
      } catch (err) {
        throw toLuckyBagError(err);
      }
    },
    onSuccess: (data) => {
      qc.setQueryData(luckyBagQueryKey(roomId), data);
      void qc.invalidateQueries({ queryKey: ['me'] });
    },
  });
}
