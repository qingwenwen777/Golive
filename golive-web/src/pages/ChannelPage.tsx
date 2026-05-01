import { useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  Bell,
  Camera,
  CheckCircle2,
  Radio,
  Settings,
  Share2,
  UserPlus,
  Video,
} from 'lucide-react';
import { toast } from 'sonner';
import { usePublicUser } from '@/api/auth';
import { useFollow, useFollowState, useRooms, useUnfollow } from '@/api/room';
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
  const channelId = primary?.channelId || (profile?.id ? `ch-${profile.id}` : normalizeChannelId(channelKey));
  const isOwner = Boolean(authUser?.id && profile?.id && authUser.id === profile.id);
  const followState = useFollowState(channelId, isAuthed && !isOwner && !!channelId);
  const follow = useFollow(channelId);
  const unfollow = useUnfollow(channelId);

  const totalViewers = channelStreams.reduce((sum, stream) => sum + stream.viewers, 0);
  const primaryCategory = primary?.category ?? 'Just Chatting';
  const subscriberCount = stableSubscriberCount(profile, channelName, totalViewers);
  const isUnknown = !profile && !primary && !publicUser.isPending && !rooms.isPending;

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
              {profile?.username ? <span>@{profile.username}</span> : <span>{formatChannelKey(channelKey)}</span>}
              <span>{subscriberCount.toLocaleString()} subscribers</span>
              <span>{channelStreams.length} active room{channelStreams.length === 1 ? '' : 's'}</span>
            </div>

            <div className="gl-channel-actions">
              {isOwner ? (
                <>
                  <button className="gl-retry-btn" type="button" onClick={() => setCreateOpen(true)}>
                    <Radio size={16} />
                    Start live
                  </button>
                  <button className="gl-secondary-btn" type="button" onClick={() => setAvatarOpen(true)}>
                    <Camera size={16} />
                    Change avatar
                  </button>
                  <button className="gl-secondary-btn" type="button" onClick={() => navigate('/settings')}>
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

      <AvatarUploadDialog open={avatarOpen} onOpenChange={setAvatarOpen} user={authUser} />
      <CreateLiveDialog open={createOpen} onOpenChange={setCreateOpen} />
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

function resolveProfile(key: string, publicUser: User | undefined, authUser: User | null): User | null {
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

function stableSubscriberCount(profile: User | null, name: string, viewers: number): number {
  const seed = profile?.id || profile?.username || name;
  let hash = 0;
  for (let i = 0; i < seed.length; i++) hash = (hash * 31 + seed.charCodeAt(i)) & 0xffff;
  return 1200 + (hash % 9000) + viewers * 12;
}
