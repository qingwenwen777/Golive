import type { ReactNode } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ArrowLeft,
  BarChart3,
  CalendarDays,
  Crown,
  Coins,
  Eye,
  Radio,
  TrendingUp,
  UserPlus,
  Users,
} from 'lucide-react';
import {
  useCreatorAnalytics,
  useLiveAnalysis,
  type LiveHistoryItem,
  type MonthlyCreatorMetric,
} from '@/api/room';
import { Avatar } from '@/components/Avatar';

export function CreatorAnalyticsPage() {
  const { name = '' } = useParams<{ name: string }>();
  const channelKey = decodeURIComponent(name);
  const analytics = useCreatorAnalytics(channelKey);
  const data = analytics.data;
  const latest = data?.history[0];

  return (
    <div className="gl-page gl-studio-page">
      <StudioHeader
        channelKey={channelKey}
        title="Channel analytics"
        subtitle="Revenue, subscriptions, watch time, and finished live performance."
      />

      {analytics.isPending ? (
        <div className="gl-studio-panel gl-studio-loading">Loading analytics...</div>
      ) : analytics.isError ? (
        <div className="gl-channel-empty">
          <BarChart3 size={34} />
          <div>
            <strong>Analytics unavailable</strong>
            <span>Only the channel owner can view studio analytics.</span>
          </div>
        </div>
      ) : data ? (
        <>
          <section className="gl-studio-kpis" aria-label="Creator performance summary">
            <StudioKpi
              icon={<Coins size={18} />}
              label="Revenue"
              value={formatCoin(data.revenueCoin)}
              trend="last 6 months"
            />
            <StudioKpi
              icon={<UserPlus size={18} />}
              label="Subscribers"
              value={data.subscriberCount.toLocaleString()}
              trend="current total"
            />
            <StudioKpi
              icon={<Radio size={18} />}
              label="Streams"
              value={data.streams.toLocaleString()}
              trend="completed"
            />
            <StudioKpi
              icon={<Eye size={18} />}
              label="Peak online"
              value={data.peakViewers.toLocaleString()}
              trend="best live"
            />
          </section>

          <section className="gl-studio-grid">
            <div className="gl-studio-panel gl-studio-panel-wide">
              <div className="gl-studio-panel-head">
                <div>
                  <span>Monthly revenue</span>
                  <h2>{formatCoin(data.monthly.at(-1)?.revenueCoin ?? 0)}</h2>
                </div>
                <BarChart3 size={22} />
              </div>
              <BarChart
                items={data.monthly}
                value={(item) => item.revenueCoin}
                label={(item) => formatMonth(item.month)}
                valueLabel={formatCoin}
              />
            </div>

            <div className="gl-studio-panel">
              <div className="gl-studio-panel-head">
                <div>
                  <span>Subscription growth</span>
                  <h2>{data.monthly.at(-1)?.subscribers.toLocaleString() ?? '0'}</h2>
                </div>
                <TrendingUp size={22} />
              </div>
              <LineChart items={data.monthly} value={(item) => item.subscribers} />
            </div>

            <div className="gl-studio-panel">
              <div className="gl-studio-panel-head">
                <div>
                  <span>Watch time</span>
                  <h2>{data.watchHours.toLocaleString()}h</h2>
                </div>
                <Users size={22} />
              </div>
              <MetricRows items={data.monthly} />
            </div>
          </section>

          <section className="gl-studio-panel">
            <div className="gl-studio-panel-head">
              <div>
                <span>Recent live performance</span>
                <h2>History</h2>
              </div>
              <Link
                className="gl-text-link"
                to={`/channel/${encodeURIComponent(channelKey)}#history`}
              >
                View channel
              </Link>
            </div>
            <div className="gl-studio-live-table">
              {data.history.slice(0, 8).map((record) => (
                <LiveHistoryStudioRow key={record.id} record={record} channelKey={channelKey} />
              ))}
              {data.history.length === 0 && (
                <div className="gl-yt-shelf-empty">No finished live streams yet.</div>
              )}
            </div>
          </section>

          {latest && (
            <section className="gl-studio-latest">
              <div>
                <span>Last stream</span>
                <strong>{latest.title}</strong>
                <small>
                  {formatDate(latest.startedAt)} · {latest.duration}
                </small>
              </div>
              <Link
                className="gl-retry-btn"
                to={`/studio/analytics/${encodeURIComponent(channelKey)}/live/${encodeURIComponent(latest.id)}`}
              >
                <BarChart3 size={16} />
                Open analysis
              </Link>
            </section>
          )}
        </>
      ) : null}
    </div>
  );
}

