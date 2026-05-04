import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface PlayerPreferenceState {
  muted: boolean;
  volume: number;
  setAudio: (patch: { muted?: boolean; volume?: number }) => void;
}

function clampVolume(value: number): number {
  if (!Number.isFinite(value)) return 1;
  return Math.min(1, Math.max(0, value));
}

export const usePlayerPreferenceStore = create<PlayerPreferenceState>()(
  persist(
    (set) => ({
      muted: false,
      volume: 1,
      setAudio: (patch) =>
        set((state) => ({
          muted: patch.muted ?? state.muted,
          volume: patch.volume === undefined ? state.volume : clampVolume(patch.volume),
        })),
    }),
    { name: 'golive-player-preferences' },
  ),
);
