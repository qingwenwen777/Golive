import type { CSSProperties } from 'react';
import { Gift } from 'lucide-react';
import { cn } from '@/lib/cn';
import { builtInGiftKey, GIFT_ART, type GiftRef } from './giftArt';

interface GiftArtProps {
  gift: GiftRef;
  size?: number;
  className?: string;
}

// GiftArt draws a gift the same way on every platform, instead of leaving it
// to each system's emoji font. Gifts outside the built-in set show their own
// emoji, or a generic gift box.
export function GiftArt({ gift, size = 40, className }: GiftArtProps) {
  const key = builtInGiftKey(gift);
  const art = key ? GIFT_ART[key] : undefined;
  const box: CSSProperties = { width: size, height: size };

  if (!art) {
    return (
      <span
        className={cn('gl-gift-art is-plain', className)}
        style={{ ...box, fontSize: Math.round(size * 0.66) }}
        aria-hidden="true"
      >
        {gift.icon || <Gift size={Math.round(size * 0.55)} />}
      </span>
    );
  }

  const { Icon } = art;
  const style = {
    ...box,
    color: art.ink,
    '--gift-from': art.from,
    '--gift-to': art.to,
  } as CSSProperties;
  return (
    <span className={cn('gl-gift-art', className)} style={style} aria-hidden="true">
      <Icon size={Math.round(size * 0.56)} strokeWidth={2} />
    </span>
  );
}
