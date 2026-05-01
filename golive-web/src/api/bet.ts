import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AxiosError } from 'axios';
import { http } from '@/lib/axios';
import type { BetOption, BetRoundView, BetWager } from '@/types/bet';

export function betQueryKey(roomId: string) {
  return ['bet', roomId] as const;
}

export interface BetError {
  reason:
    | 'insufficient_coin'
    | 'active_bet_exists'
    | 'bet_closed'
    | 'bet_already_placed'
    | 'bet_no_winners'
    | 'forbidden'
    | 'unknown'
    | 'network';
  message: string;
}

function toBetError(err: unknown): BetError {
  if (err instanceof AxiosError) {
    const data = err.response?.data as { reason?: BetError['reason']; message?: string } | undefined;
    if (!err.response) return { reason: 'network', message: 'Network error' };
    return {
      reason: data?.reason ?? 'unknown',
      message: data?.message ?? err.message,
    };
  }
  return { reason: 'unknown', message: String(err) };
}

export function useLatestBet(roomId: string, enabled = true) {
  return useQuery<BetRoundView, Error>({
    queryKey: betQueryKey(roomId),
    queryFn: async ({ signal }) => {
      const { data } = await http.get<BetRoundView>('/bets/latest', {
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
  });
}

export function useOpenBet(roomId: string) {
  const qc = useQueryClient();
  return useMutation<BetRoundView, BetError, { amount: number }>({
    mutationFn: async ({ amount }) => {
      try {
        const { data } = await http.post<BetRoundView>('/bets', { roomId, amount });
        return data;
      } catch (err) {
        throw toBetError(err);
      }
    },
    onSuccess: (data) => {
      qc.setQueryData(betQueryKey(roomId), data);
      void qc.invalidateQueries({ queryKey: betQueryKey(roomId) });
      void qc.invalidateQueries({ queryKey: ['me'] });
    },
  });
}

export function usePlaceBet(roomId: string) {
  const qc = useQueryClient();
  return useMutation<BetWager, BetError, { roundId: string; option: BetOption }>({
    mutationFn: async ({ roundId, option }) => {
      try {
        const { data } = await http.post<BetWager>(`/bets/${roundId}/wagers`, { roomId, option });
        return data;
      } catch (err) {
        throw toBetError(err);
      }
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: betQueryKey(roomId) });
      void qc.invalidateQueries({ queryKey: ['me'] });
    },
  });
}

export function useSettleBet(roomId: string) {
  const qc = useQueryClient();
  return useMutation<BetRoundView, BetError, { roundId: string; option: BetOption }>({
    mutationFn: async ({ roundId, option }) => {
      try {
        const { data } = await http.post<BetRoundView>(`/bets/${roundId}/settle`, { option });
        return data;
      } catch (err) {
        throw toBetError(err);
      }
    },
    onSuccess: (data) => {
      qc.setQueryData(betQueryKey(roomId), data);
      void qc.invalidateQueries({ queryKey: ['me'] });
    },
  });
}

export function useCancelBet(roomId: string) {
  const qc = useQueryClient();
  return useMutation<BetRoundView, BetError, { roundId: string }>({
    mutationFn: async ({ roundId }) => {
      try {
        const { data } = await http.post<BetRoundView>(`/bets/${roundId}/cancel`);
        return data;
      } catch (err) {
        throw toBetError(err);
      }
    },
    onSuccess: (data) => {
      qc.setQueryData(betQueryKey(roomId), data);
      void qc.invalidateQueries({ queryKey: ['me'] });
    },
  });
}
