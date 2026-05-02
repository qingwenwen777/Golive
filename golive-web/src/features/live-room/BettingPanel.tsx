import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Clock3, Coins, RotateCcw, Trophy, XCircle } from 'lucide-react';
import { toast } from 'sonner';
import { useLatestBet, useOpenBet, usePlaceBet, useSettleBet, useCancelBet } from '@/api/bet';
import { useMe } from '@/api/auth';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useIsAuthed } from '@/stores/useAuthStore';
import type { BetOption, BetOptionSummary } from '@/types/bet';
import { cn } from '@/lib/cn';

const DEFAULT_AMOUNT = 1000;
const MAX_QUESTION_LENGTH = 80;
const OPTIONS: BetOption[] = ['win', 'lose'];

export function BettingPanel({
  roomId,
  ownsStream,
}: {
  roomId: string;
  ownsStream: boolean;
}) {
  const { t } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const latest = useLatestBet(roomId, Boolean(roomId));
  const openBet = useOpenBet(roomId);
  const placeBet = usePlaceBet(roomId);
  const settleBet = useSettleBet(roomId);
  const cancelBet = useCancelBet(roomId);
  const me = useMe();
  const [amount, setAmount] = useState(DEFAULT_AMOUNT);
  const [question, setQuestion] = useState('');
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
  const canOpenRound = ownsStream && (!round || round.status === 'settled' || round.status === 'cancelled');
  const optionMap = useMemo(
    () => new Map(summary.map((item) => [item.option, item] as const)),
    [summary],
  );

  const handleOpen = () => {
    const safeQuestion = question.trim();
    if (!safeQuestion) {
      toast.error(t('betting.errorQuestionRequired', { defaultValue: 'Enter a bet title first.' }));
      return;
    }
    const safeAmount = Math.max(1, Math.floor(amount));
    openBet.mutate(
      { amount: safeAmount, question: safeQuestion },
      {
        onSuccess: () => {
          setQuestion('');
          toast.success(t('betting.openSuccess', { defaultValue: 'Bet opened. It closes automatically in 60 seconds.' }));
        },
        onError: (err) => toast.error(betErrorText(err.reason, err.message, t)),
      },
    );
  };

  const handleWager = (option: BetOption) => {
    if (!round) return;
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (!accepting) {
      toast.error(t('betting.errorClosed', { defaultValue: 'Betting is closed.' }));
      return;
    }
    if (myWager) {
      toast.error(t('betting.errorAlreadyPlaced', { defaultValue: 'You already placed a bet.' }));
      return;
    }
    if (round.amount > balance) {
      toast.error(t('betting.errorInsufficient', { defaultValue: 'Insufficient coin balance.' }));
      return;
    }
    placeBet.mutate(
      { roundId: round.id, option },
      {
        onSuccess: () =>
          toast.success(
            t('betting.wagerSuccess', {
              option: betOptionLabel(option, t),
              defaultValue: 'Bet placed on "{{option}}".',
            }),
          ),
        onError: (err) => toast.error(betErrorText(err.reason, err.message, t)),
      },
    );
  };

  const handleSettle = (option: BetOption) => {
    if (!round) return;
    settleBet.mutate(
      { roundId: round.id, option },
      {
        onSuccess: () =>
          toast.success(
            t('betting.settleSuccess', {
              option: betOptionLabel(option, t),
              defaultValue: 'Bet settled as "{{option}}".',
            }),
          ),
        onError: (err) => toast.error(betErrorText(err.reason, err.message, t)),
      },
    );
  };

  const handleCancel = () => {
    if (!round) return;
    cancelBet.mutate(
      { roundId: round.id },
      {
        onSuccess: () => toast.success(t('betting.cancelSuccess', { defaultValue: 'Bet cancelled. Wagered coins were refunded.' })),
        onError: (err) => toast.error(betErrorText(err.reason, err.message, t)),
      },
    );
  };

  if (!round && !ownsStream) return null;

  return (
    <section className="gl-bet-panel" aria-label={t('betting.title', { defaultValue: 'Betting' })}>
      <div className="gl-bet-head">
        <div className="gl-bet-icon" aria-hidden="true">
          <Trophy size={17} />
        </div>
        <div>
          <h2>{t('betting.title', { defaultValue: 'Betting' })}</h2>
          <p>{round?.question ?? t('betting.emptyHint', { defaultValue: 'Enter a title to open a bet for viewers.' })}</p>
        </div>
        {round && (
          <span className={cn('gl-bet-status', `is-${effectiveStatus}`)}>
            {statusText(effectiveStatus, t)}
          </span>
        )}
      </div>

      {!round ? (
        <OpenBetForm
          amount={amount}
          question={question}
          pending={openBet.isPending}
          submitLabel={t('betting.open', { defaultValue: 'Open bet' })}
          reopen={false}
          onAmountChange={setAmount}
          onQuestionChange={setQuestion}
          onSubmit={handleOpen}
        />
      ) : (
        <>
          <div className="gl-bet-meta">
            <span>
              <Coins size={14} />
              {round.amount.toLocaleString()} coin
            </span>
            <span>
              <Clock3 size={14} />
              {accepting
                ? t('betting.closesIn', {
                    seconds: Math.ceil(remainingMs / 1000),
                    defaultValue: '{{seconds}}s to close',
                  })
                : t('betting.closed', { defaultValue: 'Closed' })}
            </span>
            <span>
              {t('betting.pool', {
                total: totalPool.toLocaleString(),
                defaultValue: 'Pool {{total}}',
              })}
            </span>
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
              {t('betting.myWager', {
                option: betOptionLabel(myWager.option, t),
                defaultValue: 'You bet on "{{option}}"',
              })}
              {myWager.status === 'won' &&
                t('betting.myWagerWon', {
                  payout: myWager.payout.toLocaleString(),
                  defaultValue: ', returned {{payout}} coin',
                })}
              {myWager.status === 'refunded' &&
                t('betting.myWagerRefunded', { defaultValue: ', refunded' })}
            </div>
          )}

          {ownsStream && round.status !== 'settled' && round.status !== 'cancelled' && (
            <div className="gl-bet-host-actions">
              {OPTIONS.map((option) => {
                const count = optionMap.get(option)?.count ?? 0;
                return (
                  <button
                    key={option}
                    type="button"
                    onClick={() => handleSettle(option)}
                    disabled={settleBet.isPending}
                    title={
                      count === 0
                        ? t('betting.noWagerHint', {
                            defaultValue: 'No one picked this option. Cancel to refund coins.',
                          })
                        : undefined
                    }
                  >
                    <Trophy size={14} />
                    <span>{betOptionLabel(option, t)}</span>
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
                <span>
                  {cancelBet.isPending
                    ? t('betting.cancelling', { defaultValue: 'Cancelling...' })
                    : t('betting.cancel', { defaultValue: 'Cancel' })}
                </span>
              </button>
            </div>
          )}

          {canOpenRound && (
            <OpenBetForm
              amount={amount}
              question={question}
              pending={openBet.isPending}
              submitLabel={t('betting.reopen', { defaultValue: 'Open another' })}
              reopen
              onAmountChange={setAmount}
              onQuestionChange={setQuestion}
              onSubmit={handleOpen}
            />
          )}
        </>
      )}
    </section>
  );
}

function OpenBetForm({
  amount,
  question,
  pending,
  submitLabel,
  reopen,
  onAmountChange,
  onQuestionChange,
  onSubmit,
}: {
  amount: number;
  question: string;
  pending: boolean;
  submitLabel: string;
  reopen: boolean;
  onAmountChange: (value: number) => void;
  onQuestionChange: (value: string) => void;
  onSubmit: () => void;
}) {
  const { t } = useTranslation('pages');
  const disabled = pending || question.trim().length === 0;
  return (
    <div className="gl-bet-open">
      <label className="is-title">
        <span>{t('betting.formTitle', { defaultValue: 'Title' })}</span>
        <input
          type="text"
          maxLength={MAX_QUESTION_LENGTH}
          placeholder={t('betting.formTitlePlaceholder', {
            defaultValue: 'Example: Who takes game one?',
          })}
          value={question}
          onChange={(event) => onQuestionChange(event.target.value)}
        />
      </label>
      <label>
        <span>{t('betting.amount', { defaultValue: 'Wager amount' })}</span>
        <input
          type="number"
          min={1}
          step={100}
          value={amount}
          onChange={(event) => onAmountChange(Number(event.target.value))}
        />
      </label>
      <button type="button" onClick={onSubmit} disabled={disabled}>
        {reopen ? <RotateCcw size={15} /> : <Coins size={15} />}
        <span>{pending ? t('betting.opening', { defaultValue: 'Opening...' }) : submitLabel}</span>
      </button>
    </div>
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
  const { t } = useTranslation('pages');
  const total = summary?.total ?? 0;
  const count = summary?.count ?? 0;
  return (
    <button
      type="button"
      className={cn('gl-bet-option', option === 'win' ? 'is-win' : 'is-lose', active && 'is-active')}
      disabled={disabled}
      onClick={onClick}
    >
      <span className="gl-bet-option-title">{betOptionLabel(option, t)}</span>
      <span className="gl-bet-option-meta">
        {t('betting.optionMeta', {
          count,
          total: total.toLocaleString(),
          defaultValue: '{{count}} people · {{total}} coin',
        })}
      </span>
    </button>
  );
}

function statusText(status: string | undefined, t: ReturnType<typeof useTranslation>['t']): string {
  if (status === 'open') return t('betting.status.open', { defaultValue: 'Open' });
  if (status === 'closed') return t('betting.status.closed', { defaultValue: 'Closed' });
  if (status === 'settled') return t('betting.status.settled', { defaultValue: 'Settled' });
  if (status === 'cancelled') return t('betting.status.cancelled', { defaultValue: 'Cancelled' });
  return t('betting.title', { defaultValue: 'Betting' });
}

function betOptionLabel(option: BetOption, t: ReturnType<typeof useTranslation>['t']): string {
  return option === 'win'
    ? t('betting.option.win', { defaultValue: 'Can win' })
    : t('betting.option.lose', { defaultValue: 'Cannot win' });
}

function betErrorText(
  reason: string,
  fallback: string,
  t: ReturnType<typeof useTranslation>['t'],
): string {
  if (reason === 'insufficient_coin') {
    return t('betting.errorInsufficient', { defaultValue: 'Insufficient coin balance.' });
  }
  if (reason === 'active_bet_exists') {
    return t('betting.errorActiveExists', { defaultValue: 'There is still an unsettled bet.' });
  }
  if (reason === 'bet_closed') {
    return t('betting.errorClosed', { defaultValue: 'Betting is closed.' });
  }
  if (reason === 'bet_already_placed') {
    return t('betting.errorAlreadyPlaced', { defaultValue: 'You already placed a bet.' });
  }
  if (reason === 'bet_no_winners') {
    return t('betting.errorNoWinners', { defaultValue: 'No winner for this result. Cancel to refund.' });
  }
  if (reason === 'bad_bet_question') {
    return t('betting.errorBadQuestion', { defaultValue: 'Bet title is required and must be 80 characters or fewer.' });
  }
  if (reason === 'forbidden') {
    return t('betting.errorForbidden', { defaultValue: 'Only the streamer can manage betting.' });
  }
  return fallback || t('betting.errorGeneric', { defaultValue: 'Action failed.' });
}
