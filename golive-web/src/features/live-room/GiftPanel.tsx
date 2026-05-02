import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { toast } from 'sonner';
import { Coins } from 'lucide-react';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from '@/components/ui/sheet';
import { useMe } from '@/api/auth';
import { useGifts, useSendGift, newRequestId } from '@/api/gift';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { cn } from '@/lib/cn';
import type { Gift } from '@/types/gift';

const PRICE_TABS = [
  { id: 'all', label: 'All', min: 0, max: Infinity },
  { id: 't50', label: '≤50', min: 0, max: 50 },
  { id: 't100', label: '≤100', min: 0, max: 100 },
  { id: 't500', label: '≤500', min: 0, max: 500 },
  { id: 't1000', label: '≤1000', min: 0, max: 1000 },
] as const;

const COUNT_PRESETS = [1, 10, 52, 99, 520, 1314] as const;

export interface GiftPanelProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  roomId: string;
  onSent?: (payload: { gift: Gift; count: number; requestId: string }) => void;
}

export function GiftPanel({ open, onOpenChange, roomId, onSent }: GiftPanelProps) {
  const isMobile = useMediaQuery('(max-width: 640px)');
  const navigate = useNavigate();
  const { data: gifts, isPending } = useGifts();
  const me = useMe();
  const balance = me.data?.coinBalance ?? 0;

  const [tab, setTab] = useState<(typeof PRICE_TABS)[number]['id']>('all');
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [count, setCount] = useState<number>(1);
  const sendGift = useSendGift();

  const filtered = useMemo(() => {
    if (!gifts) return [];
    const tabDef = PRICE_TABS.find((t) => t.id === tab)!;
    return gifts.filter((g) => g.priceCoin <= tabDef.max);
  }, [gifts, tab]);

  const selected = gifts?.find((g) => g.id === selectedId) ?? null;
  const total = selected ? selected.priceCoin * count : 0;
  const insufficient = selected ? total > balance : false;

  const handleSend = () => {
    if (!selected) return;
    if (insufficient) {
      onOpenChange(false);
      navigate('/coins?focus=recharge');
      return;
    }
    const requestId = newRequestId();
    sendGift.mutate(
      { roomId, giftId: selected.id, count, requestId },
      {
        onSuccess: (order) => {
          if (order.status !== 'success') {
            toast.error('Insufficient coins');
            return;
          }
          toast.success(`Sent ${selected.name} ×${count}`);
          onSent?.({ gift: selected, count, requestId });
        },
        onError: (err) => {
          if (err.reason === 'insufficient_coin') {
            toast.error('Insufficient coins');
          } else {
            toast.error(err.message || 'Failed to send gift');
          }
        },
      },
    );
  };

  const side = isMobile ? 'bottom' : 'right';

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side={side}
        className={cn('flex flex-col p-0', isMobile ? 'h-[75vh]' : 'w-[420px] sm:max-w-[420px]')}
      >
        <SheetHeader className="border-b border-border px-4 py-3">
          <SheetTitle>Gifts</SheetTitle>
          <SheetDescription className="flex items-center gap-1 text-xs text-text-secondary">
            <Coins size={14} /> <span>{balance.toLocaleString()} coins</span>
          </SheetDescription>
        </SheetHeader>

        <div className="gl-gift-tabs">
          {PRICE_TABS.map((t) => (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              className={cn(
                'shrink-0 rounded-full px-3 py-1 text-sm',
                tab === t.id
                  ? 'bg-accent text-white'
                  : 'bg-bg-hover text-text-secondary hover:text-text-primary',
              )}
            >
              {t.label}
            </button>
          ))}
        </div>

        <div className="flex-1 overflow-y-auto px-4 pb-24">
          {isPending ? (
            <div className="gl-gift-grid">
              {Array.from({ length: 12 }).map((_, i) => (
                <div key={i} className="h-24 animate-pulse rounded-md bg-bg-hover" />
              ))}
            </div>
          ) : (
            <div className="gl-gift-grid">
              {filtered.map((g) => {
                const active = g.id === selectedId;
                return (
                  <button
                    key={g.id}
                    onClick={() => {
                      setSelectedId(g.id);
                      setCount(1);
                    }}
                    className={cn(
                      'gl-gift-card',
                      g.id === 'fan_light' && 'is-fan-light',
                      active ? 'is-active' : 'hover:bg-bg-hover/70 bg-bg-hover',
                    )}
                  >
                    <span className="text-3xl" aria-hidden>
                      {g.icon}
                    </span>
                    <span className="max-w-full truncate text-xs font-medium">{g.name}</span>
                    <span className="flex items-center gap-1 text-[11px] text-text-secondary">
                      <Coins size={10} /> {g.priceCoin}
                    </span>
                  </button>
                );
              })}
            </div>
          )}
        </div>

        {selected && (
          <div className="sticky bottom-0 left-0 right-0 border-t border-border bg-bg-primary p-3">
            <div className="mb-2 flex flex-wrap gap-1.5">
              {COUNT_PRESETS.map((n) => (
                <button
                  key={n}
                  onClick={() => setCount(n)}
                  className={cn(
                    'rounded-full px-2.5 py-1 text-xs',
                    count === n ? 'bg-accent text-white' : 'bg-bg-hover text-text-secondary',
                  )}
                >
                  ×{n}
                </button>
              ))}
            </div>
            <div className="flex items-center gap-3">
              <span className="text-2xl" aria-hidden>
                {selected.icon}
              </span>
              <div className="flex-1">
                <div className="text-sm font-medium">{selected.name}</div>
                <div
                  className={cn('text-xs', insufficient ? 'text-red-500' : 'text-text-secondary')}
                >
                  {insufficient
                    ? `Need ${total - balance} more coins`
                    : `Total ${total.toLocaleString()} coins`}
                </div>
              </div>
              <button
                disabled={sendGift.isPending}
                onClick={handleSend}
                className={cn(
                  'rounded-full px-5 py-2 text-sm font-semibold transition',
                  insufficient
                    ? 'bg-bg-hover text-text-secondary'
                    : 'bg-accent text-white hover:bg-accent/90',
                  sendGift.isPending && 'opacity-60',
                )}
              >
                {insufficient ? 'Top up' : sendGift.isPending ? 'Sending…' : 'Send'}
              </button>
            </div>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}
