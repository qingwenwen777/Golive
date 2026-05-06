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
  | 'creator_super_chat_income'
  | 'withdrawal';

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

export interface ConfirmTopupResp {
  user: User;
  transaction: CoinTransaction;
  credited: boolean;
  paymentStatus: string;
}

export interface StripeAccountStatus {
  stripeConfigured: boolean;
  testMode: boolean;
  connected: boolean;
  accountId?: string;
  chargesEnabled?: boolean;
  canWithdraw: boolean;
  payoutsEnabled: boolean;
  transfersStatus?: string;
  detailsSubmitted: boolean;
  currentlyDue?: string[];
  disabledReason?: string;
}

export interface StripeAccountLinkResp {
  url: string;
  accountId: string;
}

export interface BindStripeTestAccountResp {
  user: User;
  account: StripeAccountStatus;
}

export interface WithdrawCoinsResp {
  user: User;
  transaction: CoinTransaction;
  withdrawal: {
    id: string;
    amount: number;
    fee: number;
    netCoins: number;
    currency: string;
    status: string;
    stripeTransferId?: string;
    createdAt: string;
    updatedAt: string;
  };
  transferId: string;
  amount: number;
  fee: number;
  netCoins: number;
  currency: string;
  amountMinor: number;
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

export function useConfirmTopupCoins() {
  const qc = useQueryClient();
  return useMutation<ConfirmTopupResp, Error, { sessionId: string }>({
    mutationFn: async ({ sessionId }) => {
      const { data } = await http.post<ConfirmTopupResp>('/users/me/coins/topup/confirm', {
        sessionId,
      });
      return data;
    },
    onSuccess: (resp) => {
      qc.setQueryData(['me'], resp.user);
      useAuthStore.getState().setUser(resp.user);
      void qc.invalidateQueries({ queryKey: coinTransactionsKey });
    },
  });
}

export function useStripeAccountStatus() {
  const isAuthed = useIsAuthed();
  return useQuery<StripeAccountStatus, Error>({
    queryKey: ['coins', 'stripe-account'],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<StripeAccountStatus>('/users/me/coins/stripe/account', {
        signal,
      });
      return data;
    },
    enabled: isAuthed,
    staleTime: 20_000,
  });
}

export function useStripeAccountLink() {
  const qc = useQueryClient();
  return useMutation<StripeAccountLinkResp, Error, void>({
    mutationFn: async () => {
      const { data } = await http.post<StripeAccountLinkResp>(
        '/users/me/coins/stripe/account-link',
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['coins', 'stripe-account'] });
    },
  });
}

export function useBindStripeTestAccount() {
  const qc = useQueryClient();
  return useMutation<BindStripeTestAccountResp, Error, { accountId: string }>({
    mutationFn: async ({ accountId }) => {
      const { data } = await http.post<BindStripeTestAccountResp>(
        '/users/me/coins/stripe/test-account',
        { accountId },
      );
      return data;
    },
    onSuccess: (resp) => {
      qc.setQueryData(['me'], resp.user);
      qc.setQueryData(['coins', 'stripe-account'], resp.account);
      useAuthStore.getState().setUser(resp.user);
    },
  });
}

export function useWithdrawCoins() {
  const qc = useQueryClient();
  return useMutation<WithdrawCoinsResp, Error, { amount: number }>({
    mutationFn: async ({ amount }) => {
      const { data } = await http.post<WithdrawCoinsResp>('/users/me/coins/withdrawals', {
        amount,
      });
      return data;
    },
    onSuccess: (resp) => {
      qc.setQueryData(['me'], resp.user);
      useAuthStore.getState().setUser(resp.user);
      void qc.invalidateQueries({ queryKey: coinTransactionsKey });
      void qc.invalidateQueries({ queryKey: ['coins', 'stripe-account'] });
    },
  });
}
