import type { ReactNode } from 'react';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ArrowLeft,
  BarChart3,
  CalendarDays,
  Crown,
  Coins,
  Eye,
  MessageSquare,
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
import { LoadableImage } from '@/components/LoadableImage';

export function CreatorAnalyticsPage() {
  const { t, i18n } = useTranslation('pages');
  const { name = '' } = useParams<{ name: string }>();
  const channelKey = decodeURIComponent(name);
  const analytics = useCreatorAnalytics(channelKey);
  const data = analytics.data;
  const latest = data?.history[0];
  const locale = i18n.resolvedLanguage ?? i18n.language;

  return (
    <div className="gl-page gl-studio-page">
      <StudioHeader
        channelKey={channelKey}
        title={t('studio.analytics.title')}
        subtitle={t('studio.analytics.subtitle')}
      />

      {analytics.isPending ? (
        <div className="gl-studio-panel gl-studio-loading">{t('studio.analytics.loading')}</div>
      ) : analytics.isError ? (
        <div className="gl-channel-empty">
          <BarChart3 size={34} />
          <div>
            <strong>{t('studio.analytics.unavailableTitle')}</strong>
            <span>{t('studio.analytics.unavailableBody')}</span>
          </div>
        </div>
      ) : data ? (
        <>
          <section className="gl-studio-kpis" aria-label={t('studio.analytics.kpis')}>
            <StudioKpi
              tone="revenue"
              icon={<Coins size={18} />}
              label={t('studio.analytics.cards.revenue')}
              value={formatCoin(data.revenueCoin, locale, t)}
              trend={t('studio.analytics.cards.revenueSub')}
            />
            <StudioKpi
              tone="growth"
              icon={<UserPlus size={18} />}
              label={t('studio.analytics.cards.subscribers')}
              value={data.subscriberCount.toLocaleString(locale)}
              trend={t('studio.analytics.cards.subscribersSub')}
            />
            <StudioKpi
              tone="activity"
              icon={<Radio size={18} />}
              label={t('studio.analytics.cards.streams')}
              value={data.streams.toLocaleString(locale)}
              trend={t('studio.analytics.cards.streamsSub')}
            />
            <StudioKpi
              tone="attention"
              icon={<Eye size={18} />}
              label={t('studio.analytics.cards.peakOnline')}
              value={data.peakViewers.toLocaleString(locale)}
              trend={t('studio.analytics.cards.peakOnlineSub')}
            />
          </section>

          <section className="gl-studio-grid">
            <div className="gl-studio-panel gl-studio-panel-wide">
              <div className="gl-studio-panel-head">
                <div>
                  <span>{t('studio.analytics.sections.monthlyRevenue')}</span>
                  <h2>{formatCoin(data.monthly.at(-1)?.revenueCoin ?? 0, locale, t)}</h2>
                </div>
                <BarChart3 size={22} />
              </div>
              <BarChart
                tone="revenue"
                items={data.monthly}
                value={(item) => item.revenueCoin}
                label={(item) => formatMonth(item.month, locale)}
                valueLabel={(value) => formatCoin(value, locale, t)}
              />
            </div>

            <div className="gl-studio-panel">
              <div className="gl-studio-panel-head">
                <div>
                  <span>{t('studio.analytics.sections.subscriptionGrowth')}</span>
                  <h2>{data.monthly.at(-1)?.subscribers.toLocaleString(locale) ?? '0'}</h2>
                </div>
                <TrendingUp size={22} />
              </div>
              <LineChart tone="growth" items={data.monthly} value={(item) => item.subscribers} />
            </div>

            <div className="gl-studio-panel">
              <div className="gl-studio-panel-head">
                <div>
                  <span>{t('studio.analytics.sections.watchTime')}</span>
                  <h2>{formatHours(data.watchHours, locale, t)}</h2>
                </div>
                <Users size={22} />
              </div>
              <MetricRows items={data.monthly} locale={locale} t={t} />
            </div>
          </section>

          <section className="gl-studio-panel">
            <div className="gl-studio-panel-head">
              <div>
                <span>{t('studio.analytics.sections.recentPerformance')}</span>
                <h2>{t('studio.analytics.sections.history')}</h2>
              </div>
              <Link
                className="gl-text-link"
                to={`/channel/${encodeURIComponent(channelKey)}#history`}
              >
                {t('studio.analytics.sections.viewChannel')}
              </Link>
            </div>
            <div className="gl-studio-live-table">
              {data.history.slice(0, 8).map((record) => (
                <LiveHistoryStudioRow
                  key={record.id}
                  record={record}
                  channelKey={channelKey}
                  locale={locale}
                  t={t}
                />
              ))}
              {data.history.length === 0 && (
                <div className="gl-yt-shelf-empty">
                  {t('studio.analytics.sections.emptyHistory')}
                </div>
              )}
            </div>
          </section>

          {latest && (
            <section className="gl-studio-latest">
              <div>
                <span>{t('studio.analytics.sections.lastStream')}</span>
                <strong>{latest.title}</strong>
                <small>
                  {formatDate(latest.startedAt, locale)} · {latest.duration}
                </small>
              </div>
              <Link
                className="gl-retry-btn"
                to={`/studio/analytics/${encodeURIComponent(channelKey)}/live/${encodeURIComponent(latest.id)}`}
              >
                <BarChart3 size={16} />
                {t('studio.analytics.sections.openAnalysis')}
              </Link>
            </section>
          )}
        </>
      ) : null}
    </div>
  );
}

