import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  ChevronDown,
  Clock3,
  Coins,
  Gift,
  PartyPopper,
  Sparkles,
  Ticket,
  Users,
  XCircle,
} from 'lucide-react';
import { toast } from 'sonner';
import {
  useCancelLuckyBag,
  useJoinLuckyBag,
  useLatestLuckyBag,
  useOpenLuckyBag,
  type LuckyBagErrorReason,
} from '@/api/luckyBag';
import { useMe } from '@/api/auth';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useIsAuthed } from '@/stores/useAuthStore';
import type { LuckyBagAmountMode, LuckyBagEligibility } from '@/types/luckyBag';
import { cn } from '@/lib/cn';

const DEFAULT_TOTAL = 1000;
const DEFAULT_COUNT = 10;
const DEFAULT_DURATION = 60;
const MIN_DURATION = 30;
const MAX_DURATION = 600;
const MAX_COUNT = 500;
const MAX_MESSAGE = 60;

const ELIGIBILITY_OPTIONS: LuckyBagEligibility[] = ['all', 'followers', 'fans', 'fans_level'];

export function LuckyBagPanel({ roomId, ownsStream }: { roomId: string; ownsStream: boolean }) {
  const { t } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const latest = useLatestLuckyBag(roomId, Boolean(roomId));
  const openBag = useOpenLuckyBag(roomId);
  const joinBag = useJoinLuckyBag(roomId);
  const cancelBag = useCancelLuckyBag(roomId);
  const me = useMe();

  const [total, setTotal] = useState(DEFAULT_TOTAL);
  const [count, setCount] = useState(DEFAULT_COUNT);
  const [amountMode, setAmountMode] = useState<LuckyBagAmountMode>('random');
  const [eligibility, setEligibility] = useState<LuckyBagEligibility>('all');
  const [minFanLevel, setMinFanLevel] = useState(1);
  const [duration, setDuration] = useState(DEFAULT_DURATION);
  const [message, setMessage] = useState('');
  const [now, setNow] = useState(() => Date.now());

  const bag = latest.data?.bag ?? null;
  const myEntry = latest.data?.myEntry;
  const participantCount = latest.data?.participantCount ?? 0;
  const balance = me.data?.coinBalance ?? 0;

  useEffect(() => {
    if (!bag || bag.status !== 'open') return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [bag]);

  const remainingMs = bag ? new Date(bag.closeAt).getTime() - now : 0;
  const accepting = Boolean(bag && bag.status === 'open' && remainingMs > 0);
  const effectiveStatus = bag?.status === 'open' && remainingMs <= 0 ? 'drawing' : bag?.status;
  const canOpen = ownsStream && (!bag || bag.status === 'drawn' || bag.status === 'cancelled');

  const perPacket = useMemo(() => {
    if (count <= 0) return 0;
    return Math.floor(total / count);
  }, [count, total]);

  const handleOpen = () => {
    const safeTotal = Math.max(1, Math.floor(total));
    const safeCount = Math.min(MAX_COUNT, Math.max(1, Math.floor(count)));
    if (safeTotal < safeCount) {
      toast.error(
        t('luckyBag.errorTotalTooLow', {
          defaultValue: 'The total must be at least one coin per packet.',
        }),
      );
      return;
    }
    if (amountMode === 'fixed' && safeTotal % safeCount !== 0) {
      toast.error(
        t('luckyBag.errorNotDivisible', {
          defaultValue: 'For fixed packets the total must divide evenly by the count.',
        }),
      );
      return;
    }
    if (safeTotal > balance) {
      toast.error(t('luckyBag.errorInsufficient', { defaultValue: 'Insufficient coin balance.' }));
      return;
    }
    openBag.mutate(
      {
        totalCoin: safeTotal,
        count: safeCount,
        amountMode,
        eligibility,
        minFanLevel: eligibility === 'fans_level' ? Math.max(1, Math.floor(minFanLevel)) : 0,
        durationSeconds: clampDuration(duration),
        message: message.trim() || undefined,
      },
      {
        onSuccess: () => {
          setMessage('');
          toast.success(
            t('luckyBag.openSuccess', {
              defaultValue: 'Lucky bag started. It draws automatically when the timer ends.',
            }),
          );
        },
        onError: (err) => toast.error(bagErrorText(err.reason, err.message, t)),
      },
    );
  };

  const handleJoin = () => {
    if (!bag) return;
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (!accepting) {
      toast.error(t('luckyBag.errorClosed', { defaultValue: 'The lucky bag is closed.' }));
      return;
    }
    joinBag.mutate(
      { bagId: bag.id },
      {
        onSuccess: () =>
          toast.success(t('luckyBag.joinSuccess', { defaultValue: 'You joined the lucky bag.' })),
        onError: (err) => toast.error(bagErrorText(err.reason, err.message, t)),
      },
    );
  };

  const handleCancel = () => {
    if (!bag) return;
    cancelBag.mutate(
      { bagId: bag.id },
      {
        onSuccess: () =>
          toast.success(
            t('luckyBag.cancelSuccess', {
              defaultValue: 'Lucky bag cancelled. Coins were refunded.',
            }),
          ),
        onError: (err) => toast.error(bagErrorText(err.reason, err.message, t)),
      },
    );
  };

  if (!bag && !ownsStream) return null;

  return (
    <section className="gl-bag-panel" aria-label={t('luckyBag.title', { defaultValue: 'Lucky bag' })}>
      <div className="gl-bag-head">
        <div className="gl-bag-icon" aria-hidden="true">
          <Gift size={17} />
        </div>
        <div>
          <h2>{t('luckyBag.title', { defaultValue: 'Lucky bag' })}</h2>
          <p>
            {bag?.message ||
              t('luckyBag.emptyHint', {
                defaultValue: 'Pack coins into red packets for your viewers.',
              })}
          </p>
        </div>
        {bag && (
          <span className={cn('gl-bag-status', `is-${effectiveStatus}`)}>
            {statusText(effectiveStatus, t)}
          </span>
        )}
      </div>

      {!bag || canOpen ? (
        <OpenLuckyBagForm
          total={total}
          count={count}
          amountMode={amountMode}
          eligibility={eligibility}
          minFanLevel={minFanLevel}
          duration={duration}
          message={message}
          perPacket={perPacket}
          pending={openBag.isPending}
          reopen={Boolean(bag)}
          onTotalChange={setTotal}
          onCountChange={setCount}
          onAmountModeChange={setAmountMode}
          onEligibilityChange={setEligibility}
          onMinFanLevelChange={setMinFanLevel}
          onDurationChange={setDuration}
          onMessageChange={setMessage}
          onSubmit={handleOpen}
        />
      ) : (
        <>
          <div className="gl-bag-meta">
            <span>
              <Coins size={14} />
              {t('luckyBag.totalMeta', {
                total: bag.totalCoin.toLocaleString(),
                defaultValue: '{{total}} coins',
              })}
            </span>
            <span>
              <Ticket size={14} />
              {t('luckyBag.countMeta', {
                count: bag.count,
                formattedCount: bag.count.toLocaleString(),
                defaultValue: '{{formattedCount}} packets',
              })}
            </span>
            <span>
              <Clock3 size={14} />
              {accepting
                ? t('luckyBag.closesIn', {
                    seconds: Math.ceil(remainingMs / 1000),
                    defaultValue: '{{seconds}}s left',
                  })
                : t('luckyBag.drawing', { defaultValue: 'Drawing...' })}
            </span>
          </div>

          <div className="gl-bag-info-row">
            <span className="gl-bag-eligibility">
              <Users size={13} />
              {eligibilityText(bag.eligibility, bag.minFanLevel, t)}
            </span>
            <span className="gl-bag-participants">
              {t('luckyBag.participants', {
                count: participantCount,
                formattedCount: participantCount.toLocaleString(),
                defaultValue: '{{formattedCount}} joined',
              })}
            </span>
          </div>

          {!ownsStream && (
            <ViewerBagAction
              joined={Boolean(myEntry)}
              entryStatus={myEntry?.status}
              payout={myEntry?.payout ?? 0}
              accepting={accepting}
              pending={joinBag.isPending}
              onJoin={handleJoin}
            />
          )}

          {ownsStream && myEntryless(bag.status) && (
            <div className="gl-bag-host-actions">
              <button
                type="button"
                className="is-cancel"
                onClick={handleCancel}
                disabled={cancelBag.isPending}
              >
                <XCircle size={14} />
                <span>
                  {cancelBag.isPending
                    ? t('luckyBag.cancelling', { defaultValue: 'Cancelling...' })
                    : t('luckyBag.cancel', { defaultValue: 'Cancel & refund' })}
                </span>
              </button>
            </div>
          )}
        </>
      )}
    </section>
  );
}

function myEntryless(status: string): boolean {
  return status === 'open';
}

function ViewerBagAction({
  joined,
  entryStatus,
  payout,
  accepting,
  pending,
  onJoin,
}: {
  joined: boolean;
  entryStatus?: string;
  payout: number;
  accepting: boolean;
  pending: boolean;
  onJoin: () => void;
}) {
  const { t } = useTranslation('pages');

  if (entryStatus === 'won') {
    return (
      <div className="gl-bag-result is-won">
        <PartyPopper size={18} />
        <span>
          {t('luckyBag.youWon', {
            payout: payout.toLocaleString(),
            defaultValue: 'You won {{payout}} coins!',
          })}
        </span>
      </div>
    );
  }
  if (entryStatus === 'missed') {
    return (
      <div className="gl-bag-result is-missed">
        <Sparkles size={16} />
        <span>{t('luckyBag.youMissed', { defaultValue: 'No luck this time. Try the next one!' })}</span>
      </div>
    );
  }
  if (joined) {
    return (
      <div className="gl-bag-result is-joined">
        <Ticket size={16} />
        <span>{t('luckyBag.joinedWaiting', { defaultValue: 'You are in. Stay for the draw!' })}</span>
      </div>
    );
  }
  return (
    <button
      type="button"
      className="gl-bag-join"
      onClick={onJoin}
      disabled={!accepting || pending}
    >
      <Gift size={16} />
      <span>
        {pending
          ? t('luckyBag.joining', { defaultValue: 'Joining...' })
          : t('luckyBag.join', { defaultValue: 'Join the draw' })}
      </span>
    </button>
  );
}

function OpenLuckyBagForm({
  total,
  count,
  amountMode,
  eligibility,
  minFanLevel,
  duration,
  message,
  perPacket,
  pending,
  reopen,
  onTotalChange,
  onCountChange,
  onAmountModeChange,
  onEligibilityChange,
  onMinFanLevelChange,
  onDurationChange,
  onMessageChange,
  onSubmit,
}: {
  total: number;
  count: number;
  amountMode: LuckyBagAmountMode;
  eligibility: LuckyBagEligibility;
  minFanLevel: number;
  duration: number;
  message: string;
  perPacket: number;
  pending: boolean;
  reopen: boolean;
  onTotalChange: (value: number) => void;
  onCountChange: (value: number) => void;
  onAmountModeChange: (value: LuckyBagAmountMode) => void;
  onEligibilityChange: (value: LuckyBagEligibility) => void;
  onMinFanLevelChange: (value: number) => void;
  onDurationChange: (value: number) => void;
  onMessageChange: (value: string) => void;
  onSubmit: () => void;
}) {
  const { t } = useTranslation('pages');
  return (
    <div className="gl-bag-open">
      <div className="gl-bag-mode" role="tablist" aria-label={t('luckyBag.amountMode', { defaultValue: 'Amount mode' })}>
        <button
          type="button"
          role="tab"
          aria-selected={amountMode === 'random'}
          className={cn(amountMode === 'random' && 'is-active')}
          onClick={() => onAmountModeChange('random')}
        >
          {t('luckyBag.modeRandom', { defaultValue: 'Lucky draw' })}
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={amountMode === 'fixed'}
          className={cn(amountMode === 'fixed' && 'is-active')}
          onClick={() => onAmountModeChange('fixed')}
        >
          {t('luckyBag.modeFixed', { defaultValue: 'Equal' })}
        </button>
      </div>

      <div className="gl-bag-fields">
        <label>
          <span>{t('luckyBag.totalLabel', { defaultValue: 'Total coins' })}</span>
          <input
            type="number"
            min={1}
            step={100}
            value={total}
            onChange={(event) => onTotalChange(Number(event.target.value))}
          />
        </label>
        <label>
          <span>{t('luckyBag.countLabel', { defaultValue: 'Packets' })}</span>
          <input
            type="number"
            min={1}
            max={MAX_COUNT}
            step={1}
            value={count}
            onChange={(event) => onCountChange(Number(event.target.value))}
          />
        </label>
      </div>

      <div className="gl-bag-hint">
        {amountMode === 'fixed'
          ? t('luckyBag.fixedHint', {
              perPacket: perPacket.toLocaleString(),
              defaultValue: '{{perPacket}} coins per packet',
            })
          : t('luckyBag.randomHint', { defaultValue: 'Random amount per packet' })}
      </div>

      <div className="gl-bag-fields">
        <label>
          <span>{t('luckyBag.eligibilityLabel', { defaultValue: 'Who can join' })}</span>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button type="button" className="gl-bag-select-trigger">
                <span>{eligibilityOptionLabel(eligibility, t)}</span>
                <ChevronDown size={16} aria-hidden="true" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" sideOffset={6} className="gl-bag-select-content">
              {ELIGIBILITY_OPTIONS.map((option) => (
                <DropdownMenuItem
                  key={option}
                  className={cn('gl-bag-select-item', eligibility === option && 'is-selected')}
                  onSelect={() => onEligibilityChange(option)}
                >
                  {eligibilityOptionLabel(option, t)}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </label>
        {eligibility === 'fans_level' ? (
          <label>
            <span>{t('luckyBag.minFanLevelLabel', { defaultValue: 'Min fan level' })}</span>
            <input
              type="number"
              min={1}
              max={99}
              step={1}
              value={minFanLevel}
              onChange={(event) => onMinFanLevelChange(Number(event.target.value))}
            />
          </label>
        ) : (
          <label>
            <span>{t('luckyBag.durationLabel', { defaultValue: 'Countdown (sec)' })}</span>
            <input
              type="number"
              min={MIN_DURATION}
              max={MAX_DURATION}
              step={30}
              value={duration}
              onChange={(event) => onDurationChange(Number(event.target.value))}
            />
          </label>
        )}
      </div>

      {eligibility === 'fans_level' && (
        <div className="gl-bag-fields">
          <label>
            <span>{t('luckyBag.durationLabel', { defaultValue: 'Countdown (sec)' })}</span>
            <input
              type="number"
              min={MIN_DURATION}
              max={MAX_DURATION}
              step={30}
              value={duration}
              onChange={(event) => onDurationChange(Number(event.target.value))}
            />
          </label>
        </div>
      )}

      <label className="gl-bag-message">
        <span>{t('luckyBag.messageLabel', { defaultValue: 'Blessing (optional)' })}</span>
        <input
          type="text"
          maxLength={MAX_MESSAGE}
          placeholder={t('luckyBag.messagePlaceholder', { defaultValue: 'Thanks for watching!' })}
          value={message}
          onChange={(event) => onMessageChange(event.target.value)}
        />
      </label>

      <button type="button" className="gl-bag-submit" onClick={onSubmit} disabled={pending}>
        <Gift size={15} />
        <span>
          {pending
            ? t('luckyBag.opening', { defaultValue: 'Sending...' })
            : reopen
              ? t('luckyBag.reopen', { defaultValue: 'Send another' })
              : t('luckyBag.open', { defaultValue: 'Send lucky bag' })}
        </span>
      </button>
    </div>
  );
}

function clampDuration(duration: number): number {
  const value = Math.floor(duration);
  if (!Number.isFinite(value) || value < MIN_DURATION) return MIN_DURATION;
  if (value > MAX_DURATION) return MAX_DURATION;
  return value;
}

function statusText(status: string | undefined, t: ReturnType<typeof useTranslation>['t']): string {
  if (status === 'open') return t('luckyBag.status.open', { defaultValue: 'Open' });
  if (status === 'drawing') return t('luckyBag.status.drawing', { defaultValue: 'Drawing' });
  if (status === 'drawn') return t('luckyBag.status.drawn', { defaultValue: 'Drawn' });
  if (status === 'cancelled') return t('luckyBag.status.cancelled', { defaultValue: 'Cancelled' });
  return t('luckyBag.title', { defaultValue: 'Lucky bag' });
}

function eligibilityOptionLabel(
  option: LuckyBagEligibility,
  t: ReturnType<typeof useTranslation>['t'],
): string {
  switch (option) {
    case 'followers':
      return t('luckyBag.eligibility.followers', { defaultValue: 'Followers' });
    case 'fans':
      return t('luckyBag.eligibility.fans', { defaultValue: 'Fan club members' });
    case 'fans_level':
      return t('luckyBag.eligibility.fansLevel', { defaultValue: 'Fan club by level' });
    default:
      return t('luckyBag.eligibility.all', { defaultValue: 'Everyone' });
  }
}

function eligibilityText(
  eligibility: LuckyBagEligibility,
  minFanLevel: number,
  t: ReturnType<typeof useTranslation>['t'],
): string {
  if (eligibility === 'fans_level') {
    return t('luckyBag.eligibilityFansLevelMeta', {
      level: minFanLevel,
      defaultValue: 'Fan club Lv.{{level}}+',
    });
  }
  return eligibilityOptionLabel(eligibility, t);
}

function bagErrorText(
  reason: LuckyBagErrorReason,
  fallback: string,
  t: ReturnType<typeof useTranslation>['t'],
): string {
  switch (reason) {
    case 'insufficient_coin':
      return t('luckyBag.errorInsufficient', { defaultValue: 'Insufficient coin balance.' });
    case 'active_lucky_bag_exists':
      return t('luckyBag.errorActiveExists', {
        defaultValue: 'There is still an active lucky bag.',
      });
    case 'lucky_bag_closed':
      return t('luckyBag.errorClosed', { defaultValue: 'The lucky bag is closed.' });
    case 'lucky_bag_already_joined':
      return t('luckyBag.errorAlreadyJoined', { defaultValue: 'You already joined.' });
    case 'lucky_bag_not_eligible':
      return t('luckyBag.errorNotEligible', {
        defaultValue: 'You do not meet the join requirement.',
      });
    case 'bad_lucky_bag':
      return t('luckyBag.errorBad', { defaultValue: 'Please check the lucky bag settings.' });
    case 'forbidden':
      return t('luckyBag.errorForbidden', { defaultValue: 'Only the streamer can manage this.' });
    default:
      return fallback || t('luckyBag.errorGeneric', { defaultValue: 'Action failed.' });
  }
}
