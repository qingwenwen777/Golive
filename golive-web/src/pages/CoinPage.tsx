import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useSearchParams } from 'react-router-dom';
import { toast } from 'sonner';
import {
  ArrowDownLeft,
  ArrowUpRight,
  CalendarCheck,
  Coins,
  CreditCard,
  Gift,
  History,
  MessageSquareText,
  Radio,
  Sparkles,
  Trophy,
  Wallet,
} from 'lucide-react';
import { useMe, useTopupCoins } from '@/api/auth';
import {
  useBindStripeTestAccount,
  useClaimDailyCoinTask,
  useCoinTransactions,
  useConfirmTopupCoins,
  useStripeAccountLink,
  useStripeAccountStatus,
  useWithdrawCoins,
  type CoinTransaction,
  type CoinTransactionType,
} from '@/api/coins';
import { readDailyCoinActivity, coinTodayKey } from '@/lib/coinActivity';
import { cn } from '@/lib/cn';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import { userDisplayName } from '@/types/user';

type RecordFilter =
  | 'all'
  | 'income'
  | 'spend'
  | 'recharge'
  | 'gift'
  | 'sc'
  | 'bet'
  | 'task'
  | 'creator';

const COINS_PER_CURRENCY_UNIT = 10;
const STRIPE_DISPLAY_CURRENCY = 'USD';
const MIN_TOPUP_COINS = 10;
const MIN_WITHDRAW_COINS = 10;
const QUICK_TOPUPS = [10, 50, 100, 500] as const;
const WITHDRAW_FEE_RATE = 0.35;
const CERTIFIED_WITHDRAW_FEE_RATE = 0.25;
const RECORD_PAGE_SIZE = 8;

const DAILY_TASKS = [
  {
    id: 'daily-login-lottery',
    titleKey: 'coin.tasks.dailyLogin.title',
    titleDefault: 'Daily login lottery',
    descriptionKey: 'coin.tasks.dailyLogin.description',
    descriptionDefault: 'Draw once each day for a small coin bonus.',
    reward: '6-18',
    targetLabelKey: 'coin.tasks.dailyLogin.target',
    targetLabelDefault: 'available today',
    target: 1,
    kind: 'login',
  },
  {
    id: 'watch-3-lives',
    titleKey: 'coin.tasks.watchRooms.title',
    titleDefault: 'Watch 3 live rooms',
    descriptionKey: 'coin.tasks.watchRooms.description',
    descriptionDefault: 'Open 3 different live rooms today to claim.',
    reward: '18',
    targetLabelKey: 'coin.tasks.watchRooms.target',
    targetLabelDefault: '3 live rooms',
    target: 3,
    kind: 'rooms',
  },
  {
    id: 'watch-30-minutes',
    titleKey: 'coin.tasks.watchMinutes.title',
    titleDefault: 'Watch 30 minutes',
    descriptionKey: 'coin.tasks.watchMinutes.description',
    descriptionDefault: 'Spend a little real time watching to earn coins.',
    reward: '25',
    targetLabelKey: 'coin.tasks.watchMinutes.target',
    targetLabelDefault: '30 minutes',
    target: 30 * 60,
    kind: 'time',
  },
] as const;

const FILTERS: Array<{ id: RecordFilter; labelKey: string; labelDefault: string }> = [
  { id: 'all', labelKey: 'coin.filters.all', labelDefault: 'All' },
  { id: 'income', labelKey: 'coin.filters.income', labelDefault: 'Income' },
  { id: 'spend', labelKey: 'coin.filters.spend', labelDefault: 'Spending' },
  { id: 'recharge', labelKey: 'coin.filters.recharge', labelDefault: 'Top-up' },
  { id: 'gift', labelKey: 'coin.filters.gift', labelDefault: 'Gifts' },
  { id: 'sc', labelKey: 'coin.filters.sc', labelDefault: 'SC' },
  { id: 'bet', labelKey: 'coin.filters.bet', labelDefault: 'Betting' },
  { id: 'task', labelKey: 'coin.filters.task', labelDefault: 'Tasks' },
  { id: 'creator', labelKey: 'coin.filters.creator', labelDefault: 'Live income' },
];

