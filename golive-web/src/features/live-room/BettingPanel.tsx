import { useEffect, useMemo, useState } from 'react';
import { Clock3, Coins, RotateCcw, Trophy, XCircle } from 'lucide-react';
import { toast } from 'sonner';
import { useLatestBet, useOpenBet, usePlaceBet, useSettleBet, useCancelBet } from '@/api/bet';
import { useMe } from '@/api/auth';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useIsAuthed } from '@/stores/useAuthStore';
import { betOptionLabel, type BetOption, type BetOptionSummary } from '@/types/bet';
import { cn } from '@/lib/cn';

const DEFAULT_AMOUNT = 1000;
const OPTIONS: BetOption[] = ['win', 'lose'];

export function BettingPanel({
  roomId,
  ownsStream,
}: {
  roomId: string;
  ownsStream: boolean;
}) {
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const latest = useLatestBet(roomId, Boolean(roomId));
  const openBet = useOpenBet(roomId);
  const placeBet = usePlaceBet(roomId);
  const settleBet = useSettleBet(roomId);
  const cancelBet = useCancelBet(roomId);
  const me = useMe();
  const [amount, setAmount] = useState(DEFAULT_AMOUNT);
  const [now, setNow] = useState(() => Date.now());

  const round = latest.data?.round ?? null;
  const summary = latest.data?.summary ?? [];
  const myWager = latest.data?.myWager;
  const balance = me.data?.coinBalance ?? 0;

  useEffect(() => {
    if (!round || round.status !== 'open') return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [round]);

  const remainingMs = round ? new Date(round.closeAt).getTime() - now : 0;
  const accepting = Boolean(round && round.status === 'open' && remainingMs > 0);
  const effectiveStatus = round?.status === 'open' && remainingMs <= 0 ? 'closed' : round?.status;
  const totalPool = summary.reduce((sum, item) => sum + item.total, 0);
  const optionMap = useMemo(
    () => new Map(summary.map((item) => [item.option, item] as const)),
    [summary],
  );

  const handleOpen = () => {
    const safeAmount = Math.max(1, Math.floor(amount));
    openBet.mutate(
      { amount: safeAmount },
      {
        onSuccess: () => toast.success('竞猜已开盘，60 秒后自动封盘。'),
        onError: (err) => toast.error(betErrorText(err.reason, err.message)),
      },
    );
  };

  const handleWager = (option: BetOption) => {
    if (!round) return;
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (round.amount > balance) {
      toast.error('余额不足，无法下注。');
      return;
    }
    placeBet.mutate(
      { roundId: round.id, option },
      {
        onSuccess: () => toast.success(`已下注“${betOptionLabel(option)}”。`),
        onError: (err) => toast.error(betErrorText(err.reason, err.message)),
      },
    );
  };

  const handleSettle = (option: BetOption) => {
    if (!round) return;
    settleBet.mutate(
      { roundId: round.id, option },
      {
        onSuccess: () => toast.success(`竞猜已按“${betOptionLabel(option)}”结算。`),
        onError: (err) => toast.error(betErrorText(err.reason, err.message)),
      },
    );
  };

  const handleCancel = () => {
    if (!round) return;
    cancelBet.mutate(
      { roundId: round.id },
      {
        onSuccess: () => toast.success('已流盘，下注 coin 已退回。'),
        onError: (err) => toast.error(betErrorText(err.reason, err.message)),
      },
    );
  };

  if (!round && !ownsStream) return null;

  return (
    <section className="gl-bet-panel" aria-label="竞猜">
      <div className="gl-bet-head">
        <div className="gl-bet-icon" aria-hidden="true">
          <Trophy size={17} />
        </div>
        <div>
          <h2>竞猜</h2>
          <p>{round?.question ?? '这把能不能赢'}</p>
        </div>
        {round && (
          <span className={cn('gl-bet-status', `is-${effectiveStatus}`)}>
            {statusText(effectiveStatus)}
          </span>
        )}
      </div>

      {!round ? (
        <div className="gl-bet-open">
          <label>
            <span>下注额</span>
            <input
              type="number"
              min={1}
              step={100}
              value={amount}
              onChange={(event) => setAmount(Number(event.target.value))}
            />
          </label>
          <button type="button" onClick={handleOpen} disabled={openBet.isPending}>
            <Coins size={15} />
            <span>{openBet.isPending ? '开盘中...' : '开盘'}</span>
          </button>
        </div>
      ) : (
        <>
          <div className="gl-bet-meta">
            <span>
              <Coins size={14} />
              {round.amount.toLocaleString()} coin
            </span>
            <span>
              <Clock3 size={14} />
              {accepting ? `${Math.ceil(remainingMs / 1000)} 秒封盘` : '已封盘'}
            </span>
            <span>池子 {totalPool.toLocaleString()}</span>
          </div>

          <div className="gl-bet-options">
            {OPTIONS.map((option) => (
              <BetOptionButton
                key={option}
                option={option}
                summary={optionMap.get(option)}
                active={myWager?.option === option}
                disabled={!accepting || !!myWager || placeBet.isPending}
                onClick={() => handleWager(option)}
              />
            ))}
          </div>

          {myWager && (
            <div className={cn('gl-bet-my', `is-${myWager.status}`)}>
              你已下注“{betOptionLabel(myWager.option)}”
              {myWager.status === 'won' && `，返还 ${myWager.payout.toLocaleString()} coin`}
              {myWager.status === 'refunded' && '，已退回'}
            </div>
          )}

          {ownsStream && (effectiveStatus === 'closed' || round.status === 'open') && (
            <div className="gl-bet-host-actions">
              {OPTIONS.map((option) => {
                const count = optionMap.get(option)?.count ?? 0;
                return (
                  <button
                    key={option}
                    type="button"
                    onClick={() => handleSettle(option)}
                    disabled={settleBet.isPending || count === 0}
                    title={count === 0 ? '没有赢家时请流盘' : undefined}
                  >
                    <Trophy size={14} />
                    <span>{option === 'win' ? '能赢' : '不能赢'}</span>
                  </button>
                );
              })}
              <button
                type="button"
                className="is-cancel"
                onClick={handleCancel}
                disabled={cancelBet.isPending}
              >
                <XCircle size={14} />
                <span>{cancelBet.isPending ? '流盘中...' : '流盘'}</span>
              </button>
            </div>
          )}

          {ownsStream && (round.status === 'settled' || round.status === 'cancelled') && (
            <div className="gl-bet-host-actions">
              <button type="button" onClick={handleOpen} disabled={openBet.isPending}>
                <RotateCcw size={14} />
                <span>再开一盘</span>
              </button>
            </div>
          )}
        </>
      )}
    </section>
  );
}

function BetOptionButton({
  option,
  summary,
  active,
  disabled,
  onClick,
}: {
  option: BetOption;
  summary?: BetOptionSummary;
  active?: boolean;
  disabled?: boolean;
  onClick: () => void;
}) {
  const total = summary?.total ?? 0;
  const count = summary?.count ?? 0;
  return (
    <button
      type="button"
      className={cn('gl-bet-option', option === 'win' ? 'is-win' : 'is-lose', active && 'is-active')}
      disabled={disabled}
      onClick={onClick}
    >
      <span className="gl-bet-option-title">{betOptionLabel(option)}</span>
      <span className="gl-bet-option-meta">
        {count} 人 · {total.toLocaleString()} coin
      </span>
    </button>
  );
}

function statusText(status?: string): string {
  if (status === 'open') return '开盘中';
  if (status === 'closed') return '封盘';
  if (status === 'settled') return '已结算';
  if (status === 'cancelled') return '流盘';
  return '竞猜';
}

function betErrorText(reason: string, fallback: string): string {
  if (reason === 'insufficient_coin') return '余额不足，无法下注。';
  if (reason === 'active_bet_exists') return '当前还有未结算的竞猜。';
  if (reason === 'bet_closed') return '竞猜已封盘。';
  if (reason === 'bet_already_placed') return '你已经下注了。';
  if (reason === 'bet_no_winners') return '这个结果没有赢家，请选择流盘退回。';
  if (reason === 'forbidden') return '只有主播可以操作竞猜。';
  return fallback || '操作失败。';
}
