import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AxiosError } from 'axios';
import { http } from '@/lib/axios';
import type {
  Gift,
  GiftOrder,
  GiftSendPayload,
  SuperChatOrder,
  SuperChatPayload,
} from '@/types/gift';

export function newRequestId(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID();
  }
  return `req-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

export function useGifts() {
  return useQuery<Gift[], Error>({
    queryKey: ['gifts'],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<Gift[]>('/gifts', { signal });
      return data;
    },
    staleTime: 5 * 60_000,
  });
}

export interface GiftError {
  reason: 'insufficient_coin' | 'network' | 'unknown';
  message: string;
}

function toGiftError(err: unknown): GiftError {
  if (err instanceof AxiosError) {
    const data = err.response?.data as { reason?: string; message?: string } | undefined;
    if (data?.reason === 'insufficient_coin') {
      return { reason: 'insufficient_coin', message: data.message ?? 'Insufficient coins' };
    }
    if (!err.response) return { reason: 'network', message: 'Network error' };
    return { reason: 'unknown', message: data?.message ?? err.message };
  }
  return { reason: 'unknown', message: String(err) };
}

export function useSendGift() {
  const qc = useQueryClient();
  return useMutation<
    GiftOrder,
    GiftError,
    Omit<GiftSendPayload, 'requestId'> & { requestId?: string }
  >({
    mutationFn: async (input) => {
      const requestId = input.requestId ?? newRequestId();
      const body: GiftSendPayload = {
        roomId: input.roomId,
        giftId: input.giftId,
        count: input.count,
        requestId,
      };
      try {
        const { data } = await http.post<GiftOrder>('/gifts/send', body, {
          headers: { 'X-Request-Id': requestId },
        });
        return data;
      } catch (err) {
        throw toGiftError(err);
      }
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['me'] });
    },
  });
}

export function useSendSuperChat() {
  const qc = useQueryClient();
  return useMutation<
    SuperChatOrder,
    GiftError,
    Omit<SuperChatPayload, 'requestId'> & { requestId?: string }
  >({
    mutationFn: async (input) => {
      const requestId = input.requestId ?? newRequestId();
      const body: SuperChatPayload = {
        roomId: input.roomId,
        amount: input.amount,
        text: input.text,
        displayName: input.displayName,
        requestId,
      };
      try {
        const { data } = await http.post<SuperChatOrder>('/super-chats', body, {
          headers: { 'X-Request-Id': requestId },
        });
        return data;
      } catch (err) {
        throw toGiftError(err);
      }
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['me'] });
    },
  });
}