export default function CoinPage() {
  const { t, i18n } = useTranslation('pages');
  const [searchParams] = useSearchParams();
  const rechargeRef = useRef<HTMLDivElement | null>(null);
  const user = useAuthStore((s) => s.user);
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const me = useMe();
  const currentUser = me.data ?? user;
  const balance = currentUser?.coinBalance ?? 0;
  const frozenBalance = currentUser?.frozenCoins ?? 0;
  const availableBalance = Math.max(0, balance - frozenBalance);
  const transactions = useCoinTransactions();
  const topup = useTopupCoins();
  const confirmTopup = useConfirmTopupCoins();
  const stripeAccount = useStripeAccountStatus();
  const stripeAccountLink = useStripeAccountLink();
  const bindStripeTestAccount = useBindStripeTestAccount();
  const withdraw = useWithdrawCoins();
  const claimTask = useClaimDailyCoinTask();

  const [topupText, setTopupText] = useState(String(MIN_TOPUP_COINS));
  const [withdrawText, setWithdrawText] = useState('10000');
  const [testAccountText, setTestAccountText] = useState('');
  const [filter, setFilter] = useState<RecordFilter>('all');
  const [recordPage, setRecordPage] = useState(1);
  const [claimingTaskId, setClaimingTaskId] = useState<string | null>(null);
  const [activity, setActivity] = useState(() => readDailyCoinActivity(user?.id));
  const handledStripeReturn = useRef<string | null>(null);

  useEffect(() => {
    setActivity(readDailyCoinActivity(currentUser?.id));
    const timer = window.setInterval(() => {
      setActivity(readDailyCoinActivity(currentUser?.id));
    }, 5000);
    return () => window.clearInterval(timer);
  }, [currentUser?.id]);

  useEffect(() => {
    if (searchParams.get('focus') !== 'recharge') return;
    window.setTimeout(() => {
      rechargeRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }, 80);
  }, [searchParams]);

  useEffect(() => {
    const topupStatus = searchParams.get('stripe_topup');
    const sessionId = searchParams.get('session_id');
    const connectStatus = searchParams.get('stripe_connect');
    if (!topupStatus && !connectStatus) return;

    const key = `${topupStatus ?? ''}:${sessionId ?? ''}:${connectStatus ?? ''}`;
    if (handledStripeReturn.current === key) return;
    handledStripeReturn.current = key;

    const clearStripeParams = () => {
      const next = new URL(window.location.href);
      next.searchParams.delete('stripe_topup');
      next.searchParams.delete('session_id');
      next.searchParams.delete('stripe_connect');
      window.history.replaceState({}, '', `${next.pathname}${next.search}${next.hash}`);
    };

    if (topupStatus === 'cancelled') {
      toast.info(
        t('coin.toast.topupCancelled', { defaultValue: 'Stripe checkout was cancelled.' }),
      );
      clearStripeParams();
      return;
    }

    if (topupStatus === 'success' && sessionId) {
      confirmTopup.mutate(
        { sessionId },
        {
          onSuccess: (resp) => {
            toast.success(
              resp.credited
                ? t('coin.toast.topupSuccess', {
                    balance: resp.user.coinBalance.toLocaleString(),
                    defaultValue: 'Top-up complete. Current balance: {{balance}} coins.',
                  })
                : t('coin.toast.topupAlreadyCredited', {
                    defaultValue: 'This Stripe payment was already credited.',
                  }),
            );
            clearStripeParams();
          },
          onError: (err) => {
            toast.error(
              err.message ||
                t('coin.toast.topupVerifyFailed', {
                  defaultValue: 'Could not verify the Stripe payment.',
                }),
            );
            clearStripeParams();
          },
        },
      );
      return;
    }

    if (connectStatus) {
      toast.info(
        connectStatus === 'refresh'
          ? t('coin.toast.stripeConnectRefresh', {
              defaultValue: 'Continue Stripe onboarding to enable withdrawals.',
            })
          : t('coin.toast.stripeConnectReturned', {
              defaultValue: 'Stripe account status refreshed.',
            }),
      );
      void stripeAccount.refetch();
      clearStripeParams();
    }
  }, [confirmTopup, searchParams, stripeAccount, t]);

  const rows = transactions.data?.items ?? [];
  const monthKey = new Date().toISOString().slice(0, 7);
  const monthRows = rows.filter((item) => item.createdAt.slice(0, 7) === monthKey);
  const monthSpend = Math.abs(sum(monthRows.filter((item) => item.amount < 0)));
  const monthIncome = sum(monthRows.filter((item) => item.amount > 0));
  const creatorIncome = sum(rows.filter((item) => isCreatorIncome(item.type)));

  const filteredRows = useMemo(
    () => rows.filter((item) => matchesFilter(item, filter)),
    [filter, rows],
  );
  const totalRecordPages = Math.max(1, Math.ceil(filteredRows.length / RECORD_PAGE_SIZE));
  const visibleRows = filteredRows.slice(
    (recordPage - 1) * RECORD_PAGE_SIZE,
    recordPage * RECORD_PAGE_SIZE,
  );

  useEffect(() => {
    setRecordPage(1);
  }, [filter]);

  useEffect(() => {
    setRecordPage((page) => Math.min(page, totalRecordPages));
  }, [totalRecordPages]);

  const today = coinTodayKey();
  const claimedByTask = useMemo(() => {
    const next = new Map<string, CoinTransaction>();
    for (const item of rows) {
      if (item.type !== 'daily_task' || !item.sourceId) continue;
      const [taskId, date] = item.sourceId.split(':');
      if (date === today) next.set(taskId, item);
    }
    return next;
  }, [rows, today]);

  const topupAmount = parseCoinInput(topupText);
  const withdrawAmount = parseCoinInput(withdrawText);
  const platformCertified =
    currentUser?.verified === true &&
    currentUser.livePermissionStatus === 'approved' &&
    currentUser.platformVerificationStatus === 'approved';
  const withdrawFeeRate = platformCertified ? CERTIFIED_WITHDRAW_FEE_RATE : WITHDRAW_FEE_RATE;
  const withdrawFee = Math.floor(withdrawAmount * withdrawFeeRate);
  const withdrawNet = Math.max(0, withdrawAmount - withdrawFee);
  const canWithdrawWithStripe = Boolean(stripeAccount.data?.canWithdraw);
  const showTestAccountBind = Boolean(stripeAccount.data?.testMode && !canWithdrawWithStripe);

  const handleTopup = () => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (topupAmount < MIN_TOPUP_COINS) {
      toast.error(t('coin.toast.minTopup', { defaultValue: 'Top-up must be at least 10 coins.' }));
      return;
    }
    topup.mutate(
      { amount: topupAmount },
      {
        onSuccess: (resp) => {
          if (resp.checkoutUrl) {
            window.location.assign(resp.checkoutUrl);
            return;
          }
          toast.error(t('coin.toast.topupFailed', { defaultValue: 'Top-up failed.' }));
        },
        onError: (err) =>
          toast.error(
            err.message || t('coin.toast.topupFailed', { defaultValue: 'Top-up failed.' }),
          ),
      },
    );
  };

  const handleClaim = (task: (typeof DAILY_TASKS)[number], progress: number) => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (claimedByTask.has(task.id)) return;
    if (progress < task.target) {
      toast.info(
        t('coin.toast.taskIncomplete', { defaultValue: 'Finish the task before claiming.' }),
      );
      return;
    }
    setClaimingTaskId(task.id);
    claimTask.mutate(
      { taskId: task.id },
      {
        onSuccess: (resp) => {
          if (resp.alreadyClaimed) {
            toast.info(
              t('coin.toast.alreadyClaimed', {
                defaultValue: 'You already claimed this task today.',
              }),
            );
            return;
          }
          toast.success(
            t('coin.toast.claimSuccess', {
              amount: resp.transaction.amount.toLocaleString(),
              defaultValue: 'Earned {{amount}} coins.',
            }),
          );
        },
        onError: (err) =>
          toast.error(
            err.message || t('coin.toast.claimFailed', { defaultValue: 'Claim failed.' }),
          ),
        onSettled: () => setClaimingTaskId(null),
      },
    );
  };

  const startStripeOnboarding = () => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    stripeAccountLink.mutate(undefined, {
      onSuccess: (resp) => {
        window.location.assign(resp.url);
      },
      onError: (err) =>
        toast.error(
          err.message ||
            t('coin.toast.stripeConnectFailed', {
              defaultValue: 'Could not start Stripe onboarding.',
            }),
        ),
    });
  };

  const handleWithdraw = () => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (withdrawAmount < MIN_WITHDRAW_COINS) {
      toast.error(
        t('coin.toast.minWithdraw', { defaultValue: 'Withdrawal must be at least 10 coins.' }),
      );
      return;
    }
    if (withdrawAmount > availableBalance) {
      toast.error(
        t('coin.toast.withdrawInsufficient', {
          defaultValue: 'Insufficient available coin balance.',
        }),
      );
      return;
    }
    if (!canWithdrawWithStripe) {
      if (stripeAccount.data?.testMode) {
        toast.info(
          t('coin.toast.bindTestAccountFirst', {
            defaultValue: 'Bind a Stripe test connected account before withdrawing.',
          }),
        );
        return;
      }
      startStripeOnboarding();
      return;
    }
    withdraw.mutate(
      { amount: withdrawAmount },
      {
        onSuccess: (resp) => {
          toast.success(
            t('coin.toast.withdrawSuccess', {
              amount: resp.netCoins.toLocaleString(),
              balance: resp.user.coinBalance.toLocaleString(),
              defaultValue:
                'Withdrawal sent to Stripe. Net {{amount}} coins, current balance {{balance}}.',
            }),
          );
        },
        onError: (err) =>
          toast.error(
            err.message ||
              t('coin.toast.withdrawFailed', { defaultValue: 'Stripe withdrawal failed.' }),
          ),
      },
    );
  };

  const handleBindTestAccount = () => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    const accountId = testAccountText.trim();
    if (!accountId.startsWith('acct_')) {
      toast.error(
        t('coin.toast.invalidStripeAccount', {
          defaultValue: 'Enter a valid Stripe connected account id.',
        }),
      );
      return;
    }
    bindStripeTestAccount.mutate(
      { accountId },
      {
        onSuccess: (resp) => {
          setTestAccountText(resp.account.accountId ?? accountId);
          toast.success(
            t('coin.toast.stripeAccountBound', {
              accountId: resp.account.accountId ?? accountId,
              defaultValue: 'Stripe test account {{accountId}} bound.',
            }),
          );
        },
        onError: (err) =>
          toast.error(
            err.message ||
              t('coin.toast.stripeAccountBindFailed', {
                defaultValue: 'Could not bind this Stripe test account.',
              }),
          ),
      },
    );
  };

  return (
    <div className="gl-page gl-coin-page">
      {!isAuthed && (
        <div className="gl-yt-banner">
          <div>
            <strong>{t('coin.authTitle', { defaultValue: 'Sign in to manage coins' })}</strong>
            <span>
              {t('coin.authSub', {
                defaultValue:
                  'View spending records, top up, daily tasks, and withdrawal previews.',
              })}
            </span>
          </div>
          <button className="gl-secondary-btn" type="button" onClick={() => openLogin()}>
            {t('login.title', { defaultValue: 'Sign in' })}
          </button>
        </div>
      )}

      <section className="gl-coin-hero">
        <div className="gl-coin-hero-copy">
          <div className="gl-coin-kicker">
            <Wallet size={16} />
            Coin Center
          </div>
          <h1>{formatCoins(balance)}</h1>
          <p>
            {t('coin.heroSub', {
              name: currentUser
                ? userDisplayName(currentUser)
                : t('coin.guest', { defaultValue: 'Guest' }),
              defaultValue: '{{name}} coin account. Top-up rate: 10 coins = ¥1.',
            })}
          </p>
        </div>
        <div className="gl-coin-hero-actions">
          <button
            type="button"
            className="gl-coin-primary"
            onClick={() => rechargeRef.current?.scrollIntoView({ behavior: 'smooth' })}
          >
            <CreditCard size={16} />
            {t('coin.goTopup', { defaultValue: 'Top up' })}
          </button>
          <Link className="gl-secondary-btn" to="/history">
            <History size={15} />
            {t('coin.watchHistory', { defaultValue: 'Watch history' })}
          </Link>
        </div>
      </section>

      <section className="gl-coin-stats" aria-label="Coin summary">
        <StatPill
          icon={<Coins size={18} />}
          value={formatCoins(balance)}
          label={t('coin.stats.balance', { defaultValue: 'Current balance' })}
        />
        <StatPill
          icon={<ArrowUpRight size={18} />}
          value={formatCoins(monthSpend)}
          label={t('coin.stats.monthSpend', { defaultValue: 'Spent this month' })}
        />
        <StatPill
          icon={<ArrowDownLeft size={18} />}
          value={formatCoins(monthIncome)}
          label={t('coin.stats.monthIncome', { defaultValue: 'Earned this month' })}
        />
        <StatPill
          icon={<Radio size={18} />}
          value={formatCoins(creatorIncome)}
          label={t('coin.stats.liveIncome', { defaultValue: 'Live income' })}
        />
      </section>

      <div className="gl-coin-layout">
        <div className="gl-coin-main">
          <section className="gl-coin-panel">
            <div className="gl-section-title-row">
              <div>
                <h2>{t('coin.dailyTasks', { defaultValue: 'Daily tasks' })}</h2>
                <span>{t('coin.dailyResetHint', { defaultValue: '北京时间 0 点刷新' })}</span>
              </div>
            </div>
            <div className="gl-coin-task-grid">
              {DAILY_TASKS.map((task) => {
                const progress = taskProgress(task.kind, activity);
                const claimed = claimedByTask.get(task.id);
                return (
                  <TaskCard
                    key={task.id}
                    title={t(task.titleKey, { defaultValue: task.titleDefault })}
                    description={t(task.descriptionKey, { defaultValue: task.descriptionDefault })}
                    reward={claimed ? String(claimed.amount) : task.reward}
                    progress={progress}
                    target={task.target}
                    targetLabel={t(task.targetLabelKey, { defaultValue: task.targetLabelDefault })}
                    claimed={Boolean(claimed)}
                    pending={claimingTaskId === task.id && claimTask.isPending}
                    onClaim={() => handleClaim(task, progress)}
                  />
                );
              })}
            </div>
          </section>

          <section className="gl-coin-panel gl-coin-ledger-panel">
            <div className="gl-section-title-row">
              <h2>{t('coin.ledger', { defaultValue: 'Coin ledger' })}</h2>
            </div>
            <div className="gl-coin-filters" role="tablist" aria-label="Coin record filters">
              {FILTERS.map((item) => (
                <button
                  key={item.id}
                  type="button"
                  className={cn('gl-coin-filter', filter === item.id && 'is-active')}
                  onClick={() => setFilter(item.id)}
                >
                  {t(item.labelKey, { defaultValue: item.labelDefault })}
                </button>
              ))}
            </div>

            {transactions.isPending && isAuthed ? (
              <div className="gl-coin-record-list" aria-busy="true">
                {Array.from({ length: 5 }).map((_, i) => (
                  <div key={i} className="gl-coin-record is-loading" />
                ))}
              </div>
            ) : filteredRows.length === 0 ? (
              <div className="gl-coin-empty">
                <Sparkles size={32} />
                <strong>{t('coin.emptyTitle', { defaultValue: 'No coin records yet' })}</strong>
                <span>
                  {t('coin.emptySub', {
                    defaultValue:
                      'Top-ups, gifts, Super Chats, and betting activity will appear here.',
                  })}
                </span>
              </div>
            ) : (
              <>
                <div className="gl-coin-record-list">
                  {visibleRows.map((item) => (
                    <CoinRecordRow key={item.id} item={item} />
                  ))}
                </div>
                {totalRecordPages > 1 && (
                  <RecordPagination
                    page={recordPage}
                    totalPages={totalRecordPages}
                    totalItems={filteredRows.length}
                    onPageChange={setRecordPage}
                  />
                )}
              </>
            )}
          </section>
        </div>

        <aside className="gl-coin-side">
          <section className="gl-coin-panel" ref={rechargeRef}>
            <div className="gl-coin-panel-head">
              <div>
                <h2>{t('coin.topup.title', { defaultValue: 'Top up' })}</h2>
                <p>{t('coin.topup.sub', { defaultValue: 'Minimum 10 coins. No upper limit.' })}</p>
              </div>
              <CreditCard size={20} />
            </div>
            <label className="gl-coin-input-label">
              <span>{t('coin.topup.amount', { defaultValue: 'Top-up amount' })}</span>
              <div className="gl-coin-input">
                <input
                  inputMode="numeric"
                  value={topupText}
                  onChange={(event) => setTopupText(cleanCoinText(event.target.value))}
                  onBlur={() => {
                    if (topupAmount < MIN_TOPUP_COINS) setTopupText(String(MIN_TOPUP_COINS));
                  }}
                  aria-label={t('coin.topup.amountAria', { defaultValue: 'Top-up coin amount' })}
                />
                <span>coins</span>
              </div>
            </label>
            <div className="gl-coin-quick">
              {QUICK_TOPUPS.map((amount) => (
                <button key={amount} type="button" onClick={() => setTopupText(String(amount))}>
                  {amount.toLocaleString()}
                </button>
              ))}
            </div>
            <div className="gl-coin-exchange">
              <span>{t('coin.topup.pay', { defaultValue: 'Pay' })}</span>
              <strong>{formatFiat(topupAmount, i18n.language)}</strong>
            </div>
            <button
              type="button"
              className="gl-coin-primary gl-coin-wide"
              onClick={handleTopup}
              disabled={topup.isPending}
            >
              <CreditCard size={16} />
              {topup.isPending
                ? t('coin.topup.pending', { defaultValue: 'Opening Stripe...' })
                : t('coin.topup.submit', { defaultValue: 'Pay with Stripe' })}
            </button>
          </section>

          <section className="gl-coin-panel">
            <div className="gl-coin-panel-head">
              <div>
                <h2>{t('coin.withdraw.title', { defaultValue: 'Withdraw' })}</h2>
                <p>
                  {platformCertified
                    ? t('coin.withdraw.subCertified', {
                        defaultValue:
                          'Platform certified rate: 25% fee. Paid through Stripe Connect.',
                      })
                    : t('coin.withdraw.sub', {
                        defaultValue: '35% fee. Paid through Stripe Connect.',
                      })}
                </p>
              </div>
              <Wallet size={20} />
            </div>
            <label className="gl-coin-input-label">
              <span>{t('coin.withdraw.amount', { defaultValue: 'Withdrawal amount' })}</span>
              <div className="gl-coin-input">
                <input
                  inputMode="numeric"
                  value={withdrawText}
                  onChange={(event) => setWithdrawText(cleanCoinText(event.target.value))}
                  aria-label={t('coin.withdraw.amountAria', {
                    defaultValue: 'Withdrawal coin amount',
                  })}
                />
                <span>coins</span>
              </div>
            </label>
            <div className="gl-coin-withdraw-lines">
              <span>
                {t('coin.withdraw.available', { defaultValue: 'Available' })}{' '}
                <strong>{formatCoins(availableBalance)}</strong>
              </span>
              <span>
                {t('coin.withdraw.rate', { defaultValue: 'Fee rate' })}{' '}
                <strong>{Math.round(withdrawFeeRate * 100)}%</strong>
              </span>
              <span>
                {t('coin.withdraw.fee', { defaultValue: 'Fee' })}{' '}
                <strong>{formatCoins(withdrawFee)}</strong>
              </span>
              <span>
                {t('coin.withdraw.net', { defaultValue: 'Estimated arrival' })}{' '}
                <strong>{formatCoins(withdrawNet)}</strong>
              </span>
              <span>
                {t('coin.withdraw.rmb', { defaultValue: 'Stripe amount' })}{' '}
                <strong>{formatFiat(withdrawNet, i18n.language)}</strong>
              </span>
            </div>
            {platformCertified && (
              <div className="gl-coin-certified-note">
                <Trophy size={15} />
                {t('coin.withdraw.certifiedNote', {
                  defaultValue: 'Platform certification reduced this fee by 10 percentage points.',
                })}
              </div>
            )}
            {showTestAccountBind && (
              <label className="gl-coin-input-label">
                <span>
                  {t('coin.withdraw.testAccount', { defaultValue: 'Stripe test account' })}
                </span>
                <div className="gl-coin-input">
                  <input
                    value={testAccountText}
                    onChange={(event) => setTestAccountText(event.target.value)}
                    placeholder="acct_..."
                    aria-label={t('coin.withdraw.testAccountAria', {
                      defaultValue: 'Stripe test connected account id',
                    })}
                  />
                </div>
                <button
                  type="button"
                  className="gl-secondary-btn gl-coin-wide"
                  onClick={handleBindTestAccount}
                  disabled={bindStripeTestAccount.isPending}
                >
                  {bindStripeTestAccount.isPending
                    ? t('coin.withdraw.bindingTestAccount', { defaultValue: 'Binding...' })
                    : t('coin.withdraw.bindTestAccount', { defaultValue: 'Bind test account' })}
                </button>
              </label>
            )}
            <button
              type="button"
              className="gl-secondary-btn gl-coin-wide"
              onClick={handleWithdraw}
              disabled={
                withdraw.isPending ||
                stripeAccountLink.isPending ||
                stripeAccount.isPending ||
                bindStripeTestAccount.isPending
              }
            >
              {stripeAccountLink.isPending
                ? t('coin.withdraw.connecting', { defaultValue: 'Opening Stripe...' })
                : withdraw.isPending
                  ? t('coin.withdraw.pending', { defaultValue: 'Withdrawing...' })
                  : canWithdrawWithStripe
                    ? t('coin.withdraw.submit', { defaultValue: 'Withdraw with Stripe' })
                    : showTestAccountBind
                      ? t('coin.withdraw.bindTestAccount', { defaultValue: 'Bind test account' })
                      : t('coin.withdraw.connectStripe', { defaultValue: 'Connect Stripe' })}
            </button>
          </section>
        </aside>
      </div>
    </div>
  );
}

