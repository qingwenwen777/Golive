import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  Bell,
  BarChart3,
  Camera,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  CalendarDays,
  Clock3,
  FileText,
  ImagePlus,
  PlayCircle,
  Plus,
  Radio,
  Settings,
  Share2,
  UserPlus,
  Users,
  Video,
} from 'lucide-react';
import { toast } from 'sonner';
import { usePublicUser } from '@/api/auth';
import {
  useChannelLiveHistory,
  useChannelAppointments,
  useFollow,
  useFollowState,
  useRooms,
  useUnfollow,
  type LiveHistoryItem,
} from '@/api/room';
import { useChannelPosts } from '@/api/posts';
import { Avatar } from '@/components/Avatar';
import { UserLevelBadge } from '@/components/UserLevelBadge';
import { AppointmentViewerCard } from '@/components/AppointmentViewerCard';
import { LiveCard } from '@/components/LiveCard';
import { LoadableImage } from '@/components/LoadableImage';
import { LiveCardSkeleton } from '@/components/Skeleton';
import { AvatarUploadDialog } from '@/features/account/AvatarUploadDialog';
import { ChannelCoverUploadDialog } from '@/features/account/ChannelCoverUploadDialog';
import { useActiveCreatorLiveId } from '@/features/creator/useActiveCreatorLiveId';
import { PostCard } from '@/features/posts/PostCard';
import { copyText } from '@/lib/clipboard';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import { streamChannelName, type Stream } from '@/types/stream';
import { isUuidLike, userDisplayName, type User } from '@/types/user';

const HISTORY_PAGE_SIZE = 4;
const POST_PAGE_SIZE = 4;

