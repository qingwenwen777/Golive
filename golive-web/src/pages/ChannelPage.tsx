/* eslint-disable react-refresh/only-export-components */
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  BadgeCheck,
  Bell,
  BarChart3,
  Camera,
  ChevronLeft,
  ChevronRight,
  CalendarDays,
  Clock3,
  Coins,
  Crown,
  FileText,
  ImagePlus,
  MessageCircle,
  MoreHorizontal,
  PlayCircle,
  Plus,
  Radio,
  Settings,
  Share2,
  UserPlus,
  Users,
  Video,
  Flag,
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
  type ChannelHistoryItem,
} from '@/api/room';
import { useFanBadges, useFanClubMembers, useJoinFanClub } from '@/api/gift';
import { useChannelPosts } from '@/api/posts';
import { Avatar } from '@/components/Avatar';
import { UserLevelBadge } from '@/components/UserLevelBadge';
import { VerifiedBadge } from '@/components/VerifiedBadge';
import { AppointmentViewerCard } from '@/components/AppointmentViewerCard';
import { LiveCard } from '@/components/LiveCard';
import { LoadableImage } from '@/components/LoadableImage';
import { ReplayCard } from '@/components/ReplayCard';
import { ShareDialog } from '@/components/ShareDialog';
import { LiveCardSkeleton, Skeleton } from '@/components/Skeleton';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { AvatarUploadDialog } from '@/features/account/AvatarUploadDialog';
import { ChannelCoverUploadDialog } from '@/features/account/ChannelCoverUploadDialog';
import { useActiveCreatorLiveId } from '@/features/creator/useActiveCreatorLiveId';
import { PostCard } from '@/features/posts/PostCard';
import { ReportDialog, type ReportTargetDraft } from '@/features/reporting/ReportDialog';
import { cn } from '@/lib/cn';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import { fanBadgeToneClass } from '@/lib/fanBadgeTone';
import type { FanClubMember } from '@/types/gift';
import { isPlaceholderChannelName, streamChannelName, type Stream } from '@/types/stream';
import { isUuidLike, userDisplayName, type User } from '@/types/user';
import { formatNumber, formatRelativeTime } from '@/lib/format';

const HISTORY_PAGE_SIZE = 4;
const REPLAY_GRID_PAGE_SIZE = 8;
const POST_PAGE_SIZE = 4;
const FAN_BADGE_PRICE = 1000;

type ChannelTab = 'home' | 'posts' | 'history';