export function LiveAnalysisPage() {
  const navigate = useNavigate();
  const { name = '', recordId = '' } = useParams<{ name: string; recordId: string }>();
  const channelKey = decodeURIComponent(name);
  const analysis = useLiveAnalysis(channelKey, decodeURIComponent(recordId));
  const data = analysis.data;
  const record = data?.record;

  return (
    <div className="gl-page gl-studio-page">
      <button className="gl-studio-back" type="button" onClick={() => navigate(-1)}>
        <ArrowLeft size={18} />
        Back
      </button>
      <StudioHeader
        channelKey={channelKey}
        title={record?.title ?? 'Live analysis'}
        subtitle="Single-stream revenue, peak audience, and top fan contribution."
      />

      {analysis.isPending ? (
        <div className="gl-studio-panel gl-studio-loading">Loading live analysis...</div>
      ) : analysis.isError || !record ? (
        <div className="gl-channel-empty">
          <BarChart3 size={34} />
          <div>
            <strong>Live analysis unavailable</strong>
            <span>Only the channel owner can view this report.</span>
          </div>
        </div>
      ) : (
        <>
          <section className="gl-studio-live-hero">
            <HistoryCover record={record} />
            <div className="gl-studio-live-copy">
              <div className="gl-studio-live-date">
                <CalendarDays size={16} />
                {formatDate(record.startedAt)} · {record.duration}
              </div>
              <div className="gl-studio-kpis compact">
                <StudioKpi
                  icon={<Coins size={18} />}
                  label="Revenue"
                  value={formatCoin(record.revenueCoin)}
                  trend="this live"
                />
                <StudioKpi
                  icon={<Eye size={18} />}
                  label="Peak online"
                  value={record.peakViewers.toLocaleString()}
                  trend="highest"
                />
                <StudioKpi
                  icon={<UserPlus size={18} />}
                  label="New subscribers"
                  value={record.newSubscribers.toLocaleString()}
                  trend="during live"
                />
                <StudioKpi
                  icon={<Crown size={18} />}
                  label="Top fan"
                  value={record.topFan?.name ?? '-'}
                  trend={formatCoin(record.topFan?.amount ?? 0)}
                />
              </div>
            </div>
          </section>

          <section className="gl-studio-grid">
            <div className="gl-studio-panel gl-studio-panel-wide">
              <div className="gl-studio-panel-head">
                <div>
                  <span>Revenue breakdown</span>
                  <h2>{formatCoin(record.revenueCoin)}</h2>
                </div>
                <Coins size={22} />
              </div>
              <BreakdownBars gift={data.giftRevenue} superChat={data.superChatRevenue} />
            </div>

            <div className="gl-studio-panel">
              <div className="gl-studio-panel-head">
                <div>
                  <span>Top contributions</span>
                  <h2>{data.topFans.length} fans</h2>
                </div>
                <Crown size={22} />
              </div>
              <div className="gl-studio-fans">
                {data.topFans.map((fan) => (
                  <div className="gl-studio-fan-row" key={fan.userId || fan.name}>
                    <Avatar name={fan.name} src={fan.avatar} size={34} />
                    <span>{fan.name}</span>
                    <strong>{formatCoin(fan.amount)}</strong>
                  </div>
                ))}
                {data.topFans.length === 0 && (
                  <div className="gl-yt-shelf-empty">No contribution data for this live.</div>
                )}
              </div>
            </div>
          </section>
        </>
      )}
    </div>
  );
}

function StudioHeader({
  channelKey,
  title,
  subtitle,
}: {
  channelKey: string;
  title: string;
  subtitle: string;
}) {
  return (
    <header className="gl-studio-head">
      <div className="gl-studio-head-icon">
        <BarChart3 size={26} />
      </div>
      <div>
        <span>GoLive Studio</span>
        <h1>{title}</h1>
        <Link to={`/channel/${encodeURIComponent(channelKey)}`}>{subtitle}</Link>
      </div>
    </header>
  );
}

