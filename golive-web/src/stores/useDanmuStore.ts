import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type DanmuDensity = 'low' | 'med' | 'heavy';
export type DanmuFontSize = 'sm' | 'md' | 'lg';

interface DanmuState {
  on: boolean;
  opacity: number;
  fontSize: DanmuFontSize;
  density: DanmuDensity;
  toggle: () => void;
  setOn: (v: boolean) => void;
  setOpacity: (v: number) => void;
  setFontSize: (v: DanmuFontSize) => void;
  setDensity: (v: DanmuDensity) => void;
}

export const useDanmuStore = create<DanmuState>()(
  persist(
    (set) => ({
      on: true,
      opacity: 1,
      fontSize: 'md',
      density: 'med',
      toggle: () => set((s) => ({ on: !s.on })),
      setOn: (on) => set({ on }),
      setOpacity: (opacity) => set({ opacity }),
      setFontSize: (fontSize) => set({ fontSize }),
      setDensity: (density) => set({ density }),
    }),
    { name: 'golive-danmu' },
  ),
);

export const DENSITY_CAP: Record<DanmuDensity, number> = { low: 10, med: 22, heavy: 40 };
export const DENSITY_RATE_MS: Record<DanmuDensity, number> = { low: 2600, med: 1300, heavy: 650 };
export const FONT_SIZE_PX: Record<DanmuFontSize, number> = { sm: 18, md: 22, lg: 28 };
