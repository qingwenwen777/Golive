import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { http } from '@/lib/axios';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import type { User } from '@/types/user';

export type CoinTransactionType =
  | 'topup'
  | 'daily_task'
  | 'gift_spend'
  | 'super_chat_spend'
  | 'bet_wager'
  | 'bet_payout'
  | 'bet_refund'
  | 'creator_gift_income'
  | 'creator_super_chat_income';

export interface CoinTransaction {
  id: string;
  userId: string;
  type: CoinTransactionType;
  amount: number;
  balanceAfter: number;
  title: string;
  description?: string;
  sourceType?: string;
  sourceId?: string;
  roomId?: string;
  counterpartyId?: string;
  createdAt: string;
}

export interface CoinTransactionListResp {
  items: CoinTransaction[];
}

export interface DailyCoinTaskClaimResp {
  user: User;
  transaction: CoinTransaction;
  task: {
    id: string;
    title: string;
    description: string;
    rewardMin: number;
    rewardMax: number;
  };
  created: boolean;
  alreadyClaimed: boolean;
}

export const coinTransactionsKey = ['coins', 'transactions'] as const;

export function useCoinTransactions() {
  const isAuthed = useIsAuthed();
  return useQuery<CoinTransactionListResp, Error>({
    queryKey: coinTransactionsKey,
    queryFn: async ({ signal }) => {
      const { data } = await http.get<CoinTransactionListResp>('/users/me/coins/transactions', {
        params: { limit: 120 },
        signal,
      });
      return data;
    },
    enabled: isAuthed,
    staleTime: 30_000,
  });
}

export function useClaimDailyCoinTask() {
  const qc = useQueryClient();
  return useMutation<DailyCoinTaskClaimResp, Error, { taskId: string }>({
    mutationFn: async ({ taskId }) => {
      const { data } = await http.post<DailyCoinTaskClaimResp>(
        `/users/me/coins/daily-tasks/${encodeURIComponent(taskId)}/claim`,
      );
      return data;
    },
    onSuccess: (resp) => {
      qc.setQueryData(['me'], resp.user);
      useAuthStore.getState().setUser(resp.user);
      void qc.invalidateQueries({ queryKey: coinTransactionsKey });
    },
  });
}
