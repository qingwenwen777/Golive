import { useEffect, useMemo, useState } from 'react';
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
  useFollow,
  useFollowState,
  useRooms,
  useUnfollow,
  type LiveHistoryItem,
} from '@/api/room';
import { Avatar } from '@/components/Avatar';
import { LiveCard } from '@/components/LiveCard';
import { LiveCardSkeleton } from '@/components/Skeleton';
import { AvatarUploadDialog } from '@/features/account/AvatarUploadDialog';
import { CreateLiveDialog } from '@/features/creator/CreateLiveDialog';
import { copyText } from '@/lib/clipboard';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import { streamChannelName, type Stream } from '@/types/stream';
import { isUuidLike, userDisplayName, type User } from '@/types/user';

const HISTORY_PAGE_SIZE = 4;

export default function ChannelPage() {
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
  const [createOpen, setCreateOpen] = useState(false);
  const [historyPage, setHistoryPage] = useState(1);

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
  const channelName = resolveChannelName(profile, primary, channelKey);
  const channelAvatar = profile?.avatar || primary?.avatar || '';
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

  const totalViewers = channelStreams.reduce((sum, stream) => sum + stream.viewers, 0);
  const primaryCategory = primary?.category ?? 'Just Chatting';
  const subscriberCount = followState.data?.subscriberCount ?? primary?.subscriberCount ?? 0;
  const isUnknown = !profile && !primary && !publicUser.isPending && !rooms.isPending;

  useEffect(() => {
    setHistoryPage(1);
  }, [channelKey]);

  useEffect(() => {
    if (historyPage > historyPageCount) {
      setHistoryPage(historyPageCount);
    }
  }, [historyPage, historyPageCount]);

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
      const method = await copyText(url, 'channel link');
      if (method === 'manual') toast.info('Channel link opened for manual copy.');
      else toast.success('Channel link copied.');
    } catch {
      toast.info(url);
    }
  };

  return (
    <div className="gl-page gl-channel-page">
      <section className="gl-channel-hero-v2">
        <div className="gl-channel-cover" aria-hidden>
          <div className="gl-channel-cover-mark">
            <Radio size={26} />
            <span>GoLive</span>
          </div>
        </div>

        <div className="gl-channel-profile-v2">
          <Avatar name={channelName} src={channelAvatar} size={112} className="gl-channel-avatar" />
          <div className="gl-channel-profile-main">
            <div className="gl-channel-kicker">{isOwner ? 'Your channel' : 'Live channel'}</div>
            <h1>
              <span>{channelName}</span>
              {(profile?.verified || primary?.verified) && <CheckCircle2 size={22} />}
            </h1>
            <div className="gl-channel-handle">
              {profile?.username ? (
                <span>@{profile.username}</span>
              ) : (
                <span>{formatChannelKey(channelKey)}</span>
              )}
              <span>{subscriberCount.toLocaleString()} subscribers</span>
              <span>
                {channelStreams.length} active room{channelStreams.length === 1 ? '' : 's'}
              </span>
            </div>

            <div className="gl-channel-actions">
              {isOwner ? (
                <>
                  <button
                    className="gl-retry-btn"
                    type="button"
                    onClick={() => setCreateOpen(true)}
                  >
                    <Radio size={16} />
                    Start live
                  </button>
                  <button
                    className="gl-secondary-btn"
                    type="button"
                    onClick={() => setAvatarOpen(true)}
                  >
                    <Camera size={16} />
                    Change avatar
                  </button>
                  <button
                    className="gl-secondary-btn"
                    type="button"
                    onClick={() => navigate('/settings')}
                  >
                    <Settings size={16} />
                    Settings
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
                  {followState.data?.following ? 'Subscribed' : 'Subscribe'}
                </button>
              )}
              <button className="gl-secondary-btn" type="button" onClick={handleShare}>
                <Share2 size={16} />
                Share
              </button>
            </div>
          </div>
        </div>

        <div className="gl-channel-stats">
          <ChannelStat label="Live rooms" value={String(channelStreams.length)} />
          <ChannelStat label="Watching now" value={totalViewers.toLocaleString()} />
          <ChannelStat label="Main category" value={primaryCategory} />
        </div>
      </section>

      <nav className="gl-channel-tabs" aria-label="Channel sections">
        <a className="is-active" href="#live">
          Live
        </a>
        <a href="#history">History</a>
      </nav>

      <section className="gl-library-section" id="live">
        <div className="gl-section-title-row">
          <h2>Live rooms</h2>
          <Link className="gl-text-link" to="/subscriptions">
            Subscriptions
          </Link>
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
              <strong>{isUnknown ? 'Channel not found' : 'No live rooms right now'}</strong>
              <span>
                {isUnknown
                  ? 'This creator profile is unavailable or the link is incorrect.'
                  : isOwner
                    ? 'Start a live room when you are ready to broadcast.'
                    : 'Follow this channel and check back when the creator goes live.'}
              </span>
            </div>
            {isOwner && (
              <button className="gl-retry-btn" type="button" onClick={() => setCreateOpen(true)}>
                <Radio size={16} />
                Start live
              </button>
            )}
          </div>
        )}
      </section>

      <section className="gl-library-section" id="history">
        <div className="gl-section-title-row">
          <h2>Live history</h2>
          {isOwner && (
            <Link
              className="gl-text-link"
              to={`/studio/analytics/${encodeURIComponent(channelKey)}`}
            >
              Channel analytics
            </Link>
          )}
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
              <strong>No completed live streams yet</strong>
              <span>
                {isOwner
                  ? 'Finished broadcasts will appear here with duration, cover, title, and performance data.'
                  : 'This creator has not finished a broadcast that can be shown here yet.'}
              </span>
            </div>
          </div>
        )}
      </section>

      <AvatarUploadDialog open={avatarOpen} onOpenChange={setAvatarOpen} user={authUser} />
      <CreateLiveDialog open={createOpen} onOpenChange={setCreateOpen} />
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
  const start = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const end = Math.min(total, page * pageSize);
  const pages = visibleHistoryPages(page, pageCount);
  return (
    <div className="gl-history-pager" aria-label="Live history pagination">
      <div className="gl-history-pager-count">
        {start}-{end} of {total}
      </div>
      <div className="gl-history-pager-controls">
        <button
          type="button"
          aria-label="Previous history page"
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
          aria-label="Next history page"
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
  const sorted = [...pages]
    .filter((item) => item >= 1 && item <= pageCount)
    .sort((a, b) => a - b);
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
  return (
    <article className="gl-history-row">
      <HistoryThumb record={record} />
      <div className="gl-history-main">
        <h3>{record.title}</h3>
        <div className="gl-history-meta">
          <span>
            <CalendarDays size={14} />
            {formatHistoryDate(record.startedAt)}
          </span>
          <span>
            <Clock3 size={14} />
            {record.duration}
          </span>
          <span>
            <Users size={14} />
            {record.peakViewers.toLocaleString()} peak
          </span>
        </div>
        <div className="gl-history-sub">
          <span>{record.category || 'Live'}</span>
          <span>{formatCoin(record.revenueCoin)} revenue</span>
          <span>{record.topFan ? `${record.topFan.name} top fan` : 'No fan contribution yet'}</span>
        </div>
      </div>
      {isOwner && (
        <Link
          className="gl-secondary-btn gl-history-analysis-btn"
          to={`/studio/analytics/${encodeURIComponent(channelKey)}/live/${encodeURIComponent(record.id)}`}
        >
          <BarChart3 size={16} />
          Live analysis
        </Link>
      )}
    </article>
  );
}

function HistoryThumb({ record }: { record: LiveHistoryItem }) {
  const initials = record.title.trim().slice(0, 2).toUpperCase() || 'GL';
  return (
    <div className="gl-history-thumb">
      <div className="gl-history-thumb-fallback">{initials}</div>
      {record.cover && <img src={record.cover} alt="" />}
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

function resolveChannelName(profile: User | null, stream: Stream | undefined, key: string): string {
  if (profile) return userDisplayName(profile);
  if (stream) return streamChannelName(stream);
  if (key && !isUuidLike(key)) return key;
  return 'Creator';
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

function formatChannelKey(key: string): string {
  if (!key) return 'Channel';
  if (!isUuidLike(key)) return key;
  return `Creator ${key.slice(0, 8)}`;
}

function formatHistoryDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value));
}

function formatCoin(value: number): string {
  return `${Math.round(value).toLocaleString()} coins`;
}