function StatPill({ icon, value, label }: { icon: ReactNode; value: string; label: string }) {
  return (
    <div className="gl-coin-stat">
      <div className="gl-coin-stat-icon">{icon}</div>
      <div>
        <strong>{value}</strong>
        <span>{label}</span>
      </div>
    </div>
  );
}

function TaskCard({
  title,
  description,
  reward,
  progress,
  target,
  targetLabel,
  claimed,
  pending,
  onClaim,
}: {
  title: string;
  description: string;
  reward: string;
  progress: number;
  target: number;
  targetLabel: string;
  claimed: boolean;
  pending: boolean;
  onClaim: () => void;
}) {
  const { t } = useTranslation('pages');
  const complete = progress >= target;
  const ratio = Math.min(1, progress / target);
  return (
    <article className="gl-coin-task">
      <div className="gl-coin-task-top">
        <div className="gl-coin-task-icon">
          <CalendarCheck size={18} />
        </div>
        <span>+{reward} coins</span>
      </div>
      <h3>{title}</h3>
      <p>{description}</p>
      <div className="gl-coin-progress" aria-label={`${title} progress`}>
        <i style={{ width: `${ratio * 100}%` }} />
      </div>
      <div className="gl-coin-task-bottom">
        <span>{formatProgress(progress, target, targetLabel, t)}</span>
        <button
          type="button"
          onClick={onClaim}
          disabled={claimed || pending || !complete}
          className={cn(complete && !claimed ? 'is-ready' : '')}
        >
          {claimed
            ? t('coin.task.claimed', { defaultValue: 'Claimed' })
            : pending
              ? t('coin.task.claiming', { defaultValue: 'Claiming...' })
              : t('coin.task.claim', { defaultValue: 'Claim' })}
        </button>
      </div>
    </article>
  );
}

