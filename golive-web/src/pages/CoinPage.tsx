import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
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

const DAILY_TASKS = [
  {
    id: 'daily-login-lottery',
    title: '每日登录抽奖',
    description: '每天抽一次，小额鼓励不伤钱包。',
    reward: '6-18',
    targetLabel: '今日可抽',
    target: 1,
    kind: 'login',
  },
  {
    id: 'watch-3-lives',
    title: '观看 3 个直播间',
    description: '当天打开 3 个不同直播间后领取。',
    reward: '18',
    targetLabel: '3 个直播间',
    target: 3,
    kind: 'rooms',
  },
  {
    id: 'watch-30-minutes',
    title: '累计观看 30 分钟',
    description: '认真看一会儿，给一点点 coins。',
    reward: '25',
    targetLabel: '30 分钟',
    target: 30 * 60,
    kind: 'time',
  },
] as const;

const FILTERS: Array<{ id: RecordFilter; label: string }> = [
  { id: 'all', label: '全部' },
  { id: 'income', label: '获得' },
  { id: 'spend', label: '消费' },
  { id: 'recharge', label: '充值' },
  { id: 'gift', label: '礼物' },
  { id: 'sc', label: 'SC' },
  { id: 'bet', label: '竞猜' },
  { id: 'task', label: '任务' },
  { id: 'creator', label: '直播收入' },
];