export default function ChannelPage() {
  const { t } = useTranslation('pages');
  const { t: tc } = useTranslation('common');
  const navigate = useNavigate();
  const { name = '' } = useParams<{ name: string }>();
  const channelKey = decodeURIComponent(name);
  const profileLookupKey = channelKey.startsWith('ch-') ? channelKey.slice(3) : channelKey;
  const rooms = useRooms({ size: 100 });
  const publicUser = usePublicUser(profileLookupKey);
  const authUser = useAuthStore((s) => s.user);
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const [avatarOpen, setAvatarOpen] = useState(false);
  const [coverOpen, setCoverOpen] = useState(false);
  const [historyPage, setHistoryPage] = useState(1);
  const [appointmentPage, setAppointmentPage] = useState(1);
  const [postPage, setPostPage] = useState(1);
  const activeLiveId = useActiveCreatorLiveId(authUser?.id);

  const profile = useMemo(
    () => resolveProfile(profileLookupKey, publicUser.data, authUser),
    [authUser, profileLookupKey, publicUser.data],
  );
  const streams = rooms.data?.items ?? [];
  const channelStreams = useMemo(
    () => streams.filter((stream) => matchesChannel(stream, channelKey, profile)),
    [channelKey, profile, streams],
  );
  const primary = channelStreams[0];
  const channelName = resolveChannelName(profile, primary, channelKey, t);
  const channelAvatar = profile?.avatar || primary?.avatar || '';
  const channelCover = resolveChannelCover(profile);
  const channelId =
    primary?.channelId || (profile?.id ? `ch-${profile.id}` : normalizeChannelId(channelKey));
  const isOwner = Boolean(authUser?.id && profile?.id && authUser.id === profile.id);
  const followState = useFollowState(channelId, !!channelId);
  const follow = useFollow(channelId);
  const unfollow = useUnfollow(channelId);
  const liveHistory = useChannelLiveHistory(channelKey, historyPage, HISTORY_PAGE_SIZE);
  const historyTotal = liveHistory.data?.total ?? 0;
  const historyPageSize = liveHistory.data?.size ?? HISTORY_PAGE_SIZE;
  const historyPageCount = Math.max(1, Math.ceil(historyTotal / historyPageSize));
  const channelAppointments = useChannelAppointments(channelKey, true, appointmentPage, 4);
  const appointmentTotal = channelAppointments.data?.total ?? 0;
  const appointmentPageSize = channelAppointments.data?.size ?? 4;
  const appointmentPageCount = Math.max(1, Math.ceil(appointmentTotal / appointmentPageSize));
  const channelPosts = useChannelPosts(channelKey, true, postPage, POST_PAGE_SIZE);
  const postPageCount = Math.max(1, Math.ceil((channelPosts.data?.total ?? 0) / (channelPosts.data?.size ?? POST_PAGE_SIZE)));

  const totalViewers = channelStreams.reduce((sum, stream) => sum + stream.viewers, 0);
  const primaryCategory = primary?.category ?? 'Just Chatting';
  const subscriberCount = followState.data?.subscriberCount ?? primary?.subscriberCount ?? 0;
  const isUnknown = !profile && !primary && !publicUser.isPending && !rooms.isPending;

  useEffect(() => {
    setHistoryPage(1);
  }, [channelKey]);

  useEffect(() => {
    setAppointmentPage(1);
  }, [channelKey]);

  useEffect(() => {
    setPostPage(1);
  }, [channelKey]);

  useEffect(() => {
    if (historyPage > historyPageCount) {
      setHistoryPage(historyPageCount);
    }
  }, [historyPage, historyPageCount]);

  useEffect(() => {
    if (appointmentPage > appointmentPageCount) {
      setAppointmentPage(appointmentPageCount);
    }
  }, [appointmentPage, appointmentPageCount]);

  useEffect(() => {
    if (postPage > postPageCount) {
      setPostPage(postPageCount);
    }
  }, [postPage, postPageCount]);

  const handleSubscribe = () => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (isOwner || !channelId) return;
    if (followState.data?.following) unfollow.mutate();
    else follow.mutate();
  };

  const handleShare = async () => {
    const url = window.location.href;
    try {
      const method = await copyText(url, t('channel.copyTarget'));
      if (method === 'manual') toast.info(t('channel.copyManual'));
      else toast.success(t('channel.copySuccess'));
    } catch {
      toast.info(url);
    }
  };

  const handleOwnerLiveAction = () => {
    navigate(activeLiveId ? `/studio/live/${encodeURIComponent(activeLiveId)}` : '/studio/prepare');
  };

  const ownerLiveLabel = activeLiveId
    ? tc('nav.liveNow', { defaultValue: t('home.liveNow') })
    : tc('goLive', { defaultValue: t('channel.startLive') });

  return (
    <div className="gl-page gl-channel-page">
      <section className="gl-channel-hero-v2">
        <div
          className={['gl-channel-cover', channelCover ? 'has-cover' : '']
            .filter(Boolean)
            .join(' ')}
        >
          {channelCover && (
            <LoadableImage className="gl-channel-cover-img" src={channelCover} alt="" />
          )}
          <div className="gl-channel-cover-mark">
            <Radio size={26} />
            <span>GoLive</span>
          </div>
          {isOwner && (
            <button
              className="gl-channel-cover-action"
              type="button"
              onClick={() => setCoverOpen(true)}
            >
              <ImagePlus size={16} />
              {t('channel.changeCover')}
            </button>
          )}
        </div>

        <div className="gl-channel-profile-v2">
          <Avatar name={channelName} src={channelAvatar} size={112} className="gl-channel-avatar" />
          <div className="gl-channel-profile-main">
            <div className="gl-channel-kicker">
              {isOwner ? t('channel.yourChannel') : t('channel.liveChannel')}
            </div>
            <h1>
              <span>{channelName}</span>
              {(profile?.verified || primary?.verified) && <CheckCircle2 size={22} />}
              {profile?.levelInfo && <UserLevelBadge levelInfo={profile.levelInfo} />}
            </h1>
            <div className="gl-channel-handle">
              {profile?.username ? (
                <span>@{profile.username}</span>
              ) : (
                <span>{formatChannelKey(channelKey, t)}</span>
              )}
              <span>{t('channel.subscribers', { count: subscriberCount })}</span>
              <span>{t('channel.activeRooms', { count: channelStreams.length })}</span>
            </div>

            <div className="gl-channel-actions">
              {isOwner ? (
                <>
                  <button className="gl-retry-btn" type="button" onClick={handleOwnerLiveAction}>
                    {activeLiveId ? <Radio size={16} /> : <Plus size={16} />}
                    {ownerLiveLabel}
                  </button>
                  <button
                    className="gl-secondary-btn"
                    type="button"
                    onClick={() => setAvatarOpen(true)}
                  >
                    <Camera size={16} />
                    {t('channel.changeAvatar')}
                  </button>
                  <button
                    className="gl-secondary-btn"
                    type="button"
                    onClick={() => navigate('/settings')}
                  >
                    <Settings size={16} />
                    {t('channel.settings')}
                  </button>
                </>
              ) : (
                <button
                  className="gl-retry-btn"
                  type="button"
                  disabled={follow.isPending || unfollow.isPending}
                  onClick={handleSubscribe}
                >
                  {followState.data?.following ? <Bell size={16} /> : <UserPlus size={16} />}
                  {followState.data?.following ? t('channel.subscribed') : t('channel.subscribe')}
                </button>
              )}
              <button className="gl-secondary-btn" type="button" onClick={handleShare}>
                <Share2 size={16} />
                {t('channel.share')}
              </button>
            </div>
          </div>
        </div>

        <div className="gl-channel-stats">
          <ChannelStat label={t('channel.stats.liveRooms')} value={String(channelStreams.length)} />
          <ChannelStat
            label={t('channel.stats.watchingNow')}
            value={totalViewers.toLocaleString()}
          />
          <ChannelStat label={t('channel.stats.mainCategory')} value={primaryCategory} />
        </div>
      </section>

      <section className="gl-library-section" id="appointments">
        <div className="gl-section-title-row">
          <div>
            <h2>{t('channel.appointments.title', { defaultValue: 'Live appointments' })}</h2>
            <span>
              {t('channel.appointments.subtitle', {
                defaultValue: 'Reserve or follow upcoming live rooms from this channel.',
              })}
            </span>
          </div>
        </div>
        {channelAppointments.isPending ? (
          <div className="gl-grid" aria-busy="true">
            {Array.from({ length: 4 }).map((_, i) => (
              <LiveCardSkeleton key={i} />
            ))}
          </div>
        ) : channelAppointments.data?.items.length ? (
          <>
            <div className="gl-appointment-grid">
              {channelAppointments.data.items.map((item) => (
                <AppointmentViewerCard
                  key={item.id}
                  appointment={item}
                  to={`/live/${encodeURIComponent(item.roomId)}`}
                />
              ))}
            </div>
            {appointmentPageCount > 1 && (
              <HistoryPager
                page={appointmentPage}
                pageCount={appointmentPageCount}
                pageSize={appointmentPageSize}
                total={appointmentTotal}
                onPageChange={setAppointmentPage}
              />
            )}
          </>
        ) : (
          <div className="gl-channel-empty">
            <CalendarDays size={34} />
            <div>
              <strong>
                {t('channel.appointments.empty', { defaultValue: 'No upcoming appointments.' })}
              </strong>
              <span>
                {t('channel.appointments.emptySub', {
                  defaultValue: 'Check back later or browse other creators.',
                })}
              </span>
            </div>
          </div>
        )}
      </section>

      <section className="gl-library-section" id="posts">
        <div className="gl-section-title-row">
          <div>
            <h2>{t('channel.posts.title', { defaultValue: '帖子动态' })}</h2>
            <span>
              {t('channel.posts.subtitle', {
                defaultValue: '预约直播下方会显示主播最近发布的帖子。',
              })}
            </span>
          </div>
        </div>
        {channelPosts.isPending ? (
          <div className="gl-post-feed-list" aria-busy="true">
            {Array.from({ length: 2 }).map((_, index) => (
              <div className="gl-post-card is-loading" key={index} />
            ))}
          </div>
        ) : channelPosts.data?.items.length ? (
          <>
            <div className="gl-post-feed-list">
              {channelPosts.data.items.map((post) => (
                <PostCard key={post.id} post={post} />
              ))}
            </div>
            {postPageCount > 1 && (
              <ChannelPostPager
                page={postPage}
                pageCount={postPageCount}
                onPageChange={setPostPage}
              />
            )}
          </>
        ) : (
          <div className="gl-channel-empty">
            <FileText size={34} />
            <div>
              <strong>{t('channel.posts.empty', { defaultValue: '暂时还没有帖子。' })}</strong>
              <span>
                {t('channel.posts.emptySub', { defaultValue: '主播发布后，这里会展示最新动态。' })}
              </span>
            </div>
          </div>
        )}
      </section>

      <section className="gl-library-section" id="live">
        <div className="gl-section-title-row">
          <h2>{t('channel.liveRooms')}</h2>
        </div>
        {rooms.isPending ? (
          <div className="gl-grid" aria-busy="true">
            {Array.from({ length: 4 }).map((_, i) => (
              <LiveCardSkeleton key={i} />
            ))}
          </div>
        ) : channelStreams.length > 0 ? (
          <div className="gl-grid">
            {channelStreams.map((stream, i) => (
              <LiveCard key={stream.id} stream={stream} priority={i < 2} />
            ))}
          </div>
        ) : (
          <div className="gl-channel-empty">
            <Video size={34} />
            <div>
              <strong>{isUnknown ? t('channel.notFound') : t('channel.noLiveRooms')}</strong>
              <span>
                {isUnknown
                  ? t('channel.notFoundSub')
                  : isOwner
                    ? t('channel.noLiveOwnerSub')
                    : t('channel.noLiveViewerSub')}
              </span>
            </div>
            {isOwner && (
              <button className="gl-retry-btn" type="button" onClick={handleOwnerLiveAction}>
                {activeLiveId ? <Radio size={16} /> : <Plus size={16} />}
                {ownerLiveLabel}
              </button>
            )}
          </div>
        )}
      </section>

      <section className="gl-library-section" id="history">
        <div className="gl-section-title-row">
          <h2>{t('channel.liveHistory')}</h2>
        </div>
        {liveHistory.isPending ? (
          <div className="gl-history-list" aria-busy="true">
            {Array.from({ length: 3 }).map((_, i) => (
              <div className="gl-history-row is-loading" key={i} />
            ))}
          </div>
        ) : liveHistory.data?.items.length ? (
          <>
            <div className="gl-history-list">
              {liveHistory.data.items.map((record) => (
                <ChannelHistoryRow
                  key={record.id}
                  record={record}
                  channelKey={channelKey}
                  isOwner={isOwner}
                />
              ))}
            </div>
            {historyPageCount > 1 && (
              <HistoryPager
                page={historyPage}
                pageCount={historyPageCount}
                pageSize={historyPageSize}
                total={historyTotal}
                onPageChange={setHistoryPage}
              />
            )}
          </>
        ) : (
          <div className="gl-channel-empty">
            <Clock3 size={34} />
            <div>
              <strong>{t('channel.noHistory')}</strong>
              <span>
                {isOwner ? t('channel.noHistoryOwnerSub') : t('channel.noHistoryViewerSub')}
              </span>
            </div>
          </div>
        )}
      </section>

      <AvatarUploadDialog open={avatarOpen} onOpenChange={setAvatarOpen} user={authUser} />
      <ChannelCoverUploadDialog open={coverOpen} onOpenChange={setCoverOpen} user={authUser} />
    </div>
  );
}