function StudioKpi({
  icon,
  label,
  value,
  trend,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  trend: string;
}) {
  return (
    <div className="gl-studio-kpi">
      <div className="gl-studio-kpi-icon">{icon}</div>
      <span>{label}</span>
      <strong>{value}</strong>
      <small>{trend}</small>
    </div>
  );
}

function LiveHistoryStudioRow({
  record,
  channelKey,
}: {
  record: LiveHistoryItem;
  channelKey: string;
}) {
  return (
    <Link
      className="gl-studio-live-row"
      to={`/studio/analytics/${encodeURIComponent(channelKey)}/live/${encodeURIComponent(record.id)}`}
    >
      <HistoryCover record={record} />
      <div>
        <strong>{record.title}</strong>
        <span>
          {formatDate(record.startedAt)} · {record.duration}
        </span>
      </div>
      <span>{formatCoin(record.revenueCoin)}</span>
      <span>{record.peakViewers.toLocaleString()} peak</span>
    </Link>
  );
}

function HistoryCover({ record }: { record: LiveHistoryItem }) {
  return (
    <div className="gl-history-cover">
      <div className="gl-history-cover-fallback" aria-hidden>
        {(record.title || 'GL').slice(0, 2).toUpperCase()}
      </div>
      {record.cover && <img src={record.cover} alt="" />}
      <span>{record.duration}</span>
    </div>
  );
}

function BarChart({
  items,
  value,
  label,
  valueLabel,
}: {
  items: MonthlyCreatorMetric[];
  value: (item: MonthlyCreatorMetric) => number;
  label: (item: MonthlyCreatorMetric) => string;
  valueLabel: (value: number) => string;
}) {
  const max = Math.max(...items.map(value), 1);
  return (
    <div className="gl-studio-bars">
      {items.map((item) => {
        const raw = value(item);
        return (
          <div className="gl-studio-bar" key={item.month}>
            <span
              style={{ height: `${Math.max(8, (raw / max) * 100)}%` }}
              title={valueLabel(raw)}
            />
            <small>{label(item)}</small>
          </div>
        );
      })}
    </div>
  );
}

function LineChart({
  items,
  value,
}: {
  items: MonthlyCreatorMetric[];
  value: (item: MonthlyCreatorMetric) => number;
}) {
  const values = items.map(value);
  const max = Math.max(...values, 1);
  const points = values
    .map((item, index) => {
      const x = values.length === 1 ? 0 : (index / (values.length - 1)) * 100;
      const y = 100 - (item / max) * 82 - 8;
      return `${x},${y}`;
    })
    .join(' ');

  return (
    <svg className="gl-studio-line" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden>
      <polyline points={points} />
    </svg>
  );
}

function MetricRows({ items }: { items: MonthlyCreatorMetric[] }) {
  return (
    <div className="gl-studio-metric-rows">
      {items.slice(-4).map((item) => (
        <div key={item.month}>
          <span>{formatMonth(item.month)}</span>
          <strong>{item.watchHours.toLocaleString()}h</strong>
          <small>{item.streams} streams</small>
        </div>
      ))}
    </div>
  );
}

function BreakdownBars({ gift, superChat }: { gift: number; superChat: number }) {
  const total = Math.max(1, gift + superChat);
  return (
    <div className="gl-studio-breakdown">
      <div>
        <span>Gifts</span>
        <strong>{formatCoin(gift)}</strong>
        <i style={{ width: `${(gift / total) * 100}%` }} />
      </div>
      <div>
        <span>Super Chat</span>
        <strong>{formatCoin(superChat)}</strong>
        <i style={{ width: `${(superChat / total) * 100}%` }} />
      </div>
    </div>
  );
}

function formatCoin(value: number): string {
  return `${Math.round(value).toLocaleString()} coins`;
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value));
}

function formatMonth(value: string): string {
  const [year, month] = value.split('-').map(Number);
  if (!year || !month) return value;
  return new Intl.DateTimeFormat(undefined, { month: 'short' }).format(
    new Date(year, month - 1, 1),
  );
}