export default function CoinPage() {
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
      toast.error('充值至少 1,000 coins。');
      return;
    }
    topup.mutate(
      { amount: topupAmount },
      {
        onSuccess: (next) => {
          toast.success(`充值成功，当前余额 ${next.coinBalance.toLocaleString()} coins。`);
        },
        onError: (err) => toast.error(err.message || '充值失败。'),
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
      toast.info('任务还没完成，完成后就能领取。');
      return;
    }
    setClaimingTaskId(task.id);
    claimTask.mutate(
      { taskId: task.id },
      {
        onSuccess: (resp) => {
          if (resp.alreadyClaimed) {
            toast.info('今天已经领取过这个任务。');
            return;
          }
          toast.success(`已获得 ${resp.transaction.amount.toLocaleString()} coins。`);
        },
        onError: (err) => toast.error(err.message || '领取失败。'),
        onSettled: () => setClaimingTaskId(null),
      },
    );
  };

  const handleWithdrawPreview = () => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    toast.info('提现支付接口预留中，当前不会扣除 coins。');
  };

  return (
    <div className="gl-page gl-coin-page">
      {!isAuthed && (
        <div className="gl-yt-banner">
          <div>
            <strong>登录后管理 coins</strong>
            <span>查看消费记录、充值、每日任务和提现预留信息。</span>
          </div>
          <button className="gl-secondary-btn" type="button" onClick={() => openLogin()}>
            登录
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
            {currentUser ? userDisplayName(currentUser) : '游客'} 的 coin 账户， 充值比例为 1,000
            coins = 1 人民币。
          </p>
        </div>
        <div className="gl-coin-hero-actions">
          <button
            type="button"
            className="gl-coin-primary"
            onClick={() => rechargeRef.current?.scrollIntoView({ behavior: 'smooth' })}
          >
            <CreditCard size={16} />
            去充值
          </button>
          <Link className="gl-secondary-btn" to="/history">
            <History size={15} />
            观看记录
          </Link>
        </div>
      </section>

      <section className="gl-coin-stats" aria-label="Coin summary">
        <StatPill icon={<Coins size={18} />} value={formatCoins(balance)} label="当前余额" />
        <StatPill
          icon={<ArrowUpRight size={18} />}
          value={formatCoins(monthSpend)}
          label="本月消费"
        />
        <StatPill
          icon={<ArrowDownLeft size={18} />}
          value={formatCoins(monthIncome)}
          label="本月获得"
        />
        <StatPill icon={<Radio size={18} />} value={formatCoins(creatorIncome)} label="直播收入" />
      </section>

      <div className="gl-coin-layout">
        <div className="gl-coin-main">
          <section className="gl-coin-panel">
            <div className="gl-section-title-row">
              <h2>每日任务</h2>
              <span className="gl-coin-note">奖励偏小，控制成本</span>
            </div>
            <div className="gl-coin-task-grid">
              {DAILY_TASKS.map((task) => {
                const progress = taskProgress(task.kind, activity);
                const claimed = claimedByTask.get(task.id);
                return (
                  <TaskCard
                    key={task.id}
                    title={task.title}
                    description={task.description}
                    reward={claimed ? String(claimed.amount) : task.reward}
                    progress={progress}
                    target={task.target}
                    targetLabel={task.targetLabel}
                    claimed={Boolean(claimed)}
                    pending={claimingTaskId === task.id && claimTask.isPending}
                    onClaim={() => handleClaim(task, progress)}
                  />
                );
              })}
            </div>
          </section>

          <section className="gl-coin-panel">
            <div className="gl-section-title-row">
              <h2>Coin 流水</h2>
              <span className="gl-coin-note">礼物、SC、竞猜、充值、任务都会记录</span>
            </div>
            <div className="gl-coin-filters" role="tablist" aria-label="Coin record filters">
              {FILTERS.map((item) => (
                <button
                  key={item.id}
                  type="button"
                  className={cn('gl-coin-filter', filter === item.id && 'is-active')}
                  onClick={() => setFilter(item.id)}
                >
                  {item.label}
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
                <strong>暂无 coin 记录</strong>
                <span>开始充值、送礼物、发 SC 或参与竞猜后会出现在这里。</span>
              </div>
            ) : (
              <div className="gl-coin-record-list">
                {filteredRows.map((item) => (
                  <CoinRecordRow key={item.id} item={item} />
                ))}
              </div>
            )}
          </section>
        </div>

        <aside className="gl-coin-side">
          <section className="gl-coin-panel" ref={rechargeRef}>
            <div className="gl-coin-panel-head">
              <div>
                <h2>充值</h2>
                <p>1,000 coins 起充，上不封顶。</p>
              </div>
              <CreditCard size={20} />
            </div>
            <label className="gl-coin-input-label">
              <span>充值数量</span>
              <div className="gl-coin-input">
                <input
                  inputMode="numeric"
                  value={topupText}
                  onChange={(event) => setTopupText(cleanCoinText(event.target.value))}
                  onBlur={() => {
                    if (topupAmount < 1000) setTopupText('1000');
                  }}
                  aria-label="充值 coins 数量"
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
              <span>需支付</span>
              <strong>{formatRmb(topupAmount)}</strong>
            </div>
            <button
              type="button"
              className="gl-coin-primary gl-coin-wide"
              onClick={handleTopup}
              disabled={topup.isPending}
            >
              <CreditCard size={16} />
              {topup.isPending ? '充值中...' : '立即充值'}
            </button>
            <p className="gl-coin-footnote">
              当前为开发模式：点击后直接充值成功，后续接入支付接口。
            </p>
          </section>

          <section className="gl-coin-panel">
            <div className="gl-coin-panel-head">
              <div>
                <h2>提现预留</h2>
                <p>手续费 35%，支付接口接入后启用。</p>
              </div>
              <Wallet size={20} />
            </div>
            <label className="gl-coin-input-label">
              <span>提现数量</span>
              <div className="gl-coin-input">
                <input
                  inputMode="numeric"
                  value={withdrawText}
                  onChange={(event) => setWithdrawText(cleanCoinText(event.target.value))}
                  aria-label="提现 coins 数量"
                />
                <span>coins</span>
              </div>
            </label>
            <div className="gl-coin-withdraw-lines">
              <span>
                手续费 <strong>{formatCoins(withdrawFee)}</strong>
              </span>
              <span>
                预计到账 <strong>{formatCoins(withdrawNet)}</strong>
              </span>
              <span>
                折合人民币 <strong>{formatRmb(withdrawNet)}</strong>
              </span>
            </div>
            <button
              type="button"
              className="gl-secondary-btn gl-coin-wide"
              onClick={handleWithdrawPreview}
            >
              预览提现
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
        <span>{formatProgress(progress, target, targetLabel)}</span>
        <button
          type="button"
          onClick={onClaim}
          disabled={claimed || pending || !complete}
          className={cn(complete && !claimed ? 'is-ready' : '')}
        >
          {claimed ? '已领取' : pending ? '领取中...' : '领取'}
        </button>
      </div>
    </article>
  );
}

function CoinRecordRow({ item }: { item: CoinTransaction }) {
  const meta = recordMeta(item.type);
  const Icon = meta.icon;
  const positive = item.amount > 0;
  return (
    <article className="gl-coin-record">
      <div className={cn('gl-coin-record-icon', positive ? 'is-income' : 'is-spend')}>
        <Icon size={17} />
      </div>
      <div className="gl-coin-record-main">
        <strong>{item.title || meta.label}</strong>
        <span>
          {formatTime(item.createdAt)}
          {item.description ? ` · ${item.description}` : ` · ${meta.label}`}
        </span>
      </div>
      <div className={cn('gl-coin-record-amount', positive ? 'is-income' : 'is-spend')}>
        <strong>
          {positive ? '+' : ''}
          {item.amount.toLocaleString()}
        </strong>
        <span>余额 {item.balanceAfter.toLocaleString()}</span>
      </div>
    </article>
  );
}

function recordMeta(type: CoinTransactionType) {
  switch (type) {
    case 'topup':
      return { label: '充值获得', icon: CreditCard };
    case 'daily_task':
      return { label: '每日任务', icon: CalendarCheck };
    case 'gift_spend':
    case 'creator_gift_income':
      return { label: '礼物', icon: Gift };
    case 'super_chat_spend':
    case 'creator_super_chat_income':
      return { label: 'SuperChat', icon: MessageSquareText };
    case 'bet_wager':
    case 'bet_payout':
    case 'bet_refund':
      return { label: '竞猜', icon: Trophy };
    default:
      return { label: 'Coin', icon: Coins };
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

function formatProgress(progress: number, target: number, targetLabel: string): string {
  if (target === 1) return targetLabel;
  if (target >= 60) {
    return `${Math.min(Math.floor(progress / 60), target / 60)} / ${target / 60} 分钟`;
  }
  return `${Math.min(progress, target)} / ${targetLabel}`;
}

function sum(rows: CoinTransaction[]): number {
  return rows.reduce((acc, item) => acc + item.amount, 0);
}

function formatCoins(value: number): string {
  return `${Math.round(value).toLocaleString()} coins`;
}

function formatRmb(coins: number): string {
  const rmb = Math.max(0, coins) / 1000;
  return `¥${rmb.toLocaleString('zh-CN', { maximumFractionDigits: 2 })}`;
}

function formatTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat('zh-CN', {
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