const CHANNEL_TABS: Array<{ id: ChannelTab; labelKey: string; defaultValue: string }> = [
  { id: 'home', labelKey: 'channel.tabs.home', defaultValue: '主页' },
  { id: 'posts', labelKey: 'channel.tabs.posts', defaultValue: '帖子动态' },
  { id: 'history', labelKey: 'channel.tabs.liveHistory', defaultValue: '直播历史' },
];

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
  const [activeTab, setActiveTab] = useState<ChannelTab>('home');
  const [fanBadgeDialogOpen, setFanBadgeDialogOpen] = useState(false);
  const [reportTarget, setReportTarget] = useState<ReportTargetDraft | null>(null);
  const [shareOpen, setShareOpen] = useState(false);
  const activeLiveId = useActiveCreatorLiveId(authUser?.id);

  const profile = useMemo(
    () => resolveProfile(profileLookupKey, publicUser.data, authUser),
    [authUser, profileLookupKey, publicUser.data],
  );
  const streams = useMemo(() => rooms.data?.items ?? [], [rooms.data?.items]);
  const channelStreams = useMemo(
    () =>
      streams.filter((stream) => matchesChannel(stream, channelKey, profile, publicUser.isPending)),
    [channelKey, profile, publicUser.isPending, streams],
  );
  const primary = channelStreams[0];
  const resolvedChannelName = resolveChannelName(profile, primary, channelKey, t);
  const channelIdentityPending = shouldHoldChannelIdentity({
    channelKey,
    profile,
    stream: primary,
    profilePending: publicUser.isPending,
    roomsPending: rooms.isPending,
  });
  const channelName = resolvedChannelName;
  const channelAvatar = channelIdentityPending ? '' : profile?.avatar || primary?.avatar || '';
  const channelCover = channelIdentityPending ? '' : resolveChannelCover(profile);
  const channelId =
    primary?.channelId || (profile?.id ? `ch-${profile.id}` : normalizeChannelId(channelKey));
  const isOwner = Boolean(authUser?.id && profile?.id && authUser.id === profile.id);
  const historyMode = isOwner ? 'history' : 'replay';
  const historyRequestSize = historyMode === 'replay' ? REPLAY_GRID_PAGE_SIZE : HISTORY_PAGE_SIZE;
  const followState = useFollowState(channelId, !!channelId);
  const follow = useFollow(channelId);
  const unfollow = useUnfollow(channelId);
  const liveHistory = useChannelLiveHistory(
    channelKey,
    historyPage,
    historyRequestSize,
    historyMode,
  );
  const historyTotal = liveHistory.data?.total ?? 0;
  const historyPageSize = liveHistory.data?.size ?? historyRequestSize;
  const historyPageCount = Math.max(1, Math.ceil(historyTotal / historyPageSize));
  const historyItems = liveHistory.data?.items ?? [];
  const channelAppointments = useChannelAppointments(channelKey, true, appointmentPage, 4);
  const appointmentTotal = channelAppointments.data?.total ?? 0;
  const appointmentPageSize = channelAppointments.data?.size ?? 4;
  const appointmentPageCount = Math.max(1, Math.ceil(appointmentTotal / appointmentPageSize));
  const channelPosts = useChannelPosts(channelKey, true, postPage, POST_PAGE_SIZE);
  const postPageCount = Math.max(
    1,
    Math.ceil((channelPosts.data?.total ?? 0) / (channelPosts.data?.size ?? POST_PAGE_SIZE)),
  );

  const subscriberCount = followState.data?.subscriberCount ?? primary?.subscriberCount ?? 0;
  const isUnknown = !profile && !primary && !publicUser.isPending && !rooms.isPending;
  const joinFanClub = useJoinFanClub();
  const fanBadges = useFanBadges(isAuthed, authUser?.id);
  const creatorId = profile?.id || primary?.ownerId || normalizeCreatorId(channelId);
  const fanClubMembers = useFanClubMembers(creatorId, Boolean(creatorId), 5);
  const currentFanBadge =
    fanBadges.data?.find((badge) => creatorId && badge.creatorId === creatorId) ?? null;
  const fanPreviewItems = useMemo(
    () => buildFanPreviewItems({ members: fanClubMembers.data?.items ?? [] }),
    [fanClubMembers.data?.items],
  );

  useEffect(() => {
    setHistoryPage(1);
  }, [channelKey, historyMode]);

  useEffect(() => {
    setAppointmentPage(1);
  }, [channelKey]);

  useEffect(() => {
    setPostPage(1);
  }, [channelKey]);

  useEffect(() => {
    setActiveTab('home');
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

  const handleReportChannel = () => {
    const target: ReportTargetDraft = {
      targetType: 'channel',
      targetId: channelId || channelKey,
      targetUrl: window.location.href,
      channelId,
      targetOwnerId: creatorId,
      targetOwnerName: channelName,
      targetTitle: channelName,
      targetText: profile?.displayName || profile?.username || primary?.description || '',
    };
    if (!isAuthed) {
      openLogin(() => setReportTarget(target));
      return;
    }
    setReportTarget(target);
  };

  const handleMessageCreator = () => {
    if (!isAuthed) {
      openLogin();
      return;
    }
    if (!creatorId || isOwner) return;
    navigate(`/messages/direct/${encodeURIComponent(creatorId)}`);
  };

  const handleOwnerLiveAction = () => {
    navigate(activeLiveId ? `/studio/live/${encodeURIComponent(activeLiveId)}` : '/studio/prepare');
  };

  const handleJoinFanClub = () => {
    if (isOwner || currentFanBadge) return;
    setFanBadgeDialogOpen(true);
  };

  const handleConfirmFanBadge = () => {
    if (currentFanBadge) {
      toast.info(t('channel.fanClub.alreadyLit', { defaultValue: '粉丝灯牌已点亮。' }));
      setFanBadgeDialogOpen(false);
      return;
    }
    if (!isAuthed) {
      setFanBadgeDialogOpen(false);
      openLogin(() => setFanBadgeDialogOpen(true));
      return;
    }
    if (!creatorId) {
      toast.info(
        t('channel.fanBadge.noRoom', {
          defaultValue: '当前频道暂时没有可购买粉丝灯牌的直播间。',
        }),
      );
      return;
    }
    joinFanClub.mutate(
      {
        creatorId,
      },
      {
        onSuccess: (order) => {
          // A retried join can replay an earlier attempt that lacked coins.
          if (order.status !== 'success') {
            toast.error(
              t('channel.fanBadge.insufficient', {
                defaultValue: 'coins 不足，无法购买粉丝灯牌。',
              }),
            );
            return;
          }
          toast.success(t('channel.fanBadge.success', { defaultValue: '粉丝灯牌已点亮。' }));
          setFanBadgeDialogOpen(false);
        },
        onError: (err) => {
          if (err.reason === 'already_fan_club_member') {
            toast.info(
              t('channel.fanBadge.alreadyMember', {
                defaultValue: '你已经在这个粉丝团里了，本次未扣除 coins。',
              }),
            );
            setFanBadgeDialogOpen(false);
            return;
          }
          if (err.reason === 'insufficient_coin') {
            toast.error(
              t('channel.fanBadge.insufficient', {
                defaultValue: 'coins 不足，无法购买粉丝灯牌。',
              }),
            );
            return;
          }
          toast.error(
            err.message || t('channel.fanBadge.failed', { defaultValue: '粉丝灯牌购买失败。' }),
          );
        },
      },
    );
  };

  const ownerLiveLabel = activeLiveId
    ? tc('nav.liveNow', { defaultValue: t('home.liveNow') })
    : tc('goLive', { defaultValue: t('channel.startLive') });

  return (
    <div className="gl-page gl-channel-page">
      <section className="gl-channel-hero-v2">
        <div
          className={[
            'gl-channel-cover',
            channelCover ? 'has-cover' : '',
            channelIdentityPending ? 'is-loading' : '',
          ]
            .filter(Boolean)
            .join(' ')}
        >
          {channelCover && (
            <LoadableImage className="gl-channel-cover-img" src={channelCover} alt="" />
          )}
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
          <Avatar
            name={resolvedChannelName}
            src={channelAvatar}
            size={128}
            className="gl-channel-avatar"
          />
          <div className="gl-channel-profile-main">
            <h1>
              {channelIdentityPending ? (
                <Skeleton className="gl-channel-name-skeleton" />
              ) : (
                <>
                  <span>{channelName}</span>
                  {(profile?.verified || primary?.verified) && <VerifiedBadge size={24} />}
                  {profile?.levelInfo && <UserLevelBadge levelInfo={profile.levelInfo} />}
                </>
              )}
            </h1>
            <div className="gl-channel-handle">
              {channelIdentityPending ? (
                <>
                  <Skeleton className="gl-channel-handle-skeleton is-short" />
                  <Skeleton className="gl-channel-handle-skeleton" />
                </>
              ) : profile?.username ? (
                <span>@{profile.username}</span>
              ) : (
                <span>{formatChannelKey(channelKey, t)}</span>
              )}
              <span>{t('channel.subscribers', { count: subscriberCount })}</span>
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
                <>
                  <button
                    className="gl-retry-btn"
                    type="button"
                    disabled={follow.isPending || unfollow.isPending}
                    onClick={handleSubscribe}
                  >
                    {followState.data?.following ? <Bell size={16} /> : <UserPlus size={16} />}
                    {followState.data?.following ? t('channel.subscribed') : t('channel.subscribe')}
                  </button>
                  <button
                    className="gl-secondary-btn"
                    type="button"
                    disabled={!creatorId}
                    onClick={handleMessageCreator}
                    title={t('channel.message', { defaultValue: 'Message' })}
                  >
                    <MessageCircle size={16} />
                    {t('channel.message', { defaultValue: 'Message' })}
                  </button>
                </>
              )}
              <button className="gl-secondary-btn" type="button" onClick={() => setShareOpen(true)}>
                <Share2 size={16} />
                {t('channel.share')}
              </button>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button
                    className="gl-secondary-btn gl-channel-more-btn"
                    type="button"
                    aria-label={t('report.moreActions')}
                  >
                    <MoreHorizontal size={17} />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-40">
                  <DropdownMenuItem className="gl-menu-danger" onSelect={handleReportChannel}>
                    <Flag size={15} />
                    {t('report.action')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>
        </div>
      </section>

      <nav className="gl-channel-tabs" role="tablist" aria-label={t('channel.sections')}>
        {CHANNEL_TABS.map((tab) => {
          const labelKey =
            tab.id === 'history' && historyMode === 'replay'
              ? 'channel.tabs.liveReplays'
              : tab.labelKey;
          const defaultValue =
            tab.id === 'history' && historyMode === 'replay' ? 'Live replays' : tab.defaultValue;
          return (
            <button
              key={tab.id}
              type="button"
              role="tab"
              aria-selected={activeTab === tab.id}
              className={activeTab === tab.id ? 'is-active' : undefined}
              onClick={() => setActiveTab(tab.id)}
            >
              {t(labelKey, { defaultValue })}
            </button>
          );
        })}
      </nav>

      {activeTab === 'home' && (
        <div className="gl-channel-tab-panel">
          <FanClubBanner
            channelName={channelName}
            fanPreviewItems={fanPreviewItems}
            memberCount={fanClubMembers.data?.total ?? 0}
            pending={Boolean(creatorId) && fanClubMembers.isPending}
            hasFanBadge={Boolean(currentFanBadge)}
            checkingFanBadge={isAuthed && fanBadges.isPending}
            isOwner={isOwner}
            onJoin={handleJoinFanClub}
          />

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

          <section className="gl-library-section" id="appointments">
            <div className="gl-section-title-row">
              <div>
                <h2>{t('channel.appointments.title', { defaultValue: '直播预约' })}</h2>
                <span>
                  {t('channel.appointments.subtitle', {
                    defaultValue: '查看并预约这个频道即将开始的直播。',
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
                    {t('channel.appointments.empty', { defaultValue: '暂无直播预约。' })}
                  </strong>
                  <span>
                    {t('channel.appointments.emptySub', {
                      defaultValue: '稍后回来看看，或去浏览其他主播。',
                    })}
                  </span>
                </div>
              </div>
            )}
          </section>
        </div>
      )}

      {activeTab === 'posts' && (
        <section className="gl-library-section gl-channel-tab-panel" id="posts">
          <div className="gl-section-title-row">
            <div>
              <h2>{t('channel.posts.title', { defaultValue: '帖子动态' })}</h2>
              <span>
                {t('channel.posts.subtitle', {
                  defaultValue: '查看主播最近发布的帖子动态。',
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
                <strong>
                  {t('channel.posts.empty', { defaultValue: '暂时还没有帖子动态。' })}
                </strong>
                <span>
                  {t('channel.posts.emptySub', {
                    defaultValue: '主播发布后，这里会展示最新动态。',
                  })}
                </span>
              </div>
            </div>
          )}
        </section>
      )}

      {activeTab === 'history' && (
        <section className="gl-library-section gl-channel-tab-panel" id="history">
          <div className="gl-section-title-row">
            <h2>
              {isOwner
                ? t('channel.liveHistory')
                : t('channel.liveReplays', { defaultValue: 'Live replays' })}
            </h2>
          </div>
          {liveHistory.isPending ? (
            historyMode === 'replay' ? (
              <div className="gl-channel-replay-grid" aria-busy="true">
                {Array.from({ length: 4 }).map((_, i) => (
                  <div className="gl-home-replay-card is-loading" key={i} />
                ))}
              </div>
            ) : (
              <div className="gl-history-list" aria-busy="true">
                {Array.from({ length: 3 }).map((_, i) => (
                  <div className="gl-history-row is-loading" key={i} />
                ))}
              </div>
            )
          ) : historyItems.length ? (
            <>
              {historyMode === 'replay' ? (
                <div className="gl-channel-replay-grid">
                  {historyItems.map((record) => (
                    <ReplayCard
                      key={record.id}
                      replay={record}
                      channelName={channelName}
                      channelAvatar={channelAvatar}
                    />
                  ))}
                </div>
              ) : (
                <div className="gl-history-list">
                  {historyItems.map((record) => (
                    <ChannelHistoryRow
                      key={record.id}
                      record={record}
                      channelKey={channelKey}
                      isOwner={isOwner}
                    />
                  ))}
                </div>
              )}
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
                <strong>
                  {isOwner
                    ? t('channel.noHistory')
                    : t('channel.noReplays', { defaultValue: 'No live replays yet' })}
                </strong>
                <span>
                  {isOwner
                    ? t('channel.noHistoryOwnerSub')
                    : t('channel.noReplaysViewerSub', {
                        defaultValue: 'This creator has not published a replay yet.',
                      })}
                </span>
              </div>
            </div>
          )}
        </section>
      )}

      <FanBadgeConfirmDialog
        open={fanBadgeDialogOpen}
        channelName={channelName}
        canPurchase={Boolean(creatorId)}
        pending={joinFanClub.isPending}
        onOpenChange={setFanBadgeDialogOpen}
        onConfirm={handleConfirmFanBadge}
      />
      <AvatarUploadDialog open={avatarOpen} onOpenChange={setAvatarOpen} user={authUser} />
      <ChannelCoverUploadDialog open={coverOpen} onOpenChange={setCoverOpen} user={authUser} />
      <ReportDialog
        open={Boolean(reportTarget)}
        target={reportTarget}
        onOpenChange={(open) => {
          if (!open) setReportTarget(null);
        }}
      />
      <ShareDialog
        open={shareOpen}
        onOpenChange={setShareOpen}
        title={channelName}
        url={window.location.href}
        description={
          primary?.description ||
          profile?.displayName ||
          profile?.username ||
          t('channel.liveChannel')
        }
        previewImage={channelCover || primary?.cover}
        previewKicker={t('shareDialog.channelKicker', { defaultValue: 'Channel' })}
        previewMeta={t('channel.subscribers', { count: subscriberCount })}
      />
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
  record: ChannelHistoryItem;
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
              amount: formatNumber(record.peakViewers),
            })}
          </span>
        </div>
        <div className="gl-history-sub">
          <span>{record.category || t('channel.tabs.live')}</span>
          {/* Revenue and top fan are owner-only; the API omits them otherwise. */}
          {isOwner && record.revenueCoin !== undefined && (
            <>
              <span>
                {t('channel.history.revenue', { amount: formatCoin(record.revenueCoin) })}
              </span>
              <span>
                {record.topFan
                  ? t('channel.history.topFan', { name: record.topFan.name })
                  : t('channel.history.noFan')}
              </span>
            </>
          )}
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

function HistoryThumb({ record }: { record: ChannelHistoryItem }) {
  const initials = record.title.trim().slice(0, 2).toUpperCase() || 'GL';
  return (
    <div
      className={['gl-history-thumb', record.cover ? 'has-image' : ''].filter(Boolean).join(' ')}
    >
      <div className="gl-history-thumb-fallback">{initials}</div>
      {record.cover && <LoadableImage src={record.cover} alt="" />}
      <span className="gl-dur-pill">{record.duration}</span>
    </div>
  );
}

interface FanPreviewItem {
  id: string;
  name: string;
  avatar?: string;
  level?: number;
}

function FanClubBanner({
  channelName,
  fanPreviewItems,
  memberCount,
  pending,
  hasFanBadge,
  checkingFanBadge,
  isOwner,
  onJoin,
}: {
  channelName: string;
  fanPreviewItems: FanPreviewItem[];
  memberCount: number;
  pending: boolean;
  hasFanBadge: boolean;
  checkingFanBadge: boolean;
  isOwner: boolean;
  onJoin: () => void;
}) {
  const { t } = useTranslation('pages');
  const preview = fanPreviewItems.slice(0, 5);
  return (
    <section
      className="gl-fan-club-banner"
      aria-label={t('channel.fanClub.label', {
        channelName,
        defaultValue: `${channelName} 粉丝团`,
      })}
    >
      <div className="gl-fan-club-copy">
        <h2>{t('channel.fanClub.title', { defaultValue: '粉丝团' })}</h2>
        <p>
          {t('channel.fanClub.subtitle', {
            defaultValue: '感谢每一位点亮粉丝灯牌的观众。',
          })}
        </p>
        {!pending && memberCount > 0 && (
          <span className="gl-fan-club-copy-count">
            {t('channel.fanClub.memberCount', {
              count: memberCount,
              defaultValue: '{{count}} fans joined',
            })}
          </span>
        )}
      </div>
      <div className="gl-fan-club-side">
        <div className="gl-fan-club-preview">
          <div
            className="gl-fan-club-avatars"
            aria-label={t('channel.fanClub.members', { defaultValue: '粉丝灯牌成员' })}
          >
            {pending ? (
              Array.from({ length: 3 }).map((_, index) => (
                <span className="gl-fan-club-avatar-skeleton" key={index} />
              ))
            ) : preview.length ? (
              preview.map((fan) => (
                <div className="gl-fan-club-avatar-wrap" key={fan.id} title={fan.name}>
                  <Avatar name={fan.name} src={fan.avatar ?? ''} size={42} />
                  {fan.level && (
                    <span
                      className={cn(
                        'gl-fan-badge-level gl-fan-club-level',
                        fanBadgeToneClass(fan.level),
                      )}
                    >
                      <Crown size={11} strokeWidth={2.4} />
                      <span>#{fan.level}</span>
                    </span>
                  )}
                </div>
              ))
            ) : (
              <span className="gl-fan-club-empty-avatars">
                {t('channel.fanClub.emptyMembers', { defaultValue: '等待首位粉丝' })}
              </span>
            )}
          </div>
        </div>
        {!isOwner && (
          <button
            className={['gl-fan-club-join', hasFanBadge ? 'is-lit' : ''].filter(Boolean).join(' ')}
            type="button"
            disabled={hasFanBadge || checkingFanBadge}
            onClick={onJoin}
          >
            <BadgeCheck size={17} />
            {hasFanBadge
              ? t('channel.fanClub.lit', { defaultValue: '已点亮' })
              : t('channel.fanClub.join', { defaultValue: '加入粉丝团' })}
          </button>
        )}
      </div>
    </section>
  );
}

function FanBadgeConfirmDialog({
  open,
  channelName,
  canPurchase,
  pending,
  onOpenChange,
  onConfirm,
}: {
  open: boolean;
  channelName: string;
  canPurchase: boolean;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}) {
  const { t } = useTranslation('pages');
  const amount = formatNumber(FAN_BADGE_PRICE);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gl-fan-badge-dialog p-0 sm:max-w-[460px]">
        <div className="gl-fan-badge-dialog-body">
          <div className="gl-fan-badge-dialog-icon">
            <Coins size={24} />
          </div>
          <DialogTitle>
            {t('channel.fanBadge.title', {
              channelName,
              defaultValue: '加入 {{channelName}} 粉丝团',
            })}
          </DialogTitle>
          <DialogDescription>
            {t('channel.fanBadge.description', {
              amount,
              defaultValue: '确认花费 {{amount}} coins 购买粉丝灯牌。',
            })}
          </DialogDescription>
          <div className="gl-fan-badge-dialog-cost">
            <span>{t('channel.fanBadge.name', { defaultValue: '粉丝灯牌' })}</span>
            <strong>
              {t('channel.fanBadge.price', { amount, defaultValue: '{{amount}} coins' })}
            </strong>
          </div>
          {!canPurchase && (
            <p className="gl-fan-badge-dialog-note">
              {t('channel.fanBadge.noRoom', {
                defaultValue: '当前频道暂时没有可购买粉丝灯牌的直播间。',
              })}
            </p>
          )}
          <div className="gl-fan-badge-dialog-actions">
            <button
              className="gl-fan-badge-cancel"
              type="button"
              onClick={() => onOpenChange(false)}
            >
              {t('channel.fanBadge.cancel', { defaultValue: '取消' })}
            </button>
            <button
              className="gl-fan-badge-confirm"
              type="button"
              disabled={!canPurchase || pending}
              onClick={onConfirm}
            >
              {pending
                ? t('channel.fanBadge.pending', { defaultValue: '购买中...' })
                : t('channel.fanBadge.confirm', { defaultValue: '确认购买' })}
            </button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function buildFanPreviewItems({ members }: { members: FanClubMember[] }): FanPreviewItem[] {
  return members.slice(0, 5).map((member) => ({
    id: member.userId,
    name: member.name || member.username || member.userId,
    avatar: member.avatar || '',
    level: member.level,
  }));
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

function shouldHoldChannelIdentity({
  channelKey,
  profile,
  stream,
  profilePending,
  roomsPending,
}: {
  channelKey: string;
  profile: User | null;
  stream: Stream | undefined;
  profilePending: boolean;
  roomsPending: boolean;
}): boolean {
  if (profile || (!profilePending && !roomsPending)) return false;
  const key = channelKey.startsWith('ch-') ? channelKey.slice(3) : channelKey;
  if (!key || isUuidLike(key)) return true;
  if (isPlaceholderChannelName(key)) return true;
  return Boolean(stream && isPlaceholderChannelName(streamChannelName(stream)));
}

export function resolveChannelCover(profile: User | null): string {
  return profile?.cover || '';
}

// A stream's channel label is whatever its streamer sent, so it can copy
// another user's name. Once the key names a user, only that user's streams
// belong to the channel; labels count only for keys that name no user, and
// not while that is still being looked up.
export function matchesChannel(
  stream: Stream,
  key: string,
  profile: User | null,
  profilePending = false,
): boolean {
  if (profile) {
    const ownerID = profile.id.toLowerCase();
    return (
      stream.ownerId?.toLowerCase() === ownerID ||
      stream.channelId.toLowerCase() === `ch-${ownerID}`
    );
  }
  const normalized = key.toLowerCase();
  if (
    stream.channelId.toLowerCase() === normalized ||
    stream.ownerId?.toLowerCase() === normalized
  ) {
    return true;
  }
  return (
    !profilePending &&
    (stream.channel.toLowerCase() === normalized ||
      streamChannelName(stream).toLowerCase() === normalized)
  );
}

function normalizeChannelId(key: string): string {
  if (!key) return '';
  if (key.startsWith('ch-')) return key;
  return `ch-${key}`;
}

function normalizeCreatorId(id: string): string {
  if (!id) return '';
  return id.startsWith('ch-') ? id.slice(3) : id;
}

function formatChannelKey(key: string, t: ReturnType<typeof useTranslation>['t']): string {
  if (!key) return t('channel.title');
  if (!isUuidLike(key)) return key;
  return t('channel.creatorShort', { id: key.slice(0, 8) });
}

function formatHistoryDate(value: string, locale: string): string {
  return formatRelativeTime(value, locale);
}

function formatCoin(value: number): string {
  return `${formatNumber(Math.round(value))} coins`;
}