function HistoryPager({
  page,
  pageCount,
  pageSize,
  total,
  onPageChange,
}: {
  page: number;
  pageCount: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
}) {
  const { t } = useTranslation('pages');
  const start = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const end = Math.min(total, page * pageSize);
  const pages = visibleHistoryPages(page, pageCount);
  return (
    <div className="gl-history-pager" aria-label={t('channel.history.pagination')}>
      <div className="gl-history-pager-count">
        {t('channel.history.pageCount', { start, end, total })}
      </div>
      <div className="gl-history-pager-controls">
        <button
          type="button"
          aria-label={t('channel.history.previousPage')}
          disabled={page <= 1}
          onClick={() => onPageChange(Math.max(1, page - 1))}
        >
          <ChevronLeft size={16} />
        </button>
        {pages.map((item, index) =>
          item === 'gap' ? (
            <span key={`gap-${index}`} className="gl-history-pager-gap">
              ...
            </span>
          ) : (
            <button
              key={item}
              type="button"
              className={item === page ? 'is-active' : undefined}
              aria-current={item === page ? 'page' : undefined}
              onClick={() => onPageChange(item)}
            >
              {item}
            </button>
          ),
        )}
        <button
          type="button"
          aria-label={t('channel.history.nextPage')}
          disabled={page >= pageCount}
          onClick={() => onPageChange(Math.min(pageCount, page + 1))}
        >
          <ChevronRight size={16} />
        </button>
      </div>
    </div>
  );
}