function CoinRecordRow({ item }: { item: CoinTransaction }) {
  const { t, i18n } = useTranslation('pages');
  const meta = recordMeta(item.type);
  const Icon = meta.icon;
  const positive = item.amount > 0;
  const metaLabel = t(meta.labelKey, { defaultValue: meta.labelDefault });
  return (
    <article className="gl-coin-record">
      <div className={cn('gl-coin-record-icon', positive ? 'is-income' : 'is-spend')}>
        <Icon size={17} />
      </div>
      <div className="gl-coin-record-main">
        <strong>{item.title || metaLabel}</strong>
        <span>
          {formatTime(item.createdAt, i18n.language)}
          {item.description ? ` · ${item.description}` : ` · ${metaLabel}`}
        </span>
      </div>
      <div className={cn('gl-coin-record-amount', positive ? 'is-income' : 'is-spend')}>
        <strong>
          {positive ? '+' : ''}
          {item.amount.toLocaleString()}
        </strong>
        <span>
          {t('coin.record.balance', {
            balance: item.balanceAfter.toLocaleString(),
            defaultValue: 'Balance {{balance}}',
          })}
        </span>
      </div>
    </article>
  );
}

function RecordPagination({
  page,
  totalPages,
  totalItems,
  onPageChange,
}: {
  page: number;
  totalPages: number;
  totalItems: number;
  onPageChange: (page: number) => void;
}) {
  const { t } = useTranslation('pages');
  return (
    <div className="gl-coin-pagination" aria-label="Coin record pagination">
      <span>
        {t('coin.pagination', {
          total: totalItems.toLocaleString(),
          page,
          totalPages,
          defaultValue: '{{total}} records · Page {{page}} / {{totalPages}}',
        })}
      </span>
      <div className="gl-coin-page-controls">
        <button
          type="button"
          onClick={() => onPageChange(Math.max(1, page - 1))}
          disabled={page <= 1}
        >
          {t('coin.previous', { defaultValue: 'Previous' })}
        </button>
        <button
          type="button"
          onClick={() => onPageChange(Math.min(totalPages, page + 1))}
          disabled={page >= totalPages}
        >
          {t('coin.next', { defaultValue: 'Next' })}
        </button>
      </div>
    </div>
  );
}

