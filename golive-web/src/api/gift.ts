import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { AxiosError } from 'axios';
import { http } from '@/lib/axios';
import type {
  Gift,
  GiftOrder,
  GiftSendPayload,
  FanBadge,
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

export function fanBadgesQueryKey(userId?: string) {
  return ['fan-badges', 'me', userId ?? 'anonymous'] as const;
}

export function useFanBadges(enabled = true, userId?: string) {
  return useQuery<FanBadge[], Error>({
    queryKey: fanBadgesQueryKey(userId),
    queryFn: async ({ signal }) => {
      const { data } = await http.get<FanBadge[]>('/gifts/fan-badges/me', { signal });
      return data;
    },
    enabled: enabled && !!userId,
    staleTime: 60_000,
  });
}

export interface GiftError {
  reason: 'insufficient_coin' | 'gift_level_locked' | 'blocked_word' | 'network' | 'unknown';
  message: string;
  requiredLevel?: number;
  userLevel?: number;
}

function toGiftError(err: unknown): GiftError {
  if (err instanceof AxiosError) {
    const data = err.response?.data as { reason?: string; message?: string } | undefined;
    if (data?.reason === 'insufficient_coin') {
      return { reason: 'insufficient_coin', message: data.message ?? 'Insufficient coins' };
    }
    if (data?.reason === 'gift_level_locked') {
      const locked = data as { message?: string; requiredLevel?: number; userLevel?: number };
      return {
        reason: 'gift_level_locked',
        message: locked.message ?? 'Gift unlock level not reached',
        requiredLevel: locked.requiredLevel,
        userLevel: locked.userLevel,
      };
    }
    if (data?.reason === 'blocked_word') {
      return { reason: 'blocked_word', message: data.message ?? 'Content contains blocked word' };
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
      void qc.invalidateQueries({ queryKey: ['fan-badges', 'me'] });
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
      void qc.invalidateQueries({ queryKey: ['fan-badges', 'me'] });
    },
  });
}