function ChannelPostPager({
  page,
  pageCount,
  onPageChange,
}: {
  page: number;
  pageCount: number;
  onPageChange: (page: number) => void;
}) {
  const { t } = useTranslation('pages');
  const pages = visibleHistoryPages(page, pageCount);
  return (
    <div
      className="gl-history-pager gl-channel-post-pager"
      aria-label={t('channel.posts.pagination', { defaultValue: 'Post pagination' })}
    >
      <div className="gl-history-pager-controls">
        <button
          type="button"
          aria-label={t('channel.posts.previous', { defaultValue: 'Previous page' })}
          disabled={page <= 1}
          onClick={() => onPageChange(Math.max(1, page - 1))}
        >
          <ChevronLeft size={16} />
        </button>
        {pages.map((item, index) =>
          item === 'gap' ? (
            <span key={`gap-${index}`} className="gl-history-pager-gap">
              ...
            </span>
          ) : (
            <button
              key={item}
              type="button"
              className={item === page ? 'is-active' : undefined}
              aria-current={item === page ? 'page' : undefined}
              onClick={() => onPageChange(item)}
            >
              {item}
            </button>
          ),
        )}
        <button
          type="button"
          aria-label={t('channel.posts.next', { defaultValue: 'Next page' })}
          disabled={page >= pageCount}
          onClick={() => onPageChange(Math.min(pageCount, page + 1))}
        >
          <ChevronRight size={16} />
        </button>
      </div>
    </div>
  );
}

