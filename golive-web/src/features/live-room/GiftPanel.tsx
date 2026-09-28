import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { toast } from 'sonner';
import { Coins, LockKeyhole, Sparkles } from 'lucide-react';
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
import { localizedGiftName } from '@/lib/gift';
import { normalizeLevelInfo } from '@/lib/userLevel';
import { UserLevelBadge } from '@/components/UserLevelBadge';
import type { Gift } from '@/types/gift';

type PriceTab = { id: string; label: string; labelKey?: string; min: number; max: number };

const ALL_PRICE_TAB: PriceTab = {
  id: 'all',
  label: 'All',
  labelKey: 'liveRoom.giftPanel.tabs.all',
  min: 0,
  max: Infinity,
};

const PRICE_TABS: PriceTab[] = [
  ALL_PRICE_TAB,
  { id: 't50', label: '<=50', min: 0, max: 50 },
  { id: 't100', label: '<=100', min: 0, max: 100 },
  { id: 't500', label: '<=500', min: 0, max: 500 },
  { id: 't1000', label: '<=1000', min: 0, max: 1000 },
];

const COUNT_PRESETS = [1, 10, 52, 99, 520, 1314] as const;

export interface GiftPanelProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  roomId: string;
  onSent?: (payload: { gift: Gift; count: number; requestId: string }) => void;
}