function recordMeta(type: CoinTransactionType) {
  switch (type) {
    case 'topup':
      return { labelKey: 'coin.record.topup', labelDefault: 'Top-up', icon: CreditCard };
    case 'withdrawal':
      return { labelKey: 'coin.record.withdrawal', labelDefault: 'Withdrawal', icon: Wallet };
    case 'daily_task':
      return { labelKey: 'coin.record.dailyTask', labelDefault: 'Daily task', icon: CalendarCheck };
    case 'gift_spend':
    case 'creator_gift_income':
      return { labelKey: 'coin.record.gift', labelDefault: 'Gift', icon: Gift };
    case 'super_chat_spend':
    case 'creator_super_chat_income':
      return {
        labelKey: 'coin.record.superChat',
        labelDefault: 'Super Chat',
        icon: MessageSquareText,
      };
    case 'bet_wager':
    case 'bet_payout':
    case 'bet_refund':
      return { labelKey: 'coin.record.bet', labelDefault: 'Betting', icon: Trophy };
    default:
      return { labelKey: 'coin.record.coin', labelDefault: 'Coin', icon: Coins };
  }
}

function matchesFilter(item: CoinTransaction, filter: RecordFilter): boolean {
  if (filter === 'all') return true;
  if (filter === 'income') return item.amount > 0;
  if (filter === 'spend') return item.amount < 0;
  if (filter === 'recharge') return item.type === 'topup';
  if (filter === 'gift') return item.type === 'gift_spend' || item.type === 'creator_gift_income';
  if (filter === 'sc')
    return item.type === 'super_chat_spend' || item.type === 'creator_super_chat_income';
  if (filter === 'bet')
    return item.type === 'bet_wager' || item.type === 'bet_payout' || item.type === 'bet_refund';
  if (filter === 'task') return item.type === 'daily_task';
  if (filter === 'creator') return isCreatorIncome(item.type);
  return true;
}