function visibleHistoryPages(page: number, pageCount: number): Array<number | 'gap'> {
  if (pageCount <= 5) {
    return Array.from({ length: pageCount }, (_, index) => index + 1);
  }
  const pages = new Set([1, pageCount, page - 1, page, page + 1]);
  const sorted = [...pages].filter((item) => item >= 1 && item <= pageCount).sort((a, b) => a - b);
  return sorted.flatMap((item, index) => {
    const prev = sorted[index - 1];
    if (index > 0 && prev !== undefined && item - prev > 1) {
      return ['gap' as const, item];
    }
    return [item];
  });
}

function ChannelHistoryRow({
  record,
  channelKey,
  isOwner,
}: {
  record: LiveHistoryItem;
  channelKey: string;
  isOwner: boolean;
}) {
  const { t, i18n } = useTranslation('pages');

  return (
    <article className="gl-history-row">
      <HistoryThumb record={record} />
      <div className="gl-history-main">
        <h3>{record.title}</h3>
        <div className="gl-history-meta">
          <span>
            <CalendarDays size={14} />
            {formatHistoryDate(record.startedAt, i18n.language)}
          </span>
          <span>
            <Clock3 size={14} />
            {record.duration}
          </span>
          <span>
            <Users size={14} />
            {t('channel.history.peakViewers', {
              amount: record.peakViewers.toLocaleString(),
            })}
          </span>
        </div>
        <div className="gl-history-sub">
          <span>{record.category || t('channel.tabs.live')}</span>
          <span>{t('channel.history.revenue', { amount: formatCoin(record.revenueCoin) })}</span>
          <span>
            {record.topFan
              ? t('channel.history.topFan', { name: record.topFan.name })
              : t('channel.history.noFan')}
          </span>
        </div>
      </div>
      {isOwner && (
        <Link
          className="gl-secondary-btn gl-history-analysis-btn"
          to={`/studio/analytics/${encodeURIComponent(channelKey)}/live/${encodeURIComponent(record.id)}`}
        >
          <BarChart3 size={16} />
          {t('channel.history.liveAnalysis')}
        </Link>
      )}
      {record.replay?.canWatch && (
        <Link
          className="gl-secondary-btn gl-history-analysis-btn"
          to={`/live/${encodeURIComponent(record.id)}`}
        >
          <PlayCircle size={16} />
          {t('channel.history.watchReplay', { defaultValue: 'Watch replay' })}
        </Link>
      )}
    </article>
  );
}