export function GiftPanel({ open, onOpenChange, roomId, onSent }: GiftPanelProps) {
  const { t, i18n } = useTranslation('pages');
  const isMobile = useMediaQuery('(max-width: 640px)');
  const navigate = useNavigate();
  const { data: gifts, isPending } = useGifts();
  const me = useMe();
  const balance = me.data?.coinBalance ?? 0;
  const levelInfo = normalizeLevelInfo(me.data?.levelInfo);
  const userLevel = levelInfo.level;

  const [tab, setTab] = useState(ALL_PRICE_TAB.id);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [count, setCount] = useState<number>(1);
  const sendGift = useSendGift();

  const filtered = useMemo(() => {
    if (!gifts) return [];
    const tabDef = PRICE_TABS.find((t) => t.id === tab) ?? ALL_PRICE_TAB;
    return gifts.filter((g) => g.priceCoin <= tabDef.max);
  }, [gifts, tab]);

  const selected = gifts?.find((g) => g.id === selectedId) ?? null;
  const total = selected ? selected.priceCoin * count : 0;
  const insufficient = selected ? total > balance : false;
  const requiredLevel = selected?.unlockLevel ?? 1;
  const locked = selected ? requiredLevel > userLevel : false;
  const language = i18n.resolvedLanguage ?? i18n.language;
  const selectedName = selected ? localizedGiftName(selected, language, t) : '';

  const handleSend = () => {
    if (!selected) return;
    if (locked) {
      onOpenChange(false);
      navigate('/coins?focus=recharge');
      return;
    }
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
            toast.error(
              t('liveRoom.giftPanel.insufficientCoins', { defaultValue: 'Insufficient coins' }),
            );
            return;
          }
          toast.success(
            t('liveRoom.giftPanel.sent', {
              name: selectedName,
              count,
              defaultValue: 'Sent {{name}} x{{count}}',
            }),
          );
          onSent?.({ gift: selected, count, requestId });
        },
        onError: (err) => {
          if (err.reason === 'insufficient_coin') {
            toast.error(
              t('liveRoom.giftPanel.insufficientCoins', { defaultValue: 'Insufficient coins' }),
            );
          } else if (err.reason === 'gift_level_locked') {
            toast.error(
              t('liveRoom.giftPanel.unlockToast', {
                level: err.requiredLevel ?? requiredLevel,
                defaultValue: 'Unlocks at Lv.{{level}}',
              }),
            );
          } else {
            toast.error(
              err.message ||
                t('liveRoom.giftPanel.failed', { defaultValue: 'Failed to send gift' }),
            );
          }
        },
      },
    );
  };

  const side = isMobile ? 'bottom' : 'right';

  return (
    <Sheet open={open} onOpenChange={onOpenChange} modal={false}>
      <SheetContent
        side={side}
        showOverlay={false}
        className={cn('gl-gift-sheet flex flex-col p-0', isMobile && 'is-mobile')}
      >
        <SheetHeader className="gl-gift-sheet-head">
          <SheetTitle>{t('liveRoom.giftPanel.title', { defaultValue: 'Gifts' })}</SheetTitle>
          <SheetDescription className="flex flex-wrap items-center gap-2 text-xs text-text-secondary">
            <span className="inline-flex items-center gap-1">
              <Coins size={14} />
              <span>
                {t('liveRoom.giftPanel.balance', {
                  coins: balance.toLocaleString(language),
                  defaultValue: '{{coins}} coins',
                })}
              </span>
            </span>
            <UserLevelBadge levelInfo={levelInfo} size="compact" />
          </SheetDescription>
        </SheetHeader>

        <div className="gl-gift-tabs">
          {PRICE_TABS.map((tabDef) => (
            <button
              key={tabDef.id}
              onClick={() => setTab(tabDef.id)}
              className={cn(
                'gl-gift-tab',
                tab === tabDef.id ? 'is-active' : 'text-text-secondary hover:text-text-primary',
              )}
            >
              {tabDef.labelKey ? t(tabDef.labelKey, { defaultValue: tabDef.label }) : tabDef.label}
            </button>
          ))}
        </div>

        <div className="gl-gift-scroll">
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
                const giftRequiredLevel = g.unlockLevel ?? 1;
                const giftLocked = giftRequiredLevel > userLevel;
                const giftName = localizedGiftName(g, language, t);
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
                      giftRequiredLevel > 1 && 'is-level-gift',
                      giftLocked && 'is-locked',
                      active ? 'is-active' : 'hover:bg-bg-hover/70 bg-bg-hover',
                    )}
                  >
                    {giftLocked && (
                      <span className="gl-gift-lock-mark" aria-hidden>
                        <LockKeyhole size={12} />
                      </span>
                    )}
                    <span className="gl-gift-icon" aria-hidden>
                      {g.icon}
                    </span>
                    <span className="gl-gift-name">{giftName}</span>
                    <span className="gl-gift-price">
                      <Coins size={10} /> {g.priceCoin.toLocaleString(language)}
                    </span>
                    {giftRequiredLevel > 1 && (
                      <span className="gl-gift-level-chip">
                        <Sparkles size={10} />
                        {t('liveRoom.giftPanel.levelChip', {
                          level: giftRequiredLevel,
                          defaultValue: 'Lv.{{level}}',
                        })}
                      </span>
                    )}
                  </button>
                );
              })}
            </div>
          )}
        </div>

        {selected && (
          <div className="gl-gift-sendbar">
            <div className="gl-gift-counts">
              {COUNT_PRESETS.map((n) => (
                <button
                  key={n}
                  onClick={() => setCount(n)}
                  className={cn('gl-gift-count', count === n && 'is-active')}
                >
                  x{n}
                </button>
              ))}
            </div>
            <div className="gl-gift-sendrow">
              <span className="gl-gift-selected-icon" aria-hidden>
                {selected.icon}
              </span>
              <div className="gl-gift-selected-copy">
                <div className="gl-gift-selected-name">{selectedName}</div>
                <div
                  className={cn(
                    'gl-gift-selected-meta',
                    locked || insufficient
                      ? 'text-red-600 dark:text-red-400'
                      : 'text-text-secondary',
                  )}
                >
                  {locked
                    ? t('liveRoom.giftPanel.unlockHint', {
                        level: requiredLevel,
                        defaultValue: 'Unlocks at Lv.{{level}}. Recharge to level up.',
                      })
                    : insufficient
                      ? t('liveRoom.giftPanel.needCoins', {
                          coins: (total - balance).toLocaleString(language),
                          defaultValue: 'Need {{coins}} more coins',
                        })
                      : t('liveRoom.giftPanel.totalCoins', {
                          coins: total.toLocaleString(language),
                          defaultValue: 'Total {{coins}} coins',
                        })}
                </div>
              </div>
              <button
                disabled={sendGift.isPending}
                onClick={handleSend}
                className={cn(
                  'gl-gift-send-btn',
                  locked || insufficient ? 'is-muted' : 'is-ready',
                  sendGift.isPending && 'opacity-60',
                )}
              >
                {locked || insufficient
                  ? t('liveRoom.giftPanel.topUp', { defaultValue: 'Top up' })
                  : sendGift.isPending
                    ? t('liveRoom.giftPanel.sending', { defaultValue: 'Sending...' })
                    : t('liveRoom.giftPanel.send', { defaultValue: 'Send' })}
              </button>
            </div>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}