function isCreatorIncome(type: CoinTransactionType): boolean {
  return type === 'creator_gift_income' || type === 'creator_super_chat_income';
}

function taskProgress(
  kind: (typeof DAILY_TASKS)[number]['kind'],
  activity: ReturnType<typeof readDailyCoinActivity>,
) {
  if (kind === 'login') return 1;
  if (kind === 'rooms') return activity.watchedRoomIds.length;
  return activity.watchSeconds;
}

function formatProgress(
  progress: number,
  target: number,
  targetLabel: string,
  t: ReturnType<typeof useTranslation>['t'],
): string {
  if (target === 1) return targetLabel;
  if (target >= 60) {
    return t('coin.progressMinutes', {
      current: Math.min(Math.floor(progress / 60), target / 60),
      total: target / 60,
      defaultValue: '{{current}} / {{total}} min',
    });
  }
  return `${Math.min(progress, target)} / ${targetLabel}`;
}

function sum(rows: CoinTransaction[]): number {
  return rows.reduce((acc, item) => acc + item.amount, 0);
}

function formatCoins(value: number): string {
  return `${Math.round(value).toLocaleString()} coins`;
}

function formatFiat(coins: number, locale: string): string {
  const amount = Math.max(0, coins) / COINS_PER_CURRENCY_UNIT;
  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency: STRIPE_DISPLAY_CURRENCY,
    maximumFractionDigits: 2,
  }).format(amount);
}

function formatTime(value: string, locale: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(locale, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function parseCoinInput(value: string): number {
  const next = Number(value.replace(/[^\d]/g, ''));
  if (!Number.isFinite(next)) return 0;
  return Math.max(0, Math.floor(next));
}

function cleanCoinText(value: string): string {
  return value.replace(/[^\d]/g, '').replace(/^0+(?=\d)/, '');
}