export function LiveAnalysisPage() {
  const { t, i18n } = useTranslation('pages');
  const navigate = useNavigate();
  const { name = '', recordId = '' } = useParams<{ name: string; recordId: string }>();
  const channelKey = decodeURIComponent(name);
  const analysis = useLiveAnalysis(channelKey, decodeURIComponent(recordId));
  const data = analysis.data;
  const record = data?.record;
  const locale = i18n.resolvedLanguage ?? i18n.language;

  return (
    <div className="gl-page gl-studio-page">
      <button className="gl-studio-back" type="button" onClick={() => navigate(-1)}>
        <ArrowLeft size={18} />
        {t('studio.analytics.live.back')}
      </button>
      <StudioHeader
        channelKey={channelKey}
        title={record?.title ?? t('studio.analytics.live.title')}
        subtitle={t('studio.analytics.live.subtitle')}
      />

      {analysis.isPending ? (
        <div className="gl-studio-panel gl-studio-loading">
          {t('studio.analytics.live.loading')}
        </div>
      ) : analysis.isError || !record ? (
        <div className="gl-channel-empty">
          <BarChart3 size={34} />
          <div>
            <strong>{t('studio.analytics.live.unavailableTitle')}</strong>
            <span>{t('studio.analytics.live.unavailableBody')}</span>
          </div>
        </div>
      ) : (
        <>
          <section className="gl-studio-live-hero">
            <HistoryCover record={record} />
            <div className="gl-studio-live-copy">
              <div className="gl-studio-live-date">
                <CalendarDays size={16} />
                {formatDate(record.startedAt, locale)} · {record.duration}
              </div>
              <div className="gl-studio-kpis compact">
                <StudioKpi
                  tone="revenue"
                  icon={<Coins size={18} />}
                  label={t('studio.analytics.cards.revenue')}
                  value={formatCoin(record.revenueCoin, locale, t)}
                  trend={t('studio.analytics.live.cards.revenueSub')}
                />
                <StudioKpi
                  tone="attention"
                  icon={<Eye size={18} />}
                  label={t('studio.analytics.cards.peakOnline')}
                  value={record.peakViewers.toLocaleString(locale)}
                  trend={t('studio.analytics.live.cards.peakOnlineSub')}
                />
                <StudioKpi
                  tone="growth"
                  icon={<UserPlus size={18} />}
                  label={t('studio.analytics.live.cards.newSubscribers')}
                  value={record.newSubscribers.toLocaleString(locale)}
                  trend={t('studio.analytics.live.cards.newSubscribersSub')}
                />
                <StudioKpi
                  tone="activity"
                  icon={<MessageSquare size={18} />}
                  label={t('studio.analytics.live.cards.danmu')}
                  value={(record.danmuCount ?? 0).toLocaleString(locale)}
                  trend={t('studio.analytics.units.danmuCount', {
                    count: record.danmuCount ?? 0,
                    formattedCount: (record.danmuCount ?? 0).toLocaleString(locale),
                  })}
                />
                <StudioKpi
                  tone="revenue"
                  icon={<Crown size={18} />}
                  label={t('studio.analytics.live.cards.topFan')}
                  value={record.topFan?.name ?? '-'}
                  trend={formatCoin(record.topFan?.amount ?? 0, locale, t)}
                />
              </div>
            </div>
          </section>

          <section className="gl-studio-grid">
            <div className="gl-studio-panel gl-studio-panel-wide">
              <div className="gl-studio-panel-head">
                <div>
                  <span>{t('studio.analytics.sections.revenueBreakdown')}</span>
                  <h2>{formatCoin(record.revenueCoin, locale, t)}</h2>
                </div>
                <Coins size={22} />
              </div>
              <BreakdownBars
                gift={data.giftRevenue}
                superChat={data.superChatRevenue}
                locale={locale}
                t={t}
              />
            </div>

            <div className="gl-studio-panel">
              <div className="gl-studio-panel-head">
                <div>
                  <span>{t('studio.analytics.sections.topContributions')}</span>
                  <h2>
                    {t('studio.analytics.units.fanCount', {
                      count: data.topFans.length,
                      formattedCount: data.topFans.length.toLocaleString(locale),
                    })}
                  </h2>
                </div>
                <Crown size={22} />
              </div>
              <div className="gl-studio-fans">
                {data.topFans.map((fan) => (
                  <div className="gl-studio-fan-row" key={fan.userId || fan.name}>
                    <Avatar name={fan.name} src={fan.avatar} size={34} />
                    <span>{fan.name}</span>
                    <strong>{formatCoin(fan.amount, locale, t)}</strong>
                  </div>
                ))}
                {data.topFans.length === 0 && (
                  <div className="gl-yt-shelf-empty">
                    {t('studio.analytics.sections.emptyContributions')}
                  </div>
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
  const { t } = useTranslation('pages');

  return (
    <header className="gl-studio-head">
      <div className="gl-studio-head-icon">
        <BarChart3 size={26} />
      </div>
      <div>
        <span>{t('studio.brand')}</span>
        <h1>{title}</h1>
        <Link to={`/channel/${encodeURIComponent(channelKey)}`}>{subtitle}</Link>
      </div>
    </header>
  );
}

function StudioKpi({
  tone = 'neutral',
  icon,
  label,
  value,
  trend,
}: {
  tone?: 'revenue' | 'growth' | 'activity' | 'attention' | 'neutral';
  icon: ReactNode;
  label: string;
  value: string;
  trend: string;
}) {
  return (
    <div className={`gl-studio-kpi is-${tone}`}>
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
  locale,
  t,
}: {
  record: LiveHistoryItem;
  channelKey: string;
  locale: string;
  t: TFunction;
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
          {formatDate(record.startedAt, locale)} · {record.duration}
        </span>
      </div>
      <span>{formatCoin(record.revenueCoin, locale, t)}</span>
      <span>
        {t('studio.analytics.units.peakViewers', {
          amount: record.peakViewers.toLocaleString(locale),
        })}
      </span>
      <span>
        {t('studio.analytics.units.danmuCount', {
          count: record.danmuCount ?? 0,
          formattedCount: (record.danmuCount ?? 0).toLocaleString(locale),
        })}
      </span>
    </Link>
  );
}

function HistoryCover({ record }: { record: LiveHistoryItem }) {
  return (
    <div className={`gl-history-cover${record.cover ? 'has-image' : ''}`}>
      <div className="gl-history-cover-fallback" aria-hidden>
        {(record.title || 'GL').slice(0, 2).toUpperCase()}
      </div>
      {record.cover && <LoadableImage src={record.cover} alt="" />}
      <span>{record.duration}</span>
    </div>
  );
}

function BarChart({
  tone = 'neutral',
  items,
  value,
  label,
  valueLabel,
}: {
  tone?: 'revenue' | 'growth' | 'activity' | 'attention' | 'neutral';
  items: MonthlyCreatorMetric[];
  value: (item: MonthlyCreatorMetric) => number;
  label: (item: MonthlyCreatorMetric) => string;
  valueLabel: (value: number) => string;
}) {
  const max = Math.max(...items.map(value), 1);
  return (
    <div className={`gl-studio-bars is-${tone}`}>
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
  tone = 'neutral',
  items,
  value,
}: {
  tone?: 'revenue' | 'growth' | 'activity' | 'attention' | 'neutral';
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
    <svg
      className={`gl-studio-line is-${tone}`}
      viewBox="0 0 100 100"
      preserveAspectRatio="none"
      aria-hidden
    >
      <polyline points={points} />
    </svg>
  );
}

function MetricRows({
  items,
  locale,
  t,
}: {
  items: MonthlyCreatorMetric[];
  locale: string;
  t: TFunction;
}) {
  return (
    <div className="gl-studio-metric-rows">
      {items.slice(-4).map((item) => (
        <div key={item.month}>
          <span>{formatMonth(item.month, locale)}</span>
          <strong>{formatHours(item.watchHours, locale, t)}</strong>
          <small>
            {t('studio.analytics.units.streamCount', {
              count: item.streams,
              formattedCount: item.streams.toLocaleString(locale),
            })}
          </small>
        </div>
      ))}
    </div>
  );
}

function BreakdownBars({
  gift,
  superChat,
  locale,
  t,
}: {
  gift: number;
  superChat: number;
  locale: string;
  t: TFunction;
}) {
  const total = Math.max(1, gift + superChat);
  return (
    <div className="gl-studio-breakdown">
      <div>
        <span>{t('studio.analytics.breakdown.gifts')}</span>
        <strong>{formatCoin(gift, locale, t)}</strong>
        <i className="is-gift" style={{ width: `${(gift / total) * 100}%` }} />
      </div>
      <div>
        <span>{t('studio.analytics.breakdown.superChat')}</span>
        <strong>{formatCoin(superChat, locale, t)}</strong>
        <i className="is-super-chat" style={{ width: `${(superChat / total) * 100}%` }} />
      </div>
    </div>
  );
}

function formatCoin(value: number, locale: string, t: TFunction): string {
  return t('studio.analytics.units.coinAmount', {
    amount: Math.round(value).toLocaleString(locale),
  });
}

function formatHours(value: number, locale: string, t: TFunction): string {
  return t('studio.analytics.units.hourAmount', {
    amount: value.toLocaleString(locale),
  });
}

function formatDate(value: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value));
}

function formatMonth(value: string, locale: string): string {
  const [year, month] = value.split('-').map(Number);
  if (!year || !month) return value;
  return new Intl.DateTimeFormat(locale, { month: 'short' }).format(new Date(year, month - 1, 1));
}