function HistoryThumb({ record }: { record: LiveHistoryItem }) {
  const initials = record.title.trim().slice(0, 2).toUpperCase() || 'GL';
  return (
    <div
      className={['gl-history-thumb', record.cover ? 'has-image' : ''].filter(Boolean).join(' ')}
    >
      <div className="gl-history-thumb-fallback">{initials}</div>
      {record.cover && <LoadableImage src={record.cover} alt="" />}
      <span>{record.duration}</span>
    </div>
  );
}

function ChannelStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="gl-channel-stat">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function resolveProfile(
  key: string,
  publicUser: User | undefined,
  authUser: User | null,
): User | null {
  if (publicUser) {
    if (authUser?.id === publicUser.id) return authUser;
    return publicUser;
  }
  if (authUser && (authUser.id === key || authUser.username === key)) return authUser;
  return null;
}

function resolveChannelName(
  profile: User | null,
  stream: Stream | undefined,
  key: string,
  t: ReturnType<typeof useTranslation>['t'],
): string {
  if (profile) return userDisplayName(profile);
  if (stream) return streamChannelName(stream);
  if (key && !isUuidLike(key)) return key;
  return t('channel.creatorFallback');
}

export function resolveChannelCover(profile: User | null): string {
  return profile?.cover || '';
}

function matchesChannel(stream: Stream, key: string, profile: User | null): boolean {
  const normalized = key.toLowerCase();
  const channelName = streamChannelName(stream).toLowerCase();
  const ownerID = profile?.id.toLowerCase();
  const channelID = ownerID ? `ch-${ownerID}` : '';
  return (
    stream.channelId.toLowerCase() === normalized ||
    stream.channel.toLowerCase() === normalized ||
    stream.ownerId?.toLowerCase() === normalized ||
    channelName === normalized ||
    (!!ownerID && stream.ownerId?.toLowerCase() === ownerID) ||
    (!!channelID && stream.channelId.toLowerCase() === channelID)
  );
}

function normalizeChannelId(key: string): string {
  if (!key) return '';
  if (key.startsWith('ch-')) return key;
  return `ch-${key}`;
}

function formatChannelKey(key: string, t: ReturnType<typeof useTranslation>['t']): string {
  if (!key) return t('channel.title');
  if (!isUuidLike(key)) return key;
  return t('channel.creatorShort', { id: key.slice(0, 8) });
}

function formatHistoryDate(value: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value));
}

function formatCoin(value: number): string {
  return `${Math.round(value).toLocaleString()} coins`;
}
