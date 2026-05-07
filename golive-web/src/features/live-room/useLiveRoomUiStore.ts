import { create } from 'zustand';
import type { ReportTargetDraft } from '@/features/reporting/ReportDialog';
import type { FlyingGift } from '@/features/live-room/FlyingGiftLayer';
import type { ChatModerationTarget } from '@/features/live-room/Chat';

interface LiveRoomUiState {
  flying: FlyingGift[];
  moderationTarget: ChatModerationTarget | null;
  reportTarget: ReportTargetDraft | null;
  pushFlyingGift: (gift: FlyingGift) => void;
  removeFlyingGift: (id: string) => void;
  openModeration: (target: ChatModerationTarget) => void;
  closeModeration: () => void;
  openReport: (target: ReportTargetDraft) => void;
  closeReport: () => void;
  resetRoomUi: () => void;
}

export const useLiveRoomUiStore = create<LiveRoomUiState>((set) => ({
  flying: [],
  moderationTarget: null,
  reportTarget: null,
  pushFlyingGift: (gift) =>
    set((state) => ({
      flying: [...state.flying, gift].slice(-8),
    })),
  removeFlyingGift: (id) =>
    set((state) => ({
      flying: state.flying.filter((gift) => gift.id !== id),
    })),
  openModeration: (target) => set({ moderationTarget: target }),
  closeModeration: () => set({ moderationTarget: null }),
  openReport: (target) => set({ reportTarget: target }),
  closeReport: () => set({ reportTarget: null }),
  resetRoomUi: () => set({ flying: [], moderationTarget: null, reportTarget: null }),
}));
