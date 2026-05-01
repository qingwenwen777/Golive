import { useEffect, useRef, useState } from 'react';
import {
  useDanmuStore,
  DENSITY_CAP,
  FONT_SIZE_PX,
} from '@/stores/useDanmuStore';
import { DANMU_COLORS, randomPick } from '@/constants/chat';
import type { Bullet } from '@/stores/useRealtimeStore';

interface RenderBullet {
  id: string;
  text: string;
  color: string;
  row: number;
  dur: number;
  size: number;
}

const ROWS = 7;
const TOP_OFFSET_PCT = 8;
const ACTIVE_HEIGHT_PCT = 68;

export interface DanmuLayerProps {
  bullets: Bullet[];
  onBulletEnd?: (id: string) => void;
}

export function DanmuLayer({ bullets, onBulletEnd }: DanmuLayerProps) {
  const on = useDanmuStore((s) => s.on);
  const opacity = useDanmuStore((s) => s.opacity);
  const density = useDanmuStore((s) => s.density);
  const fontSize = useDanmuStore((s) => s.fontSize);

  const [rendered, setRendered] = useState<RenderBullet[]>([]);
  const seenRef = useRef<Set<string>>(new Set());
  const occupiedRef = useRef<number[]>(Array(ROWS).fill(0));

  useEffect(() => {
    if (!on) return;
    const cap = DENSITY_CAP[density];
    const size = FONT_SIZE_PX[fontSize];
    const seen = seenRef.current;
    const occ = occupiedRef.current;

    const toAdd: RenderBullet[] = [];
    for (const b of bullets) {
      if (seen.has(b.id)) continue;
      seen.add(b.id);

      const now = performance.now();
      let row = -1;
      for (let i = 0; i < ROWS; i++) {
        if (now >= occ[i]!) {
          row = i;
          break;
        }
      }
      if (row === -1) continue;

      const dur = 5 + Math.random() * 2;
      const color = b.color ?? randomPick(DANMU_COLORS);
      const approxWidth = b.text.length * size * 0.6;
      const screenW = window.innerWidth || 1280;
      const tailMs = (approxWidth / (screenW + approxWidth)) * dur * 1000;
      occ[row] = now + tailMs + 100;

      toAdd.push({ id: b.id, text: b.text, color, row, dur, size });
    }
    if (toAdd.length === 0) return;
    setRendered((prev) => {
      const next = [...prev, ...toAdd];
      return next.length > cap ? next.slice(next.length - cap) : next;
    });
  }, [bullets, on, density, fontSize]);

  const handleEnd = (id: string) => {
    setRendered((prev) => prev.filter((b) => b.id !== id));
    onBulletEnd?.(id);
  };

  return (
    <div
      className="gl-danmu-layer"
      style={{ opacity: on ? opacity : 0, display: on ? 'block' : 'none' }}
      aria-hidden="true"
    >
      {rendered.map((b) => {
        const rowHeightPct = ACTIVE_HEIGHT_PCT / ROWS;
        const top = `${TOP_OFFSET_PCT + b.row * rowHeightPct}%`;
        return (
          <span
            key={b.id}
            className="gl-bullet"
            style={{
              top,
              fontSize: b.size,
              color: b.color,
              animationDuration: `${b.dur}s`,
            }}
            onAnimationEnd={() => handleEnd(b.id)}
          >
            {b.text}
          </span>
        );
      })}
    </div>
  );
}
