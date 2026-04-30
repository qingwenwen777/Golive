import { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';

export interface FlyingGift {
  id: string;
  icon: string;
  label: string;
}

export interface FlyingGiftLayerProps {
  items: FlyingGift[];
  onDone: (id: string) => void;
}

export function FlyingGiftLayer({ items, onDone }: FlyingGiftLayerProps) {
  const [mounted, setMounted] = useState(false);
  useEffect(() => {
    setMounted(true);
  }, []);
  if (!mounted) return null;

  return createPortal(
    <div className="pointer-events-none fixed inset-0 z-[9999] overflow-hidden">
      {items.map((g) => (
        <FlyingGiftItem key={g.id} item={g} onDone={() => onDone(g.id)} />
      ))}
    </div>,
    document.body,
  );
}

function FlyingGiftItem({ item, onDone }: { item: FlyingGift; onDone: () => void }) {
  useEffect(() => {
    const t = window.setTimeout(onDone, 3100);
    return () => window.clearTimeout(t);
  }, [onDone]);

  return (
    <div
      className="absolute bottom-6 right-6 flex flex-col items-center gap-1 text-center"
      style={{ animation: 'gl-gift-fly 3s cubic-bezier(0.2, 0.7, 0.3, 1) forwards' }}
    >
      <span className="text-6xl drop-shadow-lg" aria-hidden>{item.icon}</span>
      <span className="rounded-full bg-black/60 px-3 py-1 text-xs font-medium text-white">
        {item.label}
      </span>
    </div>
  );
}
