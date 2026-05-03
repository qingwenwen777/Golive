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
  useClaimDailyCoinTask,
  useCoinTransactions,
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

const QUICK_TOPUPS = [1000, 5000, 10000, 50000] as const;
const WITHDRAW_FEE_RATE = 0.35;
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
  const transactions = useCoinTransactions();
  const topup = useTopupCoins();
  const claimTask = useClaimDailyCoinTask();

  const [topupText, setTopupText] = useState('1000');
  const [withdrawText, setWithdrawText] = useState('10000');
  const [filter, setFilter] = useState<RecordFilter>('all');
  const [recordPage, setRecordPage] = useState(1);
  const [claimingTaskId, setClaimingTaskId] = useState<string | null>(null);
  const [activity, setActivity] = useState(() => readDailyCoinActivity(user?.id));

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
  const withdrawFee = Math.floor(withdrawAmount * WITHDRAW_FEE_RATE);
  const withdrawNet = Math.max(0, withdrawAmount - withdrawFee);

  const handleTopup = () => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (topupAmount < 1000) {
      toast.error(t('coin.toast.minTopup', { defaultValue: 'Top-up must be at least 1,000 coins.' }));
      return;
    }
    topup.mutate(
      { amount: topupAmount },
      {
        onSuccess: (next) => {
          toast.success(
            t('coin.toast.topupSuccess', {
              balance: next.coinBalance.toLocaleString(),
              defaultValue: 'Top-up complete. Current balance: {{balance}} coins.',
            }),
          );
        },
        onError: (err) =>
          toast.error(err.message || t('coin.toast.topupFailed', { defaultValue: 'Top-up failed.' })),
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
      toast.info(t('coin.toast.taskIncomplete', { defaultValue: 'Finish the task before claiming.' }));
      return;
    }
    setClaimingTaskId(task.id);
    claimTask.mutate(
      { taskId: task.id },
      {
        onSuccess: (resp) => {
          if (resp.alreadyClaimed) {
            toast.info(t('coin.toast.alreadyClaimed', { defaultValue: 'You already claimed this task today.' }));
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
          toast.error(err.message || t('coin.toast.claimFailed', { defaultValue: 'Claim failed.' })),
        onSettled: () => setClaimingTaskId(null),
      },
    );
  };

  const handleWithdrawPreview = () => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    toast.info(
      t('coin.toast.withdrawPreview', {
        defaultValue:
          'Withdrawals are reserved for payment integration and no coins will be deducted.',
      }),
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
                defaultValue: 'View spending records, top up, daily tasks, and withdrawal previews.',
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
              defaultValue: '{{name}} coin account. Top-up rate: 1,000 coins = 1 RMB.',
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
                <p>{t('coin.topup.sub', { defaultValue: 'Minimum 1,000 coins. No upper limit.' })}</p>
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
                    if (topupAmount < 1000) setTopupText('1000');
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
              <strong>{formatRmb(topupAmount, i18n.language)}</strong>
            </div>
            <button
              type="button"
              className="gl-coin-primary gl-coin-wide"
              onClick={handleTopup}
              disabled={topup.isPending}
            >
              <CreditCard size={16} />
              {topup.isPending
                ? t('coin.topup.pending', { defaultValue: 'Topping up...' })
                : t('coin.topup.submit', { defaultValue: 'Top up now' })}
            </button>
          </section>

          <section className="gl-coin-panel">
            <div className="gl-coin-panel-head">
              <div>
                <h2>{t('coin.withdraw.title', { defaultValue: 'Withdrawal preview' })}</h2>
                <p>
                  {t('coin.withdraw.sub', {
                    defaultValue: '35% fee. Enabled after payment integration.',
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
                  aria-label={t('coin.withdraw.amountAria', { defaultValue: 'Withdrawal coin amount' })}
                />
                <span>coins</span>
              </div>
            </label>
            <div className="gl-coin-withdraw-lines">
              <span>
                {t('coin.withdraw.fee', { defaultValue: 'Fee' })}{' '}
                <strong>{formatCoins(withdrawFee)}</strong>
              </span>
              <span>
                {t('coin.withdraw.net', { defaultValue: 'Estimated arrival' })}{' '}
                <strong>{formatCoins(withdrawNet)}</strong>
              </span>
              <span>
                {t('coin.withdraw.rmb', { defaultValue: 'Approx. RMB' })}{' '}
                <strong>{formatRmb(withdrawNet, i18n.language)}</strong>
              </span>
            </div>
            <button
              type="button"
              className="gl-secondary-btn gl-coin-wide"
              onClick={handleWithdrawPreview}
            >
              {t('coin.withdraw.preview', { defaultValue: 'Preview withdrawal' })}
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
    case 'daily_task':
      return { labelKey: 'coin.record.dailyTask', labelDefault: 'Daily task', icon: CalendarCheck };
    case 'gift_spend':
    case 'creator_gift_income':
      return { labelKey: 'coin.record.gift', labelDefault: 'Gift', icon: Gift };
    case 'super_chat_spend':
    case 'creator_super_chat_income':
      return { labelKey: 'coin.record.superChat', labelDefault: 'Super Chat', icon: MessageSquareText };
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

function formatRmb(coins: number, locale: string): string {
  const rmb = Math.max(0, coins) / 1000;
  return `¥${rmb.toLocaleString(locale, { maximumFractionDigits: 2 })}`;
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
