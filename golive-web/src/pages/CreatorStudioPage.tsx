import { useEffect, useMemo, useState, type ReactNode } from 'react';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';
import { Navigate, NavLink, Outlet, useNavigate, useParams } from 'react-router-dom';
import {
  BarChart3,
  CheckCircle2,
  ClipboardCheck,
  Copy,
  Crown,
  CalendarClock,
  Eye,
  FileText,
  Gift,
  Globe2,
  History,
  ImagePlus,
  ListChecks,
  LockKeyhole,
  MessageSquare,
  Mic,
  MoreVertical,
  PlayCircle,
  Radio,
  Save,
  Send,
  ShieldCheck,
  Square,
  Timer,
  Trophy,
  Upload,
  Users,
  Zap,
  Bell,
  Check,
  Clock3,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Plus,
  X,
} from 'lucide-react';
import { toast } from 'sonner';
import { useMe } from '@/api/auth';
import { useSubmitCreatorApplication, useSubmitPlatformApplication } from '@/api/creator';
import {
  useCreatePost,
  useStudioPosts,
  useUploadPostImage,
  type PostCommentMode,
  type PostVisibility,
} from '@/api/posts';
import {
  useAddModerator,
  useModeratorFollowers,
  useModeratorLogs,
  useRemoveModerator,
  useRoomModerators,
  useRoomMuteState,
  useMuteRoomUser,
  useUnmuteRoomUser,
  type MuteUserPayload,
  type ModerationLog,
  type ModerationUser,
} from '@/api/moderation';
import {
  useFanGroups,
  useSyncFanGroups,
  useUpdateFanGroupMember,
  type FanGroup,
  type FanGroupMember,
} from '@/api/messages';
import {
  useCancelAppointment,
  useCreateAppointment,
  useCreatorAnalytics,
  useDeleteAppointmentRecord,
  useGoLive,
  useRoom,
  useRooms,
  useStartAppointment,
  useDeleteReplay,
  useStudioReplays,
  useStudioAppointments,
  useStopLive,
  useUpdateLiveReplaySettings,
  useUpdateReplay,
  useUpdateAppointment,
  useUpdateLiveMetadata,
  useUploadLiveCover,
  type AppointmentItem,
} from '@/api/room';
import { Avatar } from '@/components/Avatar';
import { AppointmentCard } from '@/components/AppointmentCard';
import { FanClubExclusiveBadge } from '@/components/FanClubExclusiveBadge';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Chat, type ChatModerationTarget } from '@/features/live-room/Chat';
import { ReportDialog, type ReportTargetDraft } from '@/features/reporting/ReportDialog';
import { BettingPanel } from '@/features/live-room/BettingPanel';
import { LuckyBagPanel } from '@/features/live-room/LuckyBagPanel';
import { MicLinkPanel } from '@/features/live-room/MicLinkPanel';
import { Player } from '@/features/live-room/Player';
import { useRoomRealtime } from '@/features/live-room/useRoomRealtime';
import {
  clearPublisherSession,
  loadPublisherSession,
  publisherSessionFromStream,
  savePublisherSession,
  type PublisherSession,
} from '@/features/creator/publisherSession';
import { useActiveCreatorLiveId } from '@/features/creator/useActiveCreatorLiveId';
import { CATEGORIES_EN } from '@/constants/catalog';
import { copyText } from '@/lib/clipboard';
import { cn } from '@/lib/cn';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useAuthHydrated, useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import { LoadableImage } from '@/components/LoadableImage';
import { PostCard } from '@/features/posts/PostCard';
import type { Message } from '@/types/message';
import type { ReplayVisibility, Stream } from '@/types/stream';
import { userDisplayName, type User } from '@/types/user';

const DEFAULT_CATEGORY = 'Just Chatting';
const LEGACY_APPOINTMENT_CATEGORY = 'Scheduled';
const CONSOLE_ACTIVITY_PAGE_SIZE = 5;

function categoryKey(category: string): string {
  return category.toLowerCase().replace(/\s+/g, '');
}

function appointmentCategoryDraft(category?: string): string {
  const normalized = category?.trim();
  if (!normalized || normalized === LEGACY_APPOINTMENT_CATEGORY) return DEFAULT_CATEGORY;
  return normalized;
}

function currentChannelKey(user: User | null | undefined): string {
  return user?.id ?? '';
}

function defaultAppointmentTime(): string {
  return toLocalDateTimeInput(new Date(Date.now() + 60 * 60 * 1000));
}

function isStreamLive(stream: Pick<Stream, 'isLive' | 'status'>): boolean {
  return stream.isLive === true || stream.status === 'live';
}

function mergeStreamSnapshot(
  roomStream: Stream | undefined,
  directoryStream: Stream | null,
): Stream | undefined {
  if (!roomStream) return directoryStream ?? undefined;
  if (!directoryStream) return roomStream;
  return {
    ...roomStream,
    ...directoryStream,
    ownerId: roomStream.ownerId ?? directoryStream.ownerId,
    streamKey: roomStream.streamKey ?? directoryStream.streamKey,
    playbackUrl: directoryStream.playbackUrl ?? roomStream.playbackUrl,
    isLive: true,
    status: directoryStream.status ?? 'live',
  };
}

function useStudioUser() {
  const storeUser = useAuthStore((s) => s.user);
  const me = useMe();
  return { user: me.data ?? storeUser, me };
}

function useActiveCreatorStream(user: User | null | undefined): { stream: { id: string } | null } {
  const activeLiveId = useActiveCreatorLiveId(user?.id);
  return { stream: activeLiveId ? { id: activeLiveId } : null };
}

export function CreatorStudioShell() {
  const { t } = useTranslation('pages');
  const hydrated = useAuthHydrated();
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const { user, me } = useStudioUser();
  const apply = useSubmitCreatorApplication();
  const status = user?.livePermissionStatus ?? 'none';
  const approved = status === 'approved';
  const restoringSession = hydrated && !isAuthed && Boolean(user);

  if (!hydrated || restoringSession || (isAuthed && me.isPending && !user)) {
    return <StudioLoading label={t('studio.loading', { defaultValue: 'Loading studio...' })} />;
  }

  if (!isAuthed) {
    return (
      <StudioAccessPage
        icon={<Radio size={24} />}
        title={t('studio.auth.title', { defaultValue: 'Sign in to open Creator Studio' })}
        body={t('studio.auth.body', {
          defaultValue:
            'Your dashboard, stream setup, and analytics are tied to your GoLive account.',
        })}
        actionLabel={t('studio.auth.action', { defaultValue: 'Sign in' })}
        onAction={() => openLogin()}
      />
    );
  }

  if (!approved) {
    return (
      <StudioPermissionPage
        status={status}
        rejectReason={user?.livePermissionRejectReason}
        applying={apply.isPending}
        onApply={(reason) => {
          apply.mutate(
            { reason },
            {
              onSuccess: (resp) => toast.success(resp.message),
              onError: (err) =>
                toast.error(err.message || t('createLive.errors.applicationFailed')),
            },
          );
        }}
      />
    );
  }

  return (
    <div className="gl-page gl-creator-studio-page">
      <StudioHeader user={user} />
      <StudioTabs />
      <Outlet />
    </div>
  );
}

export function CreatorStudioIndexPage() {
  return <Navigate to="/studio/overview" replace />;
}

export function CreatorStudioOverviewPage() {
  const { t } = useTranslation('pages');
  const navigate = useNavigate();
  const { user } = useStudioUser();
  const platformApply = useSubmitPlatformApplication();
  const channelKey = currentChannelKey(user);
  const analytics = useCreatorAnalytics(channelKey, Boolean(channelKey));
  const data = analytics.data;
  const latest = data?.history[0];
  const fanClubAudience =
    data?.fanBadgeDistribution.reduce((sum, bucket) => sum + bucket.fanCount, 0) ?? 0;
  const [platformDialogOpen, setPlatformDialogOpen] = useState(false);
  const liveApproved = user?.livePermissionStatus === 'approved';
  const platformStatus = user?.platformVerificationStatus ?? 'none';
  const platformApproved = platformStatus === 'approved';
  const platformPending = platformStatus === 'pending';
  const platformRejected = platformStatus === 'rejected';

  const submitPlatformApplication = (reason: string) => {
    platformApply.mutate(
      { reason },
      {
        onSuccess: (resp) => {
          toast.success(resp.message);
          setPlatformDialogOpen(false);
        },
        onError: (err) =>
          toast.error(
            err.message ||
              t('studio.platform.applyFailed', { defaultValue: 'Application failed.' }),
          ),
      },
    );
  };

  return (
    <div className="gl-creator-overview">
      <section className="gl-creator-kpis" aria-label={t('studio.overview.kpis')}>
        <StudioKpi
          icon={<Users size={18} />}
          label={t('studio.overview.cards.subscribers', { defaultValue: 'Subscribers' })}
          value={(data?.subscriberCount ?? 0).toLocaleString()}
          sub={t('studio.overview.cards.subscribersSub', { defaultValue: 'channel total' })}
        />
        <StudioKpi
          icon={<Crown size={18} />}
          label={t('studio.overview.cards.fanClubAudience', {
            defaultValue: 'Fan club audience',
          })}
          value={fanClubAudience.toLocaleString()}
          sub={t('studio.overview.cards.fanClubAudienceSub', {
            defaultValue: 'joined fans',
          })}
        />
        <StudioKpi
          icon={<Radio size={18} />}
          label={t('studio.overview.cards.streams', { defaultValue: 'Streams' })}
          value={(data?.streams ?? 0).toLocaleString()}
          sub={t('studio.overview.cards.streamsSub', { defaultValue: 'completed' })}
        />
        <StudioKpi
          icon={<Zap size={18} />}
          label={t('studio.overview.cards.revenue', { defaultValue: 'Revenue' })}
          value={formatCoins(data?.revenueCoin ?? 0)}
          sub={t('studio.overview.cards.revenueSub', { defaultValue: 'live income' })}
        />
      </section>

      <section className="gl-creator-dashboard-grid">
        <div className="gl-creator-panel">
          <div className="gl-creator-panel-head">
            <div>
              <span>{t('studio.overview.quickTitle', { defaultValue: 'Quick actions' })}</span>
              <h2>
                {t('studio.overview.quickHeading', { defaultValue: 'Prepare your next live' })}
              </h2>
            </div>
            <ListChecks size={22} />
          </div>
          <div className="gl-creator-actions-grid">
            <StudioAction
              icon={<PlayCircle size={18} />}
              title={t('studio.actions.prepare', { defaultValue: 'Open stream setup' })}
              body={t('studio.actions.prepareSub', {
                defaultValue: 'Title, cover, category, and OBS checks.',
              })}
              onClick={() => navigate('/studio/prepare')}
            />
            <StudioAction
              icon={<BarChart3 size={18} />}
              title={t('studio.actions.analytics', { defaultValue: 'Review analytics' })}
              body={t('studio.actions.analyticsSub', {
                defaultValue: 'Revenue, viewers, and finished live reports.',
              })}
              onClick={() => navigate(`/studio/analytics/${encodeURIComponent(channelKey)}`)}
            />
            <StudioAction
              icon={<Users size={18} />}
              title={t('studio.actions.channel', { defaultValue: 'View channel' })}
              body={t('studio.actions.channelSub', {
                defaultValue: 'Check how viewers see your creator page.',
              })}
              onClick={() => navigate(`/channel/${encodeURIComponent(channelKey)}`)}
            />
            <StudioAction
              icon={platformApproved ? <CheckCircle2 size={18} /> : <ShieldCheck size={18} />}
              title={
                !liveApproved
                  ? t('studio.actions.platformLocked', { defaultValue: 'Join the platform' })
                  : platformApproved
                    ? t('studio.actions.platformApproved', { defaultValue: 'Platform certified' })
                    : platformPending
                      ? t('studio.actions.platformPending', { defaultValue: 'Application pending' })
                      : t('studio.actions.platform', { defaultValue: 'Join the platform' })
              }
              body={
                !liveApproved
                  ? t('studio.actions.platformLockedSub', {
                      defaultValue: 'Live permission is required before platform certification.',
                    })
                  : platformApproved
                    ? t('studio.actions.platformApprovedSub', {
                        defaultValue:
                          'Certified creators get lower withdrawal fees and platform support.',
                      })
                    : platformPending
                      ? t('studio.actions.platformPendingSub', {
                          defaultValue:
                            'Admins are reviewing your platform certification application.',
                        })
                      : platformRejected
                        ? t('studio.actions.platformRejectedSub', {
                            defaultValue: 'Apply again after improving your channel profile.',
                          })
                        : t('studio.actions.platformSub', {
                            defaultValue:
                              'Apply for certification, extra protection, and recommendation.',
                          })
              }
              className="is-platform"
              disabled={
                !liveApproved || platformApproved || platformPending || platformApply.isPending
              }
              onClick={() => {
                if (!liveApproved) {
                  toast.info(
                    t('studio.platform.liveRequired', {
                      defaultValue: 'Apply for live permission before joining the platform.',
                    }),
                  );
                  return;
                }
                if (platformApproved) {
                  toast.info(
                    t('studio.platform.alreadyApproved', {
                      defaultValue: 'Your channel is already platform certified.',
                    }),
                  );
                  return;
                }
                if (platformPending) {
                  toast.info(
                    t('studio.platform.alreadyPending', {
                      defaultValue: 'Your application is already waiting for admin review.',
                    }),
                  );
                  return;
                }
                setPlatformDialogOpen(true);
              }}
            />
          </div>
        </div>

        <div className="gl-creator-panel">
          <div className="gl-creator-panel-head">
            <div>
              <span>{t('studio.overview.recentTitle', { defaultValue: 'Latest result' })}</span>
              <h2>
                {latest?.title ?? t('studio.overview.noRecent', { defaultValue: 'No stream yet' })}
              </h2>
            </div>
            <Trophy size={22} />
          </div>
          {analytics.isPending ? (
            <div className="gl-creator-muted-line">
              {t('studio.loading', { defaultValue: 'Loading studio...' })}
            </div>
          ) : latest ? (
            <div className="gl-creator-latest">
              <div className={cn('gl-creator-latest-cover', latest.cover && 'has-image')}>
                {latest.cover && <LoadableImage src={latest.cover} alt="" />}
                <span>{latest.duration}</span>
              </div>
              <div className="gl-creator-latest-meta">
                <span>
                  {t('studio.overview.latestPeak', {
                    count: latest.peakViewers,
                    defaultValue: '{{count}} peak viewers',
                  })}
                </span>
                <span>{formatCoins(latest.revenueCoin)}</span>
              </div>
            </div>
          ) : (
            <div className="gl-creator-empty-soft">
              {t('studio.overview.emptyRecent', {
                defaultValue: 'Start a live and the recap will appear here.',
              })}
            </div>
          )}
        </div>
      </section>
      <JoinPlatformDialog
        open={platformDialogOpen}
        pending={platformApply.isPending}
        rejectReason={user?.platformVerificationRejectReason}
        onOpenChange={setPlatformDialogOpen}
        onSubmit={submitPlatformApplication}
      />
    </div>
  );
}

export function CreatorPreparePage() {
  const { t } = useTranslation('pages');
  const navigate = useNavigate();
  const { user } = useStudioUser();
  const activeLive = useActiveCreatorStream(user);
  const uploadCover = useUploadLiveCover();
  const goLive = useGoLive();
  const channelKey = currentChannelKey(user);
  const analytics = useCreatorAnalytics(channelKey, Boolean(channelKey));
  const lastLive = analytics.data?.history[0] ?? null;
  const [title, setTitle] = useState(() => t('createLive.defaultTitle'));
  const [description, setDescription] = useState('');
  const [category, setCategory] = useState(DEFAULT_CATEGORY);
  const [fanClubOnly, setFanClubOnly] = useState(false);
  const [coverFile, setCoverFile] = useState<File | null>(null);
  const [coverPreview, setCoverPreview] = useState('');
  const [step, setStep] = useState(0);
  const [obsChecked, setObsChecked] = useState(false);
  const categories = CATEGORIES_EN.filter((item) => item !== 'All');
  const appointments = useStudioAppointments(Boolean(user), 1, 5);
  const dueAppointment = appointments.data?.items.find(
    (item) => item.status === 'scheduled' && item.canStart,
  );

  useEffect(() => {
    return () => {
      if (coverPreview.startsWith('blob:')) URL.revokeObjectURL(coverPreview);
    };
  }, [coverPreview]);

  const done1 = title.trim().length > 0 && description.trim().length > 0;
  const done2 = done1 && Boolean(category) && Boolean(coverPreview);
  const done3 = done2 && obsChecked && user?.livePermissionStatus === 'approved';
  const starting = goLive.isPending || uploadCover.isPending;

  if (activeLive.stream) {
    return <Navigate to={`/studio/live/${encodeURIComponent(activeLive.stream.id)}`} replace />;
  }

  const applyLastLiveInfo = () => {
    if (!lastLive) return;
    setTitle(lastLive.title || t('createLive.defaultTitle'));
    setDescription(lastLive.description ?? '');
    setCategory(lastLive.category || DEFAULT_CATEGORY);
    setFanClubOnly(Boolean(lastLive.fanClubOnly));
    setCoverFile(null);
    setCoverPreview(lastLive.cover || '');
    toast.success(
      t('studio.prepare.reuseLastDone', {
        defaultValue: 'Reused the previous live info.',
      }),
    );
  };

  const startLive = async () => {
    if (!done3 || !user) return;
    if (dueAppointment) {
      toast.info(
        t('studio.prepare.appointmentRequired', {
          defaultValue: 'A scheduled appointment is ready. Please start from Live appointments.',
        }),
      );
      navigate('/studio/appointments');
      return;
    }
    try {
      const uploadedCover = coverFile
        ? (await uploadCover.mutateAsync(coverFile)).url
        : coverPreview;
      goLive.mutate(
        {
          title: title.trim(),
          description: description.trim(),
          category,
          cover: uploadedCover,
          channelName: userDisplayName(user),
          avatar: user.avatar,
          fanClubOnly,
        },
        {
          onSuccess: (stream) => {
            savePublisherSession(stream);
            toast.success(
              t('studio.prepare.created', {
                defaultValue: 'Live room created. Open OBS and begin publishing.',
              }),
            );
            navigate(`/studio/live/${encodeURIComponent(stream.id)}`);
          },
          onError: (err) => toast.error(err.message || t('createLive.errors.startFailed')),
        },
      );
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('createLive.errors.coverUpload'));
    }
  };

  return (
    <div className="gl-creator-prepare">
      <section className="gl-creator-flow">
        <StepCard
          number={1}
          active={step === 1}
          done={done1}
          locked={false}
          title={t('studio.prepare.steps.info', { defaultValue: 'Fill live info' })}
          actionLabel={
            done1 ? undefined : t('studio.prepare.startWriting', { defaultValue: '开始填写' })
          }
          onOpen={() => setStep(1)}
        >
          <div className="gl-creator-step-tools">
            <button
              className="gl-creator-secondary"
              type="button"
              disabled={!lastLive}
              onClick={applyLastLiveInfo}
            >
              <History size={16} />
              {lastLive
                ? t('studio.prepare.reuseLast', { defaultValue: 'Reuse last live info' })
                : analytics.isPending
                  ? t('studio.prepare.loadingLast', { defaultValue: 'Loading last live...' })
                  : t('studio.prepare.noLastLive', { defaultValue: 'No previous live yet' })}
            </button>
            {lastLive && (
              <span>
                {t('studio.prepare.reuseLastHint', {
                  title: lastLive.title,
                  defaultValue: 'Use the title, description, category, and cover from "{{title}}".',
                })}
              </span>
            )}
          </div>
          <label className="gl-creator-field">
            <span>{t('createLive.fields.title')}</span>
            <input
              value={title}
              maxLength={120}
              onChange={(event) => setTitle(event.target.value)}
            />
          </label>
          <label className="gl-creator-field">
            <span>{t('createLive.fields.description')}</span>
            <textarea
              value={description}
              rows={4}
              maxLength={2000}
              placeholder={t('studio.prepare.descriptionPlaceholder', {
                defaultValue: 'Tell viewers what this live is about.',
              })}
              onChange={(event) => setDescription(event.target.value)}
            />
          </label>
          <button
            className="gl-creator-primary"
            type="button"
            disabled={!done1}
            onClick={() => setStep(2)}
          >
            {t('studio.prepare.next', { defaultValue: 'Next' })}
          </button>
        </StepCard>

        <StepCard
          number={2}
          active={step === 2}
          done={done2}
          locked={!done1}
          title={t('studio.prepare.steps.cover', { defaultValue: 'Choose cover and category' })}
          onOpen={() => done1 && setStep(2)}
        >
          <CategoryPicker categories={categories} value={category} onChange={setCategory} />
          <CoverPicker
            preview={coverPreview}
            onChange={(file) => {
              setCoverFile(file);
              setCoverPreview(file ? URL.createObjectURL(file) : '');
            }}
          />
          <FanClubOnlyToggle checked={fanClubOnly} onChange={setFanClubOnly} />
          <button
            className="gl-creator-primary"
            type="button"
            disabled={!done2}
            onClick={() => setStep(3)}
          >
            {t('studio.prepare.next', { defaultValue: 'Next' })}
          </button>
        </StepCard>

        <StepCard
          number={3}
          active={step === 3}
          done={done3}
          locked={!done2}
          title={t('studio.prepare.steps.check', {
            defaultValue: 'Check publishing and permission',
          })}
          onOpen={() => done2 && setStep(3)}
        >
          <div className="gl-creator-check-list">
            <CheckRow
              ok={user?.livePermissionStatus === 'approved'}
              label={t('studio.prepare.permissionApproved', {
                defaultValue: 'Creator permission approved',
              })}
            />
            <CheckRow
              ok={obsChecked}
              label={t('studio.prepare.obsReady', { defaultValue: 'OBS setup reviewed' })}
            />
          </div>
          <PublisherPreview rtmpServer={rtmpServer()} />
          <button
            className="gl-creator-secondary"
            type="button"
            onClick={() => setObsChecked(true)}
          >
            <ClipboardCheck size={16} />
            {t('studio.prepare.runCheck', { defaultValue: 'Mark OBS check complete' })}
          </button>
          <button
            className="gl-creator-primary"
            type="button"
            disabled={!done3}
            onClick={() => setStep(4)}
          >
            {t('studio.prepare.next', { defaultValue: 'Next' })}
          </button>
        </StepCard>

        <StepCard
          number={4}
          active={step === 4}
          done={false}
          locked={!done3}
          title={t('studio.prepare.steps.confirm', { defaultValue: 'Confirm and start live' })}
          onOpen={() => done3 && setStep(4)}
        >
          <div className="gl-creator-confirm">
            <strong>
              {t('studio.prepare.readyTitle', { defaultValue: 'Ready to create the live room' })}
            </strong>
            <span>
              {t('studio.prepare.readyBody', {
                defaultValue:
                  'After confirmation, your stream key is issued and the live control console opens.',
              })}
            </span>
          </div>
          {dueAppointment && (
            <div className="gl-creator-confirm">
              <strong>
                {t('studio.prepare.appointmentRequiredTitle', {
                  defaultValue: 'Start from your appointment',
                })}
              </strong>
              <span>{dueAppointment.title}</span>
            </div>
          )}
          <button
            className="gl-creator-start"
            type="button"
            disabled={!done3 || starting}
            onClick={() => void startLive()}
          >
            <Radio size={17} />
            {starting
              ? t('createLive.starting')
              : t('studio.prepare.startLive', { defaultValue: 'Start live' })}
          </button>
        </StepCard>
      </section>

      <aside className="gl-creator-preview-panel">
        <div className="gl-creator-panel-head">
          <div>
            <span>{t('studio.prepare.previewLabel', { defaultValue: 'Preview' })}</span>
            <h2>{t('studio.prepare.previewTitle', { defaultValue: 'Before going live' })}</h2>
          </div>
          <Eye size={22} />
        </div>
        <LiveSetupPreview
          title={title}
          description={description}
          category={category}
          cover={coverPreview}
          status={t('studio.prepare.waitingStatus', { defaultValue: 'Waiting to go live' })}
          fanClubOnly={fanClubOnly}
        />
      </aside>
    </div>
  );
}

export function CreatorReplayPage() {
  const { user } = useStudioUser();
  const channelKey = currentChannelKey(user);
  if (!channelKey) return null;
  return <Navigate to={`/studio/analytics/${encodeURIComponent(channelKey)}`} replace />;
}

export function CreatorLiveReplaysPage() {
  const { t, i18n } = useTranslation('pages');
  const { user } = useStudioUser();
  const [page, setPage] = useState(1);
  const replays = useStudioReplays(Boolean(user), page, 10);
  const total = replays.data?.total ?? 0;
  const pageSize = replays.data?.size ?? 10;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));

  useEffect(() => {
    if (page > pageCount) setPage(pageCount);
  }, [page, pageCount]);

  return (
    <div className="gl-creator-replay-page">
      <section className="gl-creator-panel">
        <div className="gl-creator-panel-head">
          <div>
            <span>{t('studio.replay.label', { defaultValue: 'Live replay' })}</span>
            <h2>{t('studio.replay.title', { defaultValue: 'Manage completed live replays' })}</h2>
          </div>
          <PlayCircle size={22} />
        </div>

        {replays.isPending ? (
          <div className="gl-creator-muted-line">
            {t('studio.loading', { defaultValue: 'Loading studio...' })}
          </div>
        ) : replays.data?.items.length ? (
          <>
            <div className="gl-replay-manage-list">
              {replays.data.items.map((stream) => (
                <ReplayManageRow key={stream.id} stream={stream} locale={i18n.language} />
              ))}
            </div>
            {pageCount > 1 && (
              <div className="gl-history-pager gl-appointment-pager">
                <div className="gl-history-pager-count">
                  {t('studio.replay.pageCount', {
                    page,
                    pageCount,
                    total,
                    defaultValue: '{{page}} / {{pageCount}} · {{total}} replays',
                  })}
                </div>
                <div className="gl-history-pager-controls">
                  <button
                    type="button"
                    disabled={page <= 1}
                    onClick={() => setPage((value) => Math.max(1, value - 1))}
                  >
                    <ChevronLeft size={16} />
                  </button>
                  <button
                    type="button"
                    disabled={page >= pageCount}
                    onClick={() => setPage((value) => Math.min(pageCount, value + 1))}
                  >
                    <ChevronRight size={16} />
                  </button>
                </div>
              </div>
            )}
          </>
        ) : (
          <div className="gl-creator-empty-soft">
            {t('studio.replay.empty', {
              defaultValue:
                'No replay has been uploaded yet. Enable replay upload in a live console before ending the live.',
            })}
          </div>
        )}
      </section>
    </div>
  );
}

function ReplayManageRow({ stream, locale }: { stream: Stream; locale: string }) {
  const { t } = useTranslation('pages');
  const replay = stream.replay;
  const [visibility, setVisibility] = useState<ReplayVisibility>(replay?.visibility ?? 'public');
  const updateReplay = useUpdateReplay(stream.id);
  const deleteReplay = useDeleteReplay(stream.id);

  useEffect(() => {
    setVisibility(replay?.visibility ?? 'public');
  }, [replay?.visibility]);

  const status = replayStatusLabel(replay?.status ?? 'none', t);
  const uploadedAt = replay?.uploadedAt
    ? new Intl.DateTimeFormat(locale, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      }).format(new Date(replay.uploadedAt))
    : '';
  const peakViewers = stream.peakViewers ?? stream.viewers;
  const saveVisibility = () => {
    updateReplay.mutate(
      { visibility },
      {
        onSuccess: () =>
          toast.success(
            t('studio.replay.visibilitySaved', { defaultValue: 'Replay visibility updated.' }),
          ),
        onError: (err) =>
          toast.error(
            err.message ||
              t('studio.replay.visibilityFailed', { defaultValue: 'Could not update replay.' }),
          ),
      },
    );
  };
  const remove = () => {
    if (
      !window.confirm(t('studio.replay.deleteConfirm', { defaultValue: 'Delete this replay?' }))
    ) {
      return;
    }
    deleteReplay.mutate(undefined, {
      onSuccess: () =>
        toast.success(t('studio.replay.deleted', { defaultValue: 'Replay deleted.' })),
      onError: (err) =>
        toast.error(
          err.message ||
            t('studio.replay.deleteFailed', { defaultValue: 'Could not delete replay.' }),
        ),
    });
  };

  return (
    <article className="gl-replay-manage-row">
      <div className={cn('gl-replay-manage-cover', stream.cover && 'has-image')}>
        {stream.cover && <LoadableImage src={stream.cover} alt="" />}
        <span>{stream.duration}</span>
      </div>
      <div className="gl-replay-manage-main">
        <div className="gl-replay-manage-title">
          <strong>{stream.title}</strong>
          <span className={`gl-replay-status is-${replay?.status ?? 'none'}`}>{status}</span>
        </div>
        <div className="gl-replay-manage-meta">
          <span>{uploadedAt || stream.category}</span>
          <span>
            {t('studio.replay.peakViewers', {
              amount: peakViewers.toLocaleString(locale),
              defaultValue: '{{amount}} peak',
            })}
          </span>
          {replay?.error && <span className="is-error">{replay.error}</span>}
        </div>
        <div className="gl-replay-manage-controls">
          <VisibilitySelect value={visibility} onChange={setVisibility} />
          <button
            type="button"
            className="gl-creator-secondary"
            disabled={updateReplay.isPending || visibility === replay?.visibility}
            onClick={saveVisibility}
          >
            <Save size={15} />
            {t('studio.replay.saveVisibility', { defaultValue: 'Save' })}
          </button>
        </div>
      </div>
      <div className="gl-replay-manage-actions">
        {replay?.canWatch && (
          <NavLink className="gl-creator-secondary" to={`/live/${encodeURIComponent(stream.id)}`}>
            <PlayCircle size={15} />
            {t('studio.replay.open', { defaultValue: 'Open' })}
          </NavLink>
        )}
        <button
          type="button"
          className="gl-creator-secondary is-danger"
          disabled={deleteReplay.isPending || replay?.status === 'deleted'}
          onClick={remove}
        >
          <TrashIcon />
          {t('studio.replay.delete', { defaultValue: 'Delete' })}
        </button>
      </div>
    </article>
  );
}

function VisibilitySelect({
  value,
  onChange,
}: {
  value: ReplayVisibility;
  onChange: (value: ReplayVisibility) => void;
}) {
  const { t } = useTranslation('pages');
  const options = replayVisibilityOptions(t);
  return (
    <div className="gl-replay-visibility-control">
      <span>{t('studio.replay.visibilityLabel', { defaultValue: 'Visibility' })}</span>
      <div
        className="gl-post-visibility-edit gl-replay-visibility-edit"
        aria-label={t('studio.replay.visibilityLabel', { defaultValue: 'Visibility' })}
      >
        {options.map((option) => (
          <button
            key={option.value}
            type="button"
            className={value === option.value ? 'is-active' : undefined}
            onClick={() => onChange(option.value)}
          >
            <option.Icon size={14} />
            {option.label}
          </button>
        ))}
      </div>
    </div>
  );
}

function replayVisibilityOptions(t: TFunction): Array<{
  value: ReplayVisibility;
  label: string;
  Icon: typeof Globe2;
}> {
  return [
    {
      value: 'public',
      label: t('studio.replay.visibility.publicShort', { defaultValue: 'Public' }),
      Icon: Globe2,
    },
    {
      value: 'followers',
      label: t('studio.replay.visibility.followersShort', { defaultValue: 'Followers' }),
      Icon: Users,
    },
    {
      value: 'private',
      label: t('studio.replay.visibility.privateShort', { defaultValue: 'Only me' }),
      Icon: LockKeyhole,
    },
  ];
}

function replayStatusLabel(status: string, t: TFunction): string {
  switch (status) {
    case 'pending':
      return t('studio.replay.status.pending', { defaultValue: 'Pending upload' });
    case 'uploading':
      return t('studio.replay.status.uploading', { defaultValue: 'Uploading' });
    case 'processing':
      return t('studio.replay.status.processing', { defaultValue: 'Processing' });
    case 'ready':
      return t('studio.replay.status.ready', { defaultValue: 'Ready' });
    case 'failed':
      return t('studio.replay.status.failed', { defaultValue: 'Failed' });
    case 'deleted':
      return t('studio.replay.status.deleted', { defaultValue: 'Deleted' });
    default:
      return t('studio.replay.status.none', { defaultValue: 'Not uploaded' });
  }
}

function TrashIcon() {
  return <X size={15} />;
}

const APPOINTMENT_STATS_PAGE_SIZE = 100;
const APPOINTMENT_LIST_PAGE_SIZE = 4;
const STUDIO_POST_PAGE_SIZE = 6;

interface PostImageDraft {
  id: string;
  file: File;
  preview: string;
}

export function CreatorAppointmentsPage() {
  const { t } = useTranslation('pages');
  const { user } = useStudioUser();
  const channelKey = currentChannelKey(user);
  const [appointmentPage, setAppointmentPage] = useState(1);
  const appointmentStats = useStudioAppointments(
    Boolean(channelKey),
    1,
    APPOINTMENT_STATS_PAGE_SIZE,
  );
  const appointments = useStudioAppointments(
    Boolean(channelKey),
    appointmentPage,
    APPOINTMENT_LIST_PAGE_SIZE,
  );
  const createAppointment = useCreateAppointment();
  const uploadCover = useUploadLiveCover();
  const [appointmentDialogOpen, setAppointmentDialogOpen] = useState(false);
  const [editing, setEditing] = useState<AppointmentItem | null>(null);
  const updateAppointment = useUpdateAppointment(editing?.id ?? '');
  const [scheduledAt, setScheduledAt] = useState(() => defaultAppointmentTime());
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [category, setCategory] = useState(DEFAULT_CATEGORY);
  const [fanClubOnly, setFanClubOnly] = useState(false);
  const [coverFile, setCoverFile] = useState<File | null>(null);
  const [coverPreview, setCoverPreview] = useState('');
  const [error, setError] = useState('');
  const categories = CATEGORIES_EN.filter((item) => item !== 'All');

  useEffect(() => {
    setAppointmentPage(1);
  }, [channelKey]);

  useEffect(() => {
    return () => {
      if (coverPreview.startsWith('blob:')) URL.revokeObjectURL(coverPreview);
    };
  }, [coverPreview]);

  const clearDraft = () => {
    setEditing(null);
    setTitle('');
    setDescription('');
    setCategory(DEFAULT_CATEGORY);
    setFanClubOnly(false);
    setScheduledAt(defaultAppointmentTime());
    setCoverFile(null);
    setCoverPreview('');
    setError('');
  };

  const resetDraft = () => {
    if (!editing) {
      clearDraft();
      return;
    }
    setTitle(editing.title);
    setDescription(editing.description ?? '');
    setCategory(appointmentCategoryDraft(editing.category));
    setFanClubOnly(Boolean(editing.fanClubOnly));
    setScheduledAt(toLocalDateTimeInput(editing.scheduledAt));
    setCoverFile(null);
    setCoverPreview(editing.cover ?? '');
    setError('');
  };

  const openCreateDialog = () => {
    clearDraft();
    setAppointmentDialogOpen(true);
  };

  const openEditDialog = (item: AppointmentItem) => {
    setEditing(item);
    setTitle(item.title);
    setDescription(item.description ?? '');
    setCategory(appointmentCategoryDraft(item.category));
    setFanClubOnly(Boolean(item.fanClubOnly));
    setScheduledAt(toLocalDateTimeInput(item.scheduledAt));
    setCoverFile(null);
    setCoverPreview(item.cover ?? '');
    setError('');
    setAppointmentDialogOpen(true);
  };

  const handleAppointmentDialogOpenChange = (open: boolean) => {
    setAppointmentDialogOpen(open);
    if (!open) clearDraft();
  };

  const saveAppointment = async () => {
    if (!user) return;
    const scheduled = new Date(scheduledAt);
    const normalizedTitle = title.trim();
    const normalizedDescription = description.trim();
    if (Number.isNaN(scheduled.getTime())) {
      setError(
        t('studio.appointments.invalidTime', { defaultValue: 'Please pick a valid future time.' }),
      );
      return;
    }
    if (scheduled.getTime() <= Date.now()) {
      setError(
        t('studio.appointments.invalidTime', { defaultValue: 'Please pick a valid future time.' }),
      );
      return;
    }
    if (!normalizedTitle || !normalizedDescription || !category || !coverPreview.trim()) {
      setError(
        t('studio.appointments.formIncomplete', {
          defaultValue: 'Start time, title, category, cover, and description are required.',
        }),
      );
      return;
    }
    setError('');
    try {
      const cover = coverFile ? (await uploadCover.mutateAsync(coverFile)).url : coverPreview;
      const payload = {
        scheduledAt: scheduled.toISOString(),
        title: normalizedTitle,
        description: normalizedDescription,
        category,
        cover,
        channelName: userDisplayName(user),
        avatar: user.avatar,
        fanClubOnly,
      };
      if (!editing) {
        await createAppointment.mutateAsync(payload);
        toast.success(t('studio.appointments.created', { defaultValue: 'Appointment published.' }));
        setAppointmentPage(1);
      } else {
        await updateAppointment.mutateAsync(payload);
        toast.success(t('studio.appointments.updated', { defaultValue: 'Appointment updated.' }));
      }
      setAppointmentDialogOpen(false);
      clearDraft();
      void appointments.refetch();
      void appointmentStats.refetch();
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : t('studio.appointments.saveFailed', {
              defaultValue: 'Could not save the appointment.',
            }),
      );
    }
  };

  const items = appointments.data?.items ?? [];
  const statsItems = appointmentStats.data?.items ?? items;
  const total = appointmentStats.data?.total ?? appointments.data?.total ?? 0;
  const appointmentPageCount = Math.max(
    1,
    Math.ceil((appointments.data?.total ?? 0) / APPOINTMENT_LIST_PAGE_SIZE),
  );
  const upcoming = statsItems.filter((item) => item.status === 'scheduled').length;
  const live = statsItems.filter((item) => item.status === 'live').length;
  const completed = statsItems.filter((item) => item.status === 'completed').length;

  useEffect(() => {
    if (!appointments.data) return;
    const nextPageCount = Math.max(
      1,
      Math.ceil(appointments.data.total / APPOINTMENT_LIST_PAGE_SIZE),
    );
    if (appointmentPage > nextPageCount) {
      setAppointmentPage(nextPageCount);
    }
  }, [appointmentPage, appointments.data]);

  return (
    <div className="gl-creator-appointments">
      <section className="gl-creator-kpis">
        <StudioKpi
          icon={<CalendarClock size={18} />}
          label={t('studio.appointments.total', { defaultValue: 'Appointments' })}
          value={String(total)}
          sub={t('studio.appointments.totalSub', { defaultValue: 'all records' })}
        />
        <StudioKpi
          icon={<Clock3 size={18} />}
          label={t('studio.appointments.upcoming', { defaultValue: 'Upcoming' })}
          value={String(upcoming)}
          sub={t('studio.appointments.upcomingSub', { defaultValue: 'scheduled' })}
        />
        <StudioKpi
          icon={<Radio size={18} />}
          label={t('studio.appointments.live', { defaultValue: 'Live now' })}
          value={String(live)}
          sub={t('studio.appointments.liveSub', { defaultValue: 'started' })}
        />
        <StudioKpi
          icon={<CheckCircle2 size={18} />}
          label={t('studio.appointments.completed', { defaultValue: 'Completed' })}
          value={String(completed)}
          sub={t('studio.appointments.completedSub', { defaultValue: 'finished or expired' })}
        />
      </section>

      <div className="gl-appointments-toolbar">
        <button
          type="button"
          className="gl-creator-primary gl-appointments-create-btn"
          onClick={openCreateDialog}
        >
          <Plus size={16} />
          {t('studio.appointments.createTitle', { defaultValue: 'Create appointment' })}
        </button>
      </div>

      <section className="gl-creator-dashboard-grid gl-appointments-grid">
        <div className="gl-creator-panel gl-appointments-list-panel">
          <div className="gl-creator-panel-head">
            <div>
              <span>{t('studio.appointments.listLabel', { defaultValue: 'My appointments' })}</span>
              <h2>
                {t('studio.appointments.listTitle', { defaultValue: 'All appointment states' })}
              </h2>
            </div>
            <Bell size={22} />
          </div>
          <div className="gl-appointment-list-wrap">
            <div className="gl-appointment-list">
              {appointments.isPending ? (
                <div className="gl-creator-empty-soft">
                  {t('studio.loading', { defaultValue: 'Loading studio...' })}
                </div>
              ) : items.length === 0 ? (
                <div className="gl-creator-empty-soft">
                  {t('studio.appointments.empty', { defaultValue: 'No appointments yet.' })}
                </div>
              ) : (
                items.map((item) => (
                  <AppointmentStudioRow
                    key={item.id}
                    item={item}
                    onEdit={() => openEditDialog(item)}
                    onUpdated={() => {
                      void appointments.refetch();
                      void appointmentStats.refetch();
                    }}
                  />
                ))
              )}
            </div>
            {appointments.data && appointments.data.total > APPOINTMENT_LIST_PAGE_SIZE && (
              <StudioAppointmentPager
                page={appointmentPage}
                pageCount={appointmentPageCount}
                total={appointments.data.total}
                pageSize={APPOINTMENT_LIST_PAGE_SIZE}
                onPageChange={setAppointmentPage}
              />
            )}
          </div>
        </div>
      </section>

      <Dialog open={appointmentDialogOpen} onOpenChange={handleAppointmentDialogOpenChange}>
        <DialogContent className="gl-appointment-dialog max-w-none overflow-visible p-0 sm:max-w-none">
          <div className="gl-appointment-form-panel gl-appointment-dialog-panel">
            <div className="gl-creator-panel-head">
              <div>
                <span>
                  {t('studio.appointments.formLabel', { defaultValue: 'Live appointments' })}
                </span>
                <DialogTitle asChild>
                  <h2>
                    {editing
                      ? t('studio.appointments.editTitle', { defaultValue: 'Edit appointment' })
                      : t('studio.appointments.createTitle', {
                          defaultValue: 'Create appointment',
                        })}
                  </h2>
                </DialogTitle>
              </div>
            </div>

            <div className="gl-appointment-form-body">
              <div className="gl-appointment-form-fields">
                <div className="gl-appointment-form-row">
                  <div className="gl-creator-field">
                    <span>{t('studio.appointments.time', { defaultValue: 'Start time' })}</span>
                    <AppointmentDateTimePicker value={scheduledAt} onChange={setScheduledAt} />
                  </div>
                  <label className="gl-creator-field">
                    <span>{t('createLive.fields.title')}</span>
                    <input
                      value={title}
                      maxLength={120}
                      onChange={(event) => setTitle(event.target.value)}
                    />
                  </label>
                  <div className="gl-creator-field gl-appointment-category-field">
                    <span>{t('createLive.fields.category')}</span>
                    <AppointmentCategorySelect
                      categories={categories}
                      value={category}
                      onChange={setCategory}
                      label={t('createLive.fields.category')}
                    />
                  </div>
                </div>
                <label className="gl-creator-field">
                  <span>{t('createLive.fields.description')}</span>
                  <textarea
                    value={description}
                    rows={4}
                    maxLength={2000}
                    placeholder={t('studio.appointments.descriptionPlaceholder', {
                      defaultValue: 'Tell viewers what this appointment is about.',
                    })}
                    onChange={(event) => setDescription(event.target.value)}
                  />
                </label>
                <FanClubOnlyToggle
                  checked={fanClubOnly}
                  onChange={setFanClubOnly}
                  title={t('studio.appointments.fanClubOnly', {
                    defaultValue: 'Fan club exclusive appointment',
                  })}
                  description={t('studio.appointments.fanClubOnlySub', {
                    defaultValue:
                      'Non-members see the fan club join screen before the appointment room.',
                  })}
                />
              </div>
              <div className="gl-creator-field gl-appointment-cover-field">
                <span>{t('createLive.fields.cover')}</span>
                <CoverPicker
                  preview={coverPreview}
                  onChange={(file) => {
                    if (coverPreview.startsWith('blob:')) URL.revokeObjectURL(coverPreview);
                    setCoverFile(file);
                    setCoverPreview(file ? URL.createObjectURL(file) : '');
                  }}
                />
              </div>
            </div>
            {error && <div className="gl-creator-empty-soft gl-appointment-error">{error}</div>}
            <div className="gl-appointment-dialog-actions">
              <button
                type="button"
                className="gl-creator-secondary"
                onClick={resetDraft}
                disabled={!title && !description && !coverPreview && !editing}
              >
                {t('studio.appointments.reset', { defaultValue: 'Reset' })}
              </button>
              <button
                type="button"
                className="gl-creator-primary gl-appointment-submit"
                onClick={() => void saveAppointment()}
                disabled={
                  createAppointment.isPending ||
                  updateAppointment.isPending ||
                  uploadCover.isPending
                }
              >
                <Save size={16} />
                {editing
                  ? t('studio.appointments.saveEdit', { defaultValue: 'Save changes' })
                  : t('studio.appointments.publish', { defaultValue: 'Publish appointment' })}
              </button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

export function CreatorPostsPage() {
  const { t } = useTranslation('pages');
  const { user } = useStudioUser();
  const [page, setPage] = useState(1);
  const posts = useStudioPosts(Boolean(user), page, STUDIO_POST_PAGE_SIZE);
  const createPost = useCreatePost();
  const uploadImage = useUploadPostImage();
  const [content, setContent] = useState('');
  const [visibility, setVisibility] = useState<PostVisibility>('public');
  const [commentsEnabled, setCommentsEnabled] = useState(true);
  const [commentMode, setCommentMode] = useState<PostCommentMode>('everyone');
  const [imageDrafts, setImageDrafts] = useState<PostImageDraft[]>([]);
  const [postDialogOpen, setPostDialogOpen] = useState(false);
  const total = posts.data?.total ?? 0;
  const pageCount = Math.max(1, Math.ceil(total / STUDIO_POST_PAGE_SIZE));
  const pageItems = posts.data?.items ?? [];
  const pagePublic = pageItems.filter((item) => item.visibility === 'public').length;
  const pageFollowers = pageItems.filter((item) => item.visibility === 'followers').length;
  const pagePrivate = pageItems.filter((item) => item.visibility === 'private').length;

  useEffect(() => {
    if (page > pageCount) setPage(pageCount);
  }, [page, pageCount]);

  const addImages = (files: FileList | null) => {
    if (!files?.length) return;
    const next = [...imageDrafts];
    for (const file of Array.from(files)) {
      if (next.length >= 6) {
        toast.info(t('posts.editor.maxImages', { defaultValue: '最多上传 6 张图片。' }));
        break;
      }
      if (!file.type.startsWith('image/')) {
        toast.error(
          t('upload.imageTypeError', { defaultValue: 'Please use JPG, PNG, WebP, or GIF images.' }),
        );
        continue;
      }
      if (file.size > 5 << 20) {
        toast.error(t('posts.editor.imageSize', { defaultValue: '图片需小于等于 5MB。' }));
        continue;
      }
      next.push({
        id: `${Date.now()}-${file.name}-${Math.random().toString(16).slice(2)}`,
        file,
        preview: URL.createObjectURL(file),
      });
    }
    setImageDrafts(next);
  };

  const removeImage = (id: string) => {
    setImageDrafts((drafts) => {
      const target = drafts.find((item) => item.id === id);
      if (target?.preview.startsWith('blob:')) URL.revokeObjectURL(target.preview);
      return drafts.filter((item) => item.id !== id);
    });
  };

  const clearDraft = () => {
    imageDrafts.forEach((item) => {
      if (item.preview.startsWith('blob:')) URL.revokeObjectURL(item.preview);
    });
    setImageDrafts([]);
    setContent('');
    setVisibility('public');
    setCommentsEnabled(true);
    setCommentMode('everyone');
  };

  const publish = async () => {
    const normalizedContent = content.trim();
    if (!normalizedContent && imageDrafts.length === 0) {
      toast.info(t('posts.editor.empty', { defaultValue: '写点内容或添加图片后再发布。' }));
      return;
    }
    try {
      const images: string[] = [];
      for (const draft of imageDrafts) {
        const uploaded = await uploadImage.mutateAsync(draft.file);
        images.push(uploaded.url);
      }
      await createPost.mutateAsync({
        content: normalizedContent,
        images,
        visibility,
        commentsEnabled,
        commentMode: commentsEnabled ? commentMode : 'everyone',
      });
      clearDraft();
      setPostDialogOpen(false);
      setPage(1);
      toast.success(t('posts.editor.published', { defaultValue: '帖子已发布。' }));
    } catch (err) {
      toast.error(
        err instanceof Error
          ? err.message
          : t('posts.editor.failed', { defaultValue: '发布失败，请稍后重试。' }),
      );
    }
  };

  const publishing = createPost.isPending || uploadImage.isPending;
  const canPublish = Boolean(content.trim() || imageDrafts.length > 0) && !publishing;

  return (
    <div className="gl-studio-posts-page">
      <section
        className="gl-creator-kpis gl-post-kpis"
        aria-label={t('posts.editor.kpis', { defaultValue: 'Post summary' })}
      >
        <StudioKpi
          icon={<FileText size={18} />}
          label={t('posts.editor.total', { defaultValue: '全部帖子' })}
          value={total.toLocaleString()}
          sub={t('posts.editor.totalSub', { defaultValue: '已发布' })}
        />
        <StudioKpi
          icon={<Globe2 size={18} />}
          label={t('posts.visibility.publicTitle', { defaultValue: '所有人可见' })}
          value={String(pagePublic)}
          sub={t('posts.editor.currentPage', { defaultValue: '当前页' })}
        />
        <StudioKpi
          icon={<Users size={18} />}
          label={t('posts.visibility.followersTitle', { defaultValue: '仅粉丝可见' })}
          value={String(pageFollowers)}
          sub={t('posts.editor.currentPage', { defaultValue: '当前页' })}
        />
        <StudioKpi
          icon={<LockKeyhole size={18} />}
          label={t('posts.visibility.privateTitle', { defaultValue: '仅自己可见' })}
          value={String(pagePrivate)}
          sub={t('posts.editor.currentPage', { defaultValue: '当前页' })}
        />
      </section>

      <div className="gl-post-toolbar">
        <button
          type="button"
          className="gl-creator-primary gl-post-create-btn"
          onClick={() => setPostDialogOpen(true)}
        >
          <Plus size={16} />
          {t('posts.editor.title', { defaultValue: '发布帖子' })}
        </button>
      </div>

      <section className="gl-studio-post-grid">
        <div className="gl-creator-panel gl-studio-post-list-panel">
          <div className="gl-creator-panel-head">
            <div>
              <span>{t('posts.editor.listLabel', { defaultValue: '帖子管理' })}</span>
              <h2>{t('posts.editor.listTitle', { defaultValue: '我的动态' })}</h2>
            </div>
            <MessageSquare size={22} />
          </div>
          <div className="gl-post-feed-list">
            {posts.isPending ? (
              Array.from({ length: 2 }).map((_, index) => (
                <div className="gl-post-card is-loading" key={index} />
              ))
            ) : pageItems.length ? (
              pageItems.map((post) => <PostCard key={post.id} post={post} context="studio" />)
            ) : (
              <div className="gl-creator-empty-soft">
                {t('posts.editor.emptyList', { defaultValue: '还没有发布过帖子。' })}
              </div>
            )}
          </div>
          {total > STUDIO_POST_PAGE_SIZE && (
            <StudioPostPager
              page={page}
              pageCount={pageCount}
              total={total}
              pageSize={STUDIO_POST_PAGE_SIZE}
              onPageChange={setPage}
            />
          )}
        </div>
      </section>

      <Dialog
        open={postDialogOpen}
        onOpenChange={(open) => {
          setPostDialogOpen(open);
          if (!open) clearDraft();
        }}
      >
        <DialogContent className="gl-post-dialog max-h-[calc(100vh-32px)] max-w-4xl overflow-y-auto sm:max-w-4xl">
          <div className="gl-appointment-form-panel gl-post-dialog-panel">
            <div className="gl-creator-panel-head">
              <div>
                <span>{t('posts.editor.label', { defaultValue: '频道动态' })}</span>
                <DialogTitle asChild>
                  <h2>{t('posts.editor.title', { defaultValue: '发布帖子' })}</h2>
                </DialogTitle>
              </div>
            </div>

            <div className="gl-post-dialog-body">
              <div className="gl-post-dialog-fields">
                <label className="gl-creator-field gl-post-text-field">
                  <span>{t('posts.editor.content', { defaultValue: '内容' })}</span>
                  <textarea
                    rows={7}
                    maxLength={2000}
                    value={content}
                    placeholder={t('posts.editor.placeholder', {
                      defaultValue: '写下直播预告、幕后花絮或想对粉丝说的话。',
                    })}
                    onChange={(event) => setContent(event.target.value)}
                  />
                </label>

                <div className="gl-post-editor-controls">
                  <div
                    className="gl-post-segmented"
                    aria-label={t('posts.editor.visibility', { defaultValue: '可见范围' })}
                  >
                    <button
                      type="button"
                      className={visibility === 'public' ? 'is-active' : undefined}
                      onClick={() => setVisibility('public')}
                    >
                      <Globe2 size={15} />
                      {t('posts.visibility.public', { defaultValue: '公开' })}
                    </button>
                    <button
                      type="button"
                      className={visibility === 'followers' ? 'is-active' : undefined}
                      onClick={() => setVisibility('followers')}
                    >
                      <Users size={15} />
                      {t('posts.visibility.followersTitle', { defaultValue: '仅粉丝可见' })}
                    </button>
                    <button
                      type="button"
                      className={visibility === 'private' ? 'is-active' : undefined}
                      onClick={() => setVisibility('private')}
                    >
                      <LockKeyhole size={15} />
                      {t('posts.visibility.privateTitle', { defaultValue: '仅自己可见' })}
                    </button>
                  </div>

                  <div className="gl-post-toggle-row">
                    <label className="gl-post-toggle">
                      <input
                        type="checkbox"
                        checked={commentsEnabled}
                        onChange={(event) => setCommentsEnabled(event.target.checked)}
                      />
                      <span>
                        {t('posts.editor.enableComments', { defaultValue: '开启评论区' })}
                      </span>
                    </label>
                    <label className={cn('gl-post-toggle', !commentsEnabled && 'is-disabled')}>
                      <input
                        type="checkbox"
                        checked={commentMode === 'followers'}
                        disabled={!commentsEnabled}
                        onChange={(event) =>
                          setCommentMode(event.target.checked ? 'followers' : 'everyone')
                        }
                      />
                      <span>
                        {t('posts.editor.followersOnlyComments', { defaultValue: '仅粉丝评论' })}
                      </span>
                    </label>
                  </div>
                </div>
              </div>

              <div className="gl-creator-field gl-post-image-field">
                <span>{t('posts.editor.addImages', { defaultValue: '添加图片' })}</span>
                <label
                  className={cn('gl-post-image-picker', imageDrafts.length >= 6 && 'is-disabled')}
                >
                  <ImagePlus size={22} />
                  <strong>{t('posts.editor.addImages', { defaultValue: '添加图片' })}</strong>
                  <input
                    type="file"
                    accept="image/jpeg,image/png,image/webp,image/gif"
                    multiple
                    disabled={imageDrafts.length >= 6}
                    onChange={(event) => {
                      addImages(event.target.files);
                      event.target.value = '';
                    }}
                  />
                </label>

                {imageDrafts.length > 0 && (
                  <div className="gl-post-draft-images">
                    {imageDrafts.map((item) => (
                      <div className="gl-post-draft-image" key={item.id}>
                        <img src={item.preview} alt="" />
                        <button
                          type="button"
                          aria-label={t('posts.editor.removeImage', { defaultValue: '移除图片' })}
                          onClick={() => removeImage(item.id)}
                        >
                          <X size={15} />
                        </button>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>

            <div className="gl-appointment-dialog-actions">
              <button
                type="button"
                className="gl-creator-secondary"
                disabled={publishing || (!content && imageDrafts.length === 0)}
                onClick={clearDraft}
              >
                {t('studio.appointments.reset', { defaultValue: 'Reset' })}
              </button>
              <button
                type="button"
                className="gl-creator-primary gl-post-submit"
                disabled={!canPublish}
                onClick={() => void publish()}
              >
                <Send size={16} />
                {publishing
                  ? t('posts.editor.publishing', { defaultValue: '发布中...' })
                  : t('posts.editor.publish', { defaultValue: '发布帖子' })}
              </button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

const MOD_FOLLOWER_PAGE_SIZE = 8;
const MOD_LOG_PAGE_SIZE = 10;

export function CreatorRoomModeratorsPage() {
  const { t, i18n } = useTranslation('pages');
  const { user } = useStudioUser();
  const [query, setQuery] = useState('');
  const [followerPage, setFollowerPage] = useState(1);
  const [logPage, setLogPage] = useState(1);
  const followers = useModeratorFollowers(
    query.trim(),
    followerPage,
    MOD_FOLLOWER_PAGE_SIZE,
    Boolean(user),
  );
  const moderators = useRoomModerators(1, 100, Boolean(user));
  const logs = useModeratorLogs(logPage, MOD_LOG_PAGE_SIZE, Boolean(user));
  const addModerator = useAddModerator();
  const removeModerator = useRemoveModerator();
  const moderatorIds = useMemo(
    () => new Set((moderators.data?.items ?? []).map((item) => item.id)),
    [moderators.data?.items],
  );
  const followerPageCount = Math.max(
    1,
    Math.ceil((followers.data?.total ?? 0) / MOD_FOLLOWER_PAGE_SIZE),
  );
  const logPageCount = Math.max(1, Math.ceil((logs.data?.total ?? 0) / MOD_LOG_PAGE_SIZE));

  useEffect(() => {
    setFollowerPage(1);
  }, [query]);

  const add = (target: ModerationUser) => {
    addModerator.mutate(target.id, {
      onSuccess: () =>
        toast.success(
          t('studio.moderators.added', {
            name: target.name,
            defaultValue: `${target.name} 已成为房间房管。`,
          }),
        ),
      onError: (err) => toast.error(moderatorAddErrorMessage(err, t)),
    });
  };

  const remove = (target: ModerationUser) => {
    removeModerator.mutate(target.id, {
      onSuccess: () =>
        toast.success(
          t('studio.moderators.removed', {
            name: target.name,
            defaultValue: `${target.name} 已取消房管资格。`,
          }),
        ),
      onError: (err) =>
        toast.error(
          err.message || t('studio.moderators.removeFailed', { defaultValue: '取消房管失败。' }),
        ),
    });
  };

  return (
    <div className="gl-room-mod-page">
      <section className="gl-creator-kpis">
        <StudioKpi
          icon={<ShieldCheck size={18} />}
          label={t('studio.moderators.active', { defaultValue: '房间房管' })}
          value={(moderators.data?.total ?? 0).toLocaleString()}
          sub={t('studio.moderators.activeSub', { defaultValue: '当前有效' })}
        />
        <StudioKpi
          icon={<Crown size={18} />}
          label={t('studio.moderators.followers', { defaultValue: '可选粉丝团成员' })}
          value={(followers.data?.total ?? 0).toLocaleString()}
          sub={t('studio.moderators.followersSub', { defaultValue: '仅已加入粉丝团' })}
        />
        <StudioKpi
          icon={<Clock3 size={18} />}
          label={t('studio.moderators.logs', { defaultValue: '操作记录' })}
          value={(logs.data?.total ?? 0).toLocaleString()}
          sub={t('studio.moderators.logsSub', { defaultValue: '分页记录' })}
        />
        <StudioKpi
          icon={<MessageSquare size={18} />}
          label={t('studio.moderators.muteOptions', { defaultValue: '禁言时长' })}
          value="5 / 10 / 30 / 60"
          sub={t('studio.moderators.muteOptionsSub', { defaultValue: '分钟' })}
        />
      </section>

      <section className="gl-room-mod-grid">
        <div className="gl-creator-panel gl-room-mod-search-panel">
          <div className="gl-creator-panel-head">
            <div>
              <span>{t('studio.moderators.searchLabel', { defaultValue: '粉丝列表' })}</span>
              <h2>{t('studio.moderators.searchTitle', { defaultValue: '添加房间房管' })}</h2>
            </div>
            <Users size={22} />
          </div>
          <label className="gl-room-mod-search">
            <span>
              {t('studio.moderators.searchPlaceholder', { defaultValue: '搜索粉丝用户名' })}
            </span>
            <input
              value={query}
              maxLength={64}
              placeholder={t('studio.moderators.searchPlaceholder', {
                defaultValue: '搜索粉丝用户名',
              })}
              onChange={(event) => setQuery(event.target.value)}
            />
          </label>
          <div className="gl-room-mod-list">
            {followers.isPending ? (
              <div className="gl-creator-empty-soft">
                {t('studio.loading', { defaultValue: 'Loading studio...' })}
              </div>
            ) : followers.data?.items.length ? (
              followers.data.items.map((item) => (
                <ModerationUserRow
                  key={item.id}
                  user={item}
                  active={moderatorIds.has(item.id)}
                  pending={addModerator.isPending || removeModerator.isPending}
                  actionLabel={
                    moderatorIds.has(item.id)
                      ? t('studio.moderators.alreadyModerator', { defaultValue: '已是房管' })
                      : t('studio.moderators.add', { defaultValue: '添加' })
                  }
                  onAction={() => add(item)}
                />
              ))
            ) : (
              <div className="gl-creator-empty-soft">
                {t('studio.moderators.noFollowers', { defaultValue: '没有找到可添加的粉丝。' })}
              </div>
            )}
          </div>
          {followers.data && followers.data.total > MOD_FOLLOWER_PAGE_SIZE && (
            <StudioAppointmentPager
              page={followerPage}
              pageCount={followerPageCount}
              total={followers.data.total}
              pageSize={MOD_FOLLOWER_PAGE_SIZE}
              onPageChange={setFollowerPage}
            />
          )}
        </div>

        <div className="gl-creator-panel gl-room-mod-current-panel">
          <div className="gl-creator-panel-head">
            <div>
              <span>{t('studio.moderators.currentLabel', { defaultValue: '当前名单' })}</span>
              <h2>{t('studio.moderators.currentTitle', { defaultValue: '正在值守的房管' })}</h2>
            </div>
            <ShieldCheck size={22} />
          </div>
          <div className="gl-room-mod-list">
            {moderators.isPending ? (
              <div className="gl-creator-empty-soft">
                {t('studio.loading', { defaultValue: 'Loading studio...' })}
              </div>
            ) : moderators.data?.items.length ? (
              moderators.data.items.map((item) => (
                <ModerationUserRow
                  key={item.id}
                  user={item}
                  active
                  pending={removeModerator.isPending}
                  actionLabel={t('studio.moderators.remove', { defaultValue: '取消资格' })}
                  danger
                  onAction={() => remove(item)}
                />
              ))
            ) : (
              <div className="gl-creator-empty-soft">
                {t('studio.moderators.noModerators', { defaultValue: '还没有房间房管。' })}
              </div>
            )}
          </div>
        </div>
      </section>

      <section className="gl-creator-panel gl-room-mod-log-panel">
        <div className="gl-creator-panel-head">
          <div>
            <span>{t('studio.moderators.logLabel', { defaultValue: '操作记录' })}</span>
            <h2>{t('studio.moderators.logTitle', { defaultValue: '房管操作流水' })}</h2>
          </div>
          <Clock3 size={22} />
        </div>
        <div className="gl-room-mod-log-list">
          {logs.isPending ? (
            <div className="gl-creator-empty-soft">
              {t('studio.loading', { defaultValue: 'Loading studio...' })}
            </div>
          ) : logs.data?.items.length ? (
            logs.data.items.map((item) => (
              <ModerationLogRow key={item.id} item={item} locale={i18n.language} />
            ))
          ) : (
            <div className="gl-creator-empty-soft">
              {t('studio.moderators.noLogs', { defaultValue: '暂无操作记录。' })}
            </div>
          )}
        </div>
        {logs.data && logs.data.total > MOD_LOG_PAGE_SIZE && (
          <StudioAppointmentPager
            page={logPage}
            pageCount={logPageCount}
            total={logs.data.total}
            pageSize={MOD_LOG_PAGE_SIZE}
            onPageChange={setLogPage}
          />
        )}
      </section>
    </div>
  );
}

export function CreatorFanGroupsPage() {
  const { t, i18n } = useTranslation('pages');
  const { user } = useStudioUser();
  const groups = useFanGroups(Boolean(user));
  const syncGroups = useSyncFanGroups();
  const updateMember = useUpdateFanGroupMember();
  const items = groups.data?.items ?? [];
  const memberCount = items.reduce((sum, group) => sum + group.memberCount, 0);
  const adminCount = items.reduce((sum, group) => sum + countGroupAdmins(group), 0);
  const mutedCount = items.reduce(
    (sum, group) => sum + group.members.filter((member) => member.muted).length,
    0,
  );

  const sync = () => {
    syncGroups.mutate(undefined, {
      onSuccess: (data) =>
        toast.success(
          t('studio.fanGroups.synced', {
            count: data.total,
            defaultValue: 'Synced {{count}} fan club group chats.',
          }),
        ),
      onError: (err) =>
        toast.error(
          err.message || t('studio.fanGroups.syncFailed', { defaultValue: '同步群聊失败。' }),
        ),
    });
  };

  const update = (
    group: FanGroup,
    member: FanGroupMember,
    patch: {
      role?: string;
      muteMinutes?: number;
      kick?: boolean;
      approveRejoin?: boolean;
      rejectRejoin?: boolean;
    },
    successKey: string,
    fallback: string,
  ) => {
    updateMember.mutate(
      { groupId: group.id, userId: member.user.id, ...patch },
      {
        onSuccess: () =>
          toast.success(t(successKey, { name: member.user.name, defaultValue: fallback })),
        onError: (err) =>
          toast.error(
            err.message || t('studio.fanGroups.updateFailed', { defaultValue: '更新群成员失败。' }),
          ),
      },
    );
  };

  return (
    <div className="gl-fan-groups-page">
      <section className="gl-creator-kpis">
        <StudioKpi
          icon={<MessageSquare size={18} />}
          label={t('studio.fanGroups.groupCount', { defaultValue: '群聊数量' })}
          value={(groups.data?.total ?? 0).toLocaleString()}
          sub={t('studio.fanGroups.groupCountSub', { defaultValue: '每群最多 200 人' })}
        />
        <StudioKpi
          icon={<Users size={18} />}
          label={t('studio.fanGroups.memberCount', { defaultValue: '粉丝团成员' })}
          value={memberCount.toLocaleString()}
          sub={t('studio.fanGroups.memberCountSub', { defaultValue: '已分配入群' })}
        />
        <StudioKpi
          icon={<Crown size={18} />}
          label={t('studio.fanGroups.adminCount', { defaultValue: '群管理员' })}
          value={adminCount.toLocaleString()}
          sub={t('studio.fanGroups.adminCountSub', { defaultValue: '可协助管理群聊' })}
        />
        <StudioKpi
          icon={<LockKeyhole size={18} />}
          label={t('studio.fanGroups.mutedCount', { defaultValue: '禁言成员' })}
          value={mutedCount.toLocaleString()}
          sub={t('studio.fanGroups.mutedCountSub', { defaultValue: '含临时禁言' })}
        />
      </section>

      <section className="gl-creator-panel gl-fan-groups-head-panel">
        <div className="gl-creator-panel-head">
          <div>
            <span>{t('studio.fanGroups.label', { defaultValue: '粉丝团群聊' })}</span>
            <h2>{t('studio.fanGroups.title', { defaultValue: '群聊管理' })}</h2>
          </div>
          <button
            type="button"
            className="gl-creator-secondary"
            disabled={syncGroups.isPending}
            onClick={sync}
          >
            <Users size={16} />
            {syncGroups.isPending
              ? t('studio.fanGroups.syncing', { defaultValue: '同步中' })
              : t('studio.fanGroups.sync', { defaultValue: '同步粉丝团' })}
          </button>
        </div>
        <p>
          {t('studio.fanGroups.description', {
            defaultValue:
              '系统会把已加入粉丝团的成员按每 200 人一个群分配，超过人数时自动创建新的粉丝团群聊。',
          })}
        </p>
      </section>

      <section className="gl-fan-groups-list">
        {groups.isPending ? (
          <div className="gl-creator-empty-soft">
            {t('studio.loading', { defaultValue: 'Loading studio...' })}
          </div>
        ) : items.length ? (
          items.map((group) => (
            <FanGroupPanel
              key={group.id}
              group={group}
              locale={i18n.language}
              pending={updateMember.isPending}
              onPromote={(member) =>
                update(
                  group,
                  member,
                  { role: 'admin' },
                  'studio.fanGroups.adminSet',
                  '{{name}} is now a group admin.',
                )
              }
              onDemote={(member) =>
                update(
                  group,
                  member,
                  { role: 'member' },
                  'studio.fanGroups.adminRemoved',
                  '{{name}} is no longer a group admin.',
                )
              }
              onMute={(member) =>
                update(
                  group,
                  member,
                  { muteMinutes: 60 },
                  'studio.fanGroups.muted',
                  '{{name}} was muted for 60 minutes.',
                )
              }
              onUnmute={(member) =>
                update(
                  group,
                  member,
                  { muteMinutes: 0 },
                  'studio.fanGroups.unmuted',
                  '{{name}} was unmuted.',
                )
              }
              onKick={(member) =>
                update(
                  group,
                  member,
                  { kick: true },
                  'studio.fanGroups.kicked',
                  '{{name}} was removed from the group chat.',
                )
              }
              onApproveRejoin={(member) =>
                update(
                  group,
                  member,
                  { approveRejoin: true },
                  'studio.fanGroups.rejoinApproved',
                  '{{name}} rejoined the group chat.',
                )
              }
              onRejectRejoin={(member) =>
                update(
                  group,
                  member,
                  { rejectRejoin: true },
                  'studio.fanGroups.rejoinRejected',
                  "{{name}}'s rejoin request was rejected.",
                )
              }
            />
          ))
        ) : (
          <div className="gl-creator-panel gl-fan-group-panel">
            <div className="gl-creator-empty-soft">
              {t('studio.fanGroups.empty', {
                defaultValue: '还没有粉丝团群聊，先同步粉丝团成员。',
              })}
            </div>
          </div>
        )}
      </section>
    </div>
  );
}

function FanGroupPanel({
  group,
  locale,
  pending,
  onPromote,
  onDemote,
  onMute,
  onUnmute,
  onKick,
  onApproveRejoin,
  onRejectRejoin,
}: {
  group: FanGroup;
  locale: string;
  pending: boolean;
  onPromote: (member: FanGroupMember) => void;
  onDemote: (member: FanGroupMember) => void;
  onMute: (member: FanGroupMember) => void;
  onUnmute: (member: FanGroupMember) => void;
  onKick: (member: FanGroupMember) => void;
  onApproveRejoin: (member: FanGroupMember) => void;
  onRejectRejoin: (member: FanGroupMember) => void;
}) {
  const { t } = useTranslation('pages');
  const pendingMembers = group.members.filter((member) => member.pendingRejoin);
  const activeMembers = group.members.filter((member) => !member.kicked);

  return (
    <article className="gl-creator-panel gl-fan-group-panel">
      <div className="gl-creator-panel-head">
        <div>
          <span>
            {t('studio.fanGroups.groupNo', {
              no: group.groupNo,
              defaultValue: 'Group {{no}}',
            })}
          </span>
          <h2>{group.name}</h2>
        </div>
        <div className="gl-fan-group-meta">
          <span>
            <Users size={15} />
            {group.memberCount}/200
          </span>
          <span>{formatFanGroupDate(group.updatedAt, locale)}</span>
        </div>
      </div>
      {pendingMembers.length > 0 && (
        <div className="gl-fan-group-requests">
          <strong>
            {t('studio.fanGroups.rejoinRequests', { defaultValue: 'Rejoin requests' })}
          </strong>
          {pendingMembers.map((member) => (
            <div key={member.user.id} className="gl-fan-group-request">
              <Avatar name={member.user.name} src={member.user.avatar} size={34} />
              <span>
                <b>{member.user.name}</b>
                <small>
                  {member.rejoinRequestedAt
                    ? formatFanGroupDate(member.rejoinRequestedAt, locale)
                    : t('studio.fanGroups.waitingApproval', { defaultValue: 'Waiting approval' })}
                </small>
              </span>
              <button
                type="button"
                className="gl-creator-secondary"
                disabled={pending}
                onClick={() => onRejectRejoin(member)}
              >
                {t('studio.fanGroups.rejectRejoin', { defaultValue: 'Reject' })}
              </button>
              <button
                type="button"
                className="gl-creator-primary"
                disabled={pending}
                onClick={() => onApproveRejoin(member)}
              >
                {t('studio.fanGroups.approveRejoin', { defaultValue: 'Approve' })}
              </button>
            </div>
          ))}
        </div>
      )}
      <div className="gl-fan-group-members">
        {activeMembers.length ? (
          activeMembers.map((member) => {
            const isAdmin = member.role === 'admin';
            const canManage = member.role !== 'owner';
            return (
              <div key={member.user.id} className="gl-fan-group-member">
                <Avatar name={member.user.name} src={member.user.avatar} size={38} />
                <div className="gl-fan-group-member-main">
                  <strong>{member.user.name}</strong>
                  <span>
                    {member.user.username ? `@${member.user.username}` : member.user.id} ·{' '}
                    {fanGroupRoleLabel(member.role, t)}
                    {member.muted
                      ? ` · ${t('studio.fanGroups.mutedState', { defaultValue: '已禁言' })}`
                      : ''}
                  </span>
                </div>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <button
                      type="button"
                      className="gl-message-icon-btn"
                      disabled={pending || !canManage}
                      aria-label={t('studio.fanGroups.memberActions', { defaultValue: '成员操作' })}
                    >
                      <MoreVertical size={16} />
                    </button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="gl-fan-group-actions">
                    {isAdmin ? (
                      <DropdownMenuItem onSelect={() => onDemote(member)}>
                        {t('studio.fanGroups.demote', { defaultValue: '取消管理员' })}
                      </DropdownMenuItem>
                    ) : (
                      <DropdownMenuItem onSelect={() => onPromote(member)}>
                        {t('studio.fanGroups.promote', { defaultValue: '设为管理员' })}
                      </DropdownMenuItem>
                    )}
                    {member.muted ? (
                      <DropdownMenuItem onSelect={() => onUnmute(member)}>
                        {t('studio.fanGroups.unmute', { defaultValue: '解除禁言' })}
                      </DropdownMenuItem>
                    ) : (
                      <DropdownMenuItem onSelect={() => onMute(member)}>
                        {t('studio.fanGroups.mute', { defaultValue: '禁言 60 分钟' })}
                      </DropdownMenuItem>
                    )}
                    <DropdownMenuItem className="text-destructive" onSelect={() => onKick(member)}>
                      {t('studio.fanGroups.kick', { defaultValue: '移出群聊' })}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            );
          })
        ) : (
          <div className="gl-creator-empty-soft">
            {t('studio.fanGroups.noMembers', { defaultValue: '这个群暂时没有成员。' })}
          </div>
        )}
      </div>
    </article>
  );
}

function countGroupAdmins(group: FanGroup) {
  return group.members.filter((member) => member.role === 'admin' && !member.kicked).length;
}

function fanGroupRoleLabel(role: string, t: TFunction<'pages'>) {
  if (role === 'owner') {
    return t('studio.fanGroups.roleOwner', { defaultValue: '群主' });
  }
  if (role === 'admin') {
    return t('studio.fanGroups.roleAdmin', { defaultValue: '管理员' });
  }
  return t('studio.fanGroups.roleMember', { defaultValue: '成员' });
}

function formatFanGroupDate(value: string, locale: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '';
  }
  return new Intl.DateTimeFormat(locale, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function ModerationUserRow({
  user,
  active,
  pending,
  actionLabel,
  danger,
  onAction,
}: {
  user: ModerationUser;
  active: boolean;
  pending: boolean;
  actionLabel: string;
  danger?: boolean;
  onAction: () => void;
}) {
  const { t } = useTranslation('pages');
  return (
    <div className="gl-room-mod-user-row">
      <Avatar name={user.name} src={user.avatar} size={38} />
      <div>
        <strong>{user.name}</strong>
        <span>{user.username ? `@${user.username}` : user.id}</span>
      </div>
      {active && (
        <span className="gl-room-mod-status">
          {t('studio.moderators.status', { defaultValue: 'Moderator' })}
        </span>
      )}
      <button
        type="button"
        className={danger ? 'gl-secondary-btn is-danger' : 'gl-creator-secondary'}
        disabled={pending || (!danger && active)}
        onClick={onAction}
      >
        {actionLabel}
      </button>
    </div>
  );
}

function moderatorAddErrorMessage(err: unknown, t: TFunction<'pages'>): string {
  const response = (err as { response?: { data?: { reason?: string; message?: string } } })
    .response;
  if (response?.data?.reason === 'not_fan_club_member') {
    return t('studio.moderators.fanClubOnlyError', {
      defaultValue: '只能添加粉丝团成员为房管。',
    });
  }
  return (
    response?.data?.message ||
    (err instanceof Error ? err.message : '') ||
    t('studio.moderators.addFailed', { defaultValue: '添加房管失败。' })
  );
}

function ModerationLogRow({ item, locale }: { item: ModerationLog; locale: string }) {
  const { t } = useTranslation('pages');
  const created = new Intl.DateTimeFormat(locale, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(item.createdAt));
  return (
    <div className="gl-room-mod-log-row">
      <div className="gl-room-mod-log-actor">
        <Avatar name={item.actorName} src={item.actorAvatar} size={32} />
        <div>
          <strong>{item.actorName}</strong>
          <span>{created}</span>
        </div>
      </div>
      <div className="gl-room-mod-log-action">
        <strong>{formatModeratorAction(item, t)}</strong>
        <span>{item.targetName}</span>
      </div>
    </div>
  );
}

function formatModeratorAction(item: ModerationLog, t: TFunction<'pages'>): string {
  if (item.action === 'add_moderator') {
    return t('studio.moderators.actionAdd', { defaultValue: '添加房管' });
  }
  if (item.action === 'remove_moderator') {
    return t('studio.moderators.actionRemove', { defaultValue: '取消房管' });
  }
  if (item.action === 'mute') {
    return t('studio.moderators.actionMute', {
      minutes: item.durationMinutes ?? 0,
      defaultValue: `禁言 ${item.durationMinutes ?? 0} 分钟`,
    });
  }
  if (item.action === 'unmute') {
    return t('studio.moderators.actionUnmute', { defaultValue: '解除禁言' });
  }
  return item.action;
}

function AppointmentStudioRow({
  item,
  onEdit,
  onUpdated,
}: {
  item: AppointmentItem;
  onEdit: () => void;
  onUpdated: () => void;
}) {
  const { t } = useTranslation('pages');
  const navigate = useNavigate();
  const startAppointment = useStartAppointment(item.id);
  const cancelAppointment = useCancelAppointment(item.id);
  const deleteAppointmentRecord = useDeleteAppointmentRecord(item.id);
  const mutable = item.status === 'scheduled';
  const pending =
    startAppointment.isPending || cancelAppointment.isPending || deleteAppointmentRecord.isPending;

  return (
    <AppointmentCard
      appointment={item}
      to={item.status === 'scheduled' ? `/live/${encodeURIComponent(item.roomId)}` : undefined}
      managementMode
      pending={pending}
      onStart={() => {
        startAppointment.mutate(undefined, {
          onSuccess: (stream) => {
            savePublisherSession(stream);
            toast.success(
              t('studio.appointments.started', { defaultValue: 'Appointment live started.' }),
            );
            navigate(`/studio/live/${encodeURIComponent(stream.id)}`);
          },
          onError: (err) =>
            toast.error(
              err.message ||
                t('studio.appointments.startFailed', {
                  defaultValue: 'Could not start the appointment.',
                }),
            ),
        });
      }}
      onEdit={() => {
        if (!mutable) {
          toast.info(
            t('studio.appointments.locked', {
              defaultValue: 'This appointment can no longer be edited.',
            }),
          );
          return;
        }
        onEdit();
      }}
      onDelete={() => {
        if (!mutable) {
          toast.info(
            t('studio.appointments.locked', {
              defaultValue: 'This appointment can no longer be edited.',
            }),
          );
          return;
        }
        cancelAppointment.mutate(undefined, {
          onSuccess: () => {
            toast.success(
              t('studio.appointments.canceled', { defaultValue: 'Appointment canceled.' }),
            );
            onUpdated();
          },
          onError: (err) =>
            toast.error(
              err.message ||
                t('studio.appointments.deleteFailed', {
                  defaultValue: 'Could not delete the appointment.',
                }),
            ),
        });
      }}
      onDeleteRecord={() => {
        const confirmed = window.confirm(
          t('studio.appointments.deleteRecordConfirm', {
            defaultValue: '确定要彻底删除这条预约记录吗？删除后它不会再出现在主播预约列表中。',
          }),
        );
        if (!confirmed) return;
        deleteAppointmentRecord.mutate(undefined, {
          onSuccess: () => {
            toast.success(
              t('studio.appointments.recordDeleted', {
                defaultValue: '预约记录已彻底删除。',
              }),
            );
            onUpdated();
          },
          onError: (err) =>
            toast.error(
              err.message ||
                t('studio.appointments.deleteRecordFailed', {
                  defaultValue: '无法彻底删除这条预约记录。',
                }),
            ),
        });
      }}
    />
  );
}

function AppointmentCategorySelect({
  categories,
  value,
  onChange,
  label,
}: {
  categories: string[];
  value: string;
  onChange: (value: string) => void;
  label: string;
}) {
  const { t } = useTranslation('pages');
  const currentLabel = t(`createLive.categories.${categoryKey(value)}`, { defaultValue: value });

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button type="button" className="gl-category-select-trigger" aria-label={label}>
          <span className="gl-category-select-current">{currentLabel}</span>
          <ChevronDown size={17} aria-hidden="true" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" sideOffset={6} className="gl-category-select-content">
        {categories.map((item) => {
          const selected = item === value;
          return (
            <DropdownMenuItem
              key={item}
              className={cn('gl-category-select-item', selected && 'is-selected')}
              onSelect={() => onChange(item)}
            >
              <span className="gl-category-select-check" aria-hidden="true">
                {selected && <Check size={14} />}
              </span>
              <span>{t(`createLive.categories.${categoryKey(item)}`, { defaultValue: item })}</span>
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function AppointmentDateTimePicker({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const { t, i18n } = useTranslation('pages');
  const [open, setOpen] = useState(false);
  const selected = useMemo(
    () => parseLocalDateTimeValue(value) ?? new Date(Date.now() + 60 * 60 * 1000),
    [value],
  );
  const [viewMonth, setViewMonth] = useState(() => startOfMonth(selected));
  const monthDays = useMemo(() => calendarMonthDays(viewMonth), [viewMonth]);
  const weekdays = useMemo(() => weekdayLabels(i18n.language), [i18n.language]);
  const selectedHour = pad2(selected.getHours());
  const selectedMinute = pad2(selected.getMinutes());

  useEffect(() => {
    if (open) return;
    setViewMonth(startOfMonth(selected));
  }, [open, selected]);

  const pickDate = (date: Date) => {
    onChange(replaceDatePart(value, date));
    setViewMonth(startOfMonth(date));
  };

  return (
    <div className={cn('gl-appointment-datetime', open && 'is-open')}>
      <button
        type="button"
        className="gl-appointment-datetime-trigger"
        aria-expanded={open}
        onClick={() => setOpen((next) => !next)}
      >
        <CalendarClock size={17} />
        <span>{formatDateTimeLabel(value, i18n.language)}</span>
      </button>
      {open && (
        <div className="gl-appointment-datetime-popover">
          <div className="gl-appointment-calendar-panel">
            <div className="gl-appointment-calendar-head">
              <button
                type="button"
                aria-label={t('studio.appointments.previousMonth', {
                  defaultValue: 'Previous month',
                })}
                onClick={() => setViewMonth(addMonths(viewMonth, -1))}
              >
                <ChevronLeft size={17} />
              </button>
              <strong>{formatMonthLabel(viewMonth, i18n.language)}</strong>
              <button
                type="button"
                aria-label={t('studio.appointments.nextMonth', { defaultValue: 'Next month' })}
                onClick={() => setViewMonth(addMonths(viewMonth, 1))}
              >
                <ChevronRight size={17} />
              </button>
            </div>
            <div className="gl-appointment-calendar-weekdays">
              {weekdays.map((day) => (
                <span key={day}>{day}</span>
              ))}
            </div>
            <div className="gl-appointment-calendar-grid">
              {monthDays.map((day) => {
                const sameMonth = day.getMonth() === viewMonth.getMonth();
                const selectedDay = isSameDate(day, selected);
                return (
                  <button
                    type="button"
                    key={day.toISOString()}
                    className={cn(!sameMonth && 'is-muted', selectedDay && 'is-selected')}
                    aria-pressed={selectedDay}
                    onClick={() => pickDate(day)}
                  >
                    {day.getDate()}
                  </button>
                );
              })}
            </div>
          </div>
          <div className="gl-appointment-time-panel">
            <div className="gl-appointment-time-row">
              <label>
                <span>{t('studio.appointments.hour', { defaultValue: 'Hour' })}</span>
                <select
                  value={selectedHour}
                  onChange={(event) =>
                    onChange(replaceTimePart(value, event.target.value, selectedMinute))
                  }
                >
                  {Array.from({ length: 24 }).map((_, index) => {
                    const hour = pad2(index);
                    return (
                      <option key={hour} value={hour}>
                        {hour}
                      </option>
                    );
                  })}
                </select>
              </label>
              <label>
                <span>{t('studio.appointments.minute', { defaultValue: 'Minute' })}</span>
                <select
                  value={selectedMinute}
                  onChange={(event) =>
                    onChange(replaceTimePart(value, selectedHour, event.target.value))
                  }
                >
                  {minuteOptions(selectedMinute).map((minute) => (
                    <option key={minute} value={minute}>
                      {minute}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            <div className="gl-appointment-datetime-actions">
              <button
                type="button"
                className="gl-creator-secondary"
                onClick={() =>
                  onChange(toLocalDateTimeInput(new Date(Date.now() + 60 * 60 * 1000)))
                }
              >
                {t('studio.appointments.oneHourLater', { defaultValue: '1 hour later' })}
              </button>
              <button type="button" className="gl-creator-primary" onClick={() => setOpen(false)}>
                {t('studio.appointments.done', { defaultValue: 'Done' })}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function StudioAppointmentPager({
  page,
  pageCount,
  total,
  pageSize,
  onPageChange,
}: {
  page: number;
  pageCount: number;
  total: number;
  pageSize: number;
  onPageChange: (page: number) => void;
}) {
  const { t } = useTranslation('pages');
  const start = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const end = Math.min(total, page * pageSize);
  return (
    <div
      className="gl-history-pager gl-appointment-pager"
      aria-label={t('appointments.pagination', { defaultValue: 'Appointment pagination' })}
    >
      <div className="gl-history-pager-count">
        {t('appointments.pageCount', {
          start,
          end,
          total,
          defaultValue: '{{start}}-{{end}} of {{total}}',
        })}
      </div>
      <div className="gl-history-pager-controls">
        <button
          type="button"
          aria-label={t('appointments.previous', { defaultValue: 'Previous page' })}
          disabled={page <= 1}
          onClick={() => onPageChange(Math.max(1, page - 1))}
        >
          <ChevronLeft size={16} />
        </button>
        <span className="gl-appointment-pager-current">
          {page} / {pageCount}
        </span>
        <button
          type="button"
          aria-label={t('appointments.next', { defaultValue: 'Next page' })}
          disabled={page >= pageCount}
          onClick={() => onPageChange(Math.min(pageCount, page + 1))}
        >
          <ChevronRight size={16} />
        </button>
      </div>
    </div>
  );
}

function StudioPostPager({
  page,
  pageCount,
  total,
  pageSize,
  onPageChange,
}: {
  page: number;
  pageCount: number;
  total: number;
  pageSize: number;
  onPageChange: (page: number) => void;
}) {
  const { t } = useTranslation('pages');
  const start = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const end = Math.min(total, page * pageSize);
  return (
    <div
      className="gl-history-pager gl-appointment-pager"
      aria-label={t('posts.editor.pagination', { defaultValue: 'Post pagination' })}
    >
      <div className="gl-history-pager-count">
        {t('posts.editor.pageCount', {
          start,
          end,
          total,
          defaultValue: '{{start}}-{{end}} / {{total}}',
        })}
      </div>
      <div className="gl-history-pager-controls">
        <button
          type="button"
          aria-label={t('posts.editor.previous', { defaultValue: 'Previous page' })}
          disabled={page <= 1}
          onClick={() => onPageChange(Math.max(1, page - 1))}
        >
          <ChevronLeft size={16} />
        </button>
        <span className="gl-appointment-pager-current">
          {page} / {pageCount}
        </span>
        <button
          type="button"
          aria-label={t('posts.editor.next', { defaultValue: 'Next page' })}
          disabled={page >= pageCount}
          onClick={() => onPageChange(Math.min(pageCount, page + 1))}
        >
          <ChevronRight size={16} />
        </button>
      </div>
    </div>
  );
}

function toLocalDateTimeInput(value: Date | string): string {
  const date = typeof value === 'string' ? new Date(value) : value;
  if (Number.isNaN(date.getTime())) return '';
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function parseLocalDateTimeValue(value: string): Date | null {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

function pad2(value: number): string {
  return String(value).padStart(2, '0');
}

function startOfMonth(value: Date): Date {
  return new Date(value.getFullYear(), value.getMonth(), 1);
}

function addMonths(value: Date, amount: number): Date {
  return new Date(value.getFullYear(), value.getMonth() + amount, 1);
}

function calendarMonthDays(month: Date): Date[] {
  const start = startOfMonth(month);
  start.setDate(start.getDate() - start.getDay());
  return Array.from(
    { length: 42 },
    (_, index) => new Date(start.getFullYear(), start.getMonth(), start.getDate() + index),
  );
}

function weekdayLabels(locale: string): string[] {
  return Array.from({ length: 7 }, (_, index) =>
    new Intl.DateTimeFormat(locale, { weekday: 'short' }).format(new Date(2026, 0, 4 + index)),
  );
}

function formatMonthLabel(value: Date, locale: string): string {
  return new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'long' }).format(value);
}

function formatDateTimeLabel(value: string, locale: string): string {
  const date = parseLocalDateTimeValue(value);
  if (!date) return value;
  return new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function isSameDate(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

function replaceDatePart(value: string, date: Date): string {
  const current = parseLocalDateTimeValue(value) ?? new Date(Date.now() + 60 * 60 * 1000);
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}T${pad2(current.getHours())}:${pad2(current.getMinutes())}`;
}

function replaceTimePart(value: string, hour: string, minute: string): string {
  const current = parseLocalDateTimeValue(value) ?? new Date(Date.now() + 60 * 60 * 1000);
  return `${current.getFullYear()}-${pad2(current.getMonth() + 1)}-${pad2(current.getDate())}T${hour}:${minute}`;
}

function minuteOptions(selectedMinute: string): string[] {
  const options = new Set(Array.from({ length: 12 }, (_, index) => pad2(index * 5)));
  options.add(selectedMinute);
  return Array.from(options).sort();
}

export function CreatorLiveConsolePage() {
  const { t } = useTranslation('pages');
  const { id = '' } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const currentUser = useAuthStore((s) => s.user);
  const room = useRoom(id, Boolean(id));
  const liveRooms = useRooms({ size: 100 });
  const stopLive = useStopLive();
  const directoryStream = useMemo(
    () => liveRooms.data?.items.find((item) => item.id === id) ?? null,
    [id, liveRooms.data?.items],
  );
  const stream = useMemo(
    () => mergeStreamSnapshot(room.data, directoryStream),
    [directoryStream, room.data],
  );
  const roomIsLive = Boolean(stream && (isStreamLive(stream) || directoryStream));
  const streamEnded = Boolean(stream && !roomIsLive && stream.status === 'ended');
  const ownsStream = Boolean(
    stream?.ownerId && currentUser?.id && stream.ownerId === currentUser.id,
  );
  const [ended, setEnded] = useState(false);
  const [moderationTarget, setModerationTarget] = useState<ChatModerationTarget | null>(null);
  const realtime = useRoomRealtime(id, Boolean(id && stream), {
    onLiveEnded: () => setEnded(true),
    ownerId: stream?.ownerId,
  });
  const muteUser = useMuteRoomUser(id);
  const unmuteUser = useUnmuteRoomUser(id);
  const targetMuteState = useRoomMuteState(
    id,
    moderationTarget?.userId ?? '',
    Boolean(id && moderationTarget),
  );
  const elapsed = useElapsed(stream?.startedAt, roomIsLive && !ended);
  const viewerCount = realtime.viewerCount > 0 ? realtime.viewerCount : (stream?.viewers ?? 0);
  const session = useMemo(() => {
    if (!stream) return null;
    return publisherSessionFromStream(stream) ?? matchingStoredSession(stream.id);
  }, [stream]);
  const { refetch: refetchRoom } = room;
  const { refetch: refetchLiveRooms } = liveRooms;

  useEffect(() => {
    if (!id) return;
    void refetchRoom();
    void refetchLiveRooms();
    const timer = window.setInterval(() => {
      void refetchRoom();
      void refetchLiveRooms();
    }, 3000);
    return () => window.clearInterval(timer);
  }, [id, refetchLiveRooms, refetchRoom]);

  useEffect(() => {
    if (!stream?.streamKey || !ownsStream) return;
    savePublisherSession(stream);
  }, [ownsStream, stream]);

  useEffect(() => {
    if (!streamEnded) return;
    clearPublisherSession();
  }, [streamEnded]);

  if (room.isPending && liveRooms.isPending && !stream) {
    return <StudioLoading label={t('studio.loading', { defaultValue: 'Loading studio...' })} />;
  }

  if ((room.isError && !directoryStream) || !stream) {
    return (
      <StudioAccessPage
        icon={<Radio size={24} />}
        title={t('studio.console.notFound', { defaultValue: 'Live room unavailable' })}
        body={t('studio.console.notFoundBody', {
          defaultValue: 'The live room could not be loaded.',
        })}
        actionLabel={t('notFound.back')}
        onAction={() => navigate('/studio/overview')}
      />
    );
  }

  if (!ownsStream) {
    return (
      <StudioAccessPage
        icon={<ShieldCheck size={24} />}
        title={t('studio.console.ownerOnly', { defaultValue: 'Owner console only' })}
        body={t('studio.console.ownerOnlyBody', {
          defaultValue: 'Only the channel owner can control this live.',
        })}
        actionLabel={t('notFound.back')}
        onAction={() => navigate('/studio/overview')}
      />
    );
  }

  if (streamEnded) {
    return (
      <StudioAccessPage
        icon={<Square size={24} />}
        title={t('studio.console.ended', { defaultValue: 'Live ended.' })}
        body={t('studio.console.endFinished', {
          defaultValue:
            'This live has already finished. Open the studio overview to start a new one.',
        })}
        actionLabel={t('notFound.back')}
        onAction={() => navigate('/studio/overview')}
      />
    );
  }

  const stop = () => {
    stopLive.mutate(undefined, {
      onSuccess: () => {
        clearPublisherSession();
        toast.success(t('studio.console.ended', { defaultValue: 'Live ended.' }));
        navigate('/studio/overview');
      },
      onError: () =>
        toast.error(t('studio.console.endFailed', { defaultValue: 'Could not end the live.' })),
    });
  };
  const submitMute = (durationMinutes: MuteUserPayload['durationMinutes']) => {
    if (!moderationTarget) return;
    muteUser.mutate(
      {
        targetUserId: moderationTarget.userId,
        targetName: moderationTarget.user,
        targetAvatar: moderationTarget.avatar,
        durationMinutes,
      },
      {
        onSuccess: (resp) => {
          toast.success(
            t('liveRoom.moderation.muted', {
              user: resp.targetName,
              minutes: resp.durationMinutes,
              defaultValue: `${resp.targetName} muted for ${resp.durationMinutes} minutes.`,
            }),
          );
          setModerationTarget(null);
        },
        onError: (err) =>
          toast.error(
            err.message ||
              t('liveRoom.moderation.muteFailed', { defaultValue: 'Could not mute this user.' }),
          ),
      },
    );
  };
  const submitUnmute = () => {
    if (!moderationTarget) return;
    unmuteUser.mutate(moderationTarget.userId, {
      onSuccess: () => {
        toast.success(
          t('liveRoom.moderation.unmuted', {
            user: moderationTarget.user,
            defaultValue: `${moderationTarget.user} was unmuted.`,
          }),
        );
        setModerationTarget(null);
      },
      onError: (err) =>
        toast.error(
          err.message ||
            t('liveRoom.moderation.unmuteFailed', { defaultValue: 'Could not unmute this user.' }),
        ),
    });
  };

  return (
    <div className="gl-page gl-live-console">
      <header className="gl-live-console-status">
        <div className="gl-live-console-status-main">
          <span className={cn('gl-live-console-pill', roomIsLive ? 'is-live' : 'is-waiting')}>
            <span />
            {roomIsLive
              ? t('studio.console.live', { defaultValue: 'LIVE' })
              : t('studio.console.waiting', { defaultValue: 'Waiting' })}
          </span>
          <StatusMetric
            label={t('studio.console.duration', { defaultValue: 'Duration' })}
            value={elapsed}
          />
          <StatusMetric
            label={t('studio.console.online', { defaultValue: 'Online' })}
            value={viewerCount.toLocaleString()}
          />
        </div>
        <button
          type="button"
          className="gl-owner-end-live"
          disabled={stopLive.isPending}
          onClick={stop}
        >
          <Square size={15} />
          {stopLive.isPending
            ? t('studio.console.ending', { defaultValue: 'Ending...' })
            : t('studio.console.endLive', { defaultValue: 'End live' })}
        </button>
      </header>

      <div className="gl-live-console-grid">
        <main className="gl-live-console-main">
          {roomIsLive ? (
            <Player
              stream={stream}
              viewerCount={viewerCount}
              bullets={realtime.bullets}
              onBulletEnd={realtime.clearBullet}
            />
          ) : (
            <WaitingPreview stream={stream} />
          )}
          <LiveMetadataEditor
            stream={stream}
            onUpdated={() => {
              void room.refetch();
              void liveRooms.refetch();
            }}
          />
          <LiveReplaySettingsPanel stream={stream} />
          {session && (
            <ConsolePublisherPanel
              session={session}
              playbackUrl={stream.playbackUrl}
              live={roomIsLive}
              onRefresh={() => {
                void room.refetch();
                void liveRooms.refetch();
              }}
            />
          )}
          <section className="gl-creator-panel gl-live-console-activity">
            <div className="gl-creator-panel-head">
              <div>
                <span>{t('studio.console.activity', { defaultValue: 'Interaction' })}</span>
                <h2>{t('studio.console.bettingTitle', { defaultValue: 'Betting module' })}</h2>
              </div>
              <Trophy size={22} />
            </div>
            <BettingPanel roomId={stream.id} ownsStream />
          </section>
          <section className="gl-creator-panel gl-live-console-activity">
            <div className="gl-creator-panel-head">
              <div>
                <span>{t('studio.console.activity', { defaultValue: 'Interaction' })}</span>
                <h2>{t('studio.console.luckyBagTitle', { defaultValue: 'Lucky bag module' })}</h2>
              </div>
              <Gift size={22} />
            </div>
            <LuckyBagPanel roomId={stream.id} ownsStream />
          </section>
          <section className="gl-creator-panel gl-live-console-activity">
            <div className="gl-creator-panel-head">
              <div>
                <span>{t('studio.console.activity', { defaultValue: 'Interaction' })}</span>
                <h2>{t('studio.console.micLinkTitle', { defaultValue: 'Mic-link module' })}</h2>
              </div>
              <Mic size={22} />
            </div>
            <MicLinkPanel roomId={stream.id} ownsStream />
          </section>
        </main>
        <StudioInteractionRail
          stream={stream}
          messages={realtime.messages}
          viewers={realtime.viewers}
          viewerCount={viewerCount}
          bulletsCount={realtime.bullets.length}
          onClearBullets={() =>
            realtime.bullets.forEach((bullet) => realtime.clearBullet(bullet.id))
          }
          onSendChat={realtime.sendChat}
          canModerate
          onOpenModeration={setModerationTarget}
          reconnecting={realtime.readyState !== 'open'}
          reconnectingLabel={
            realtime.readyState === 'reconnecting'
              ? t('studio.console.reconnecting', {
                  count: realtime.retryCount,
                  defaultValue: 'Reconnecting #{{count}}',
                })
              : t('studio.console.disconnected', { defaultValue: 'Disconnected' })
          }
        />
      </div>
      <ConsoleMuteUserDialog
        target={moderationTarget}
        currentUserId={currentUser?.id}
        muted={Boolean(targetMuteState.data?.muted)}
        statePending={targetMuteState.isFetching}
        pending={muteUser.isPending || unmuteUser.isPending}
        onOpenChange={(open) => {
          if (!open) setModerationTarget(null);
        }}
        onMute={submitMute}
        onUnmute={submitUnmute}
      />
    </div>
  );
}

function StudioHeader({ user }: { user?: User | null }) {
  const { t } = useTranslation('pages');
  return (
    <header className="gl-creator-studio-head">
      <Avatar name={userDisplayName(user)} src={user?.avatar} size={58} />
      <div>
        <span>{t('studio.brand', { defaultValue: 'Creator Studio' })}</span>
        <h1>
          {t('studio.title', { name: userDisplayName(user), defaultValue: '{{name}} workspace' })}
        </h1>
      </div>
    </header>
  );
}

function StudioTabs() {
  const { t } = useTranslation('pages');
  return (
    <nav
      className="gl-creator-tabs"
      aria-label={t('studio.tabs.label', { defaultValue: 'Creator Studio sections' })}
    >
      <NavLink to="/studio/overview">
        {t('studio.tabs.overview', { defaultValue: 'Overview' })}
      </NavLink>
      <NavLink to="/studio/prepare">
        {t('studio.tabs.prepare', { defaultValue: 'Stream setup' })}
      </NavLink>
      <NavLink to="/studio/posts">{t('studio.tabs.posts', { defaultValue: '帖子动态' })}</NavLink>
      <NavLink to="/studio/appointments">
        {t('studio.tabs.appointments', { defaultValue: 'Live appointments' })}
      </NavLink>
      <NavLink to="/studio/moderators">
        {t('studio.tabs.moderators', { defaultValue: '房间房管' })}
      </NavLink>
      <NavLink to="/studio/fan-groups">
        {t('studio.tabs.fanGroups', { defaultValue: '群聊管理' })}
      </NavLink>
      <NavLink to="/studio/replay">
        {t('studio.tabs.replay', { defaultValue: 'Data replay' })}
      </NavLink>
      <NavLink to="/studio/live-replays">
        {t('studio.tabs.liveReplays', { defaultValue: 'Live replays' })}
      </NavLink>
    </nav>
  );
}

function StudioPermissionPage({
  status,
  rejectReason,
  applying,
  onApply,
}: {
  status: User['livePermissionStatus'];
  rejectReason?: string;
  applying: boolean;
  onApply: (reason: string) => void;
}) {
  const { t } = useTranslation('pages');
  const [reason, setReason] = useState('');
  const pending = status === 'pending';
  const rejected = status === 'rejected';
  const title = pending
    ? t('studio.permission.pending.title', { defaultValue: 'Application under review' })
    : rejected
      ? t('studio.permission.rejected.title', { defaultValue: 'Application was rejected' })
      : t('studio.permission.none.title', { defaultValue: 'Apply for live access' });
  const body = pending
    ? t('studio.permission.pending.body', {
        defaultValue: 'We are checking your channel and account status.',
      })
    : rejected
      ? t('studio.permission.rejected.body', {
          defaultValue:
            'Reason: your channel information needs another review before live access can be enabled.',
        })
      : t('studio.permission.none.body', {
          defaultValue: 'Live access is required before opening the streaming workspace.',
        });

  return (
    <StudioAccessPage
      icon={
        pending ? <Timer size={24} /> : rejected ? <ShieldCheck size={24} /> : <Send size={24} />
      }
      title={title}
      body={body}
      actionLabel={
        pending
          ? t('studio.permission.pending.action', { defaultValue: 'Submitted' })
          : applying
            ? t('createLive.permission.submitting')
            : rejected
              ? t('studio.permission.rejected.action', { defaultValue: 'Apply again' })
              : t('studio.permission.none.action', { defaultValue: 'Submit application' })
      }
      actionDisabled={pending || applying || (!pending && !reason.trim())}
      onAction={() => onApply(reason.trim())}
      details={[
        t('studio.permission.reviewTime', {
          defaultValue: 'Estimated review time: within 1 business day.',
        }),
        t('studio.permission.notice', {
          defaultValue: 'Keep your channel name, avatar, and cover ready for review.',
        }),
        rejected
          ? t('studio.permission.rejected.reason', {
              reason:
                rejectReason ||
                t('studio.permission.rejected.reasonFallback', {
                  defaultValue: 'Channel readiness did not meet the current creator policy.',
                }),
              defaultValue: 'Rejected reason: {{reason}}',
            })
          : t('studio.permission.progress', {
              defaultValue: 'Progress: submitted -> manual review -> result.',
            }),
      ]}
    >
      {!pending && (
        <label className="gl-creator-access-reason">
          <span>
            {t('studio.permission.reasonLabel', {
              defaultValue: 'Why do you want live access?',
            })}
          </span>
          <textarea
            value={reason}
            rows={5}
            maxLength={500}
            onChange={(event) => setReason(event.target.value)}
            placeholder={t('studio.permission.reasonPlaceholder', {
              defaultValue: 'Tell admins your live content plan, schedule, and channel readiness.',
            })}
          />
        </label>
      )}
    </StudioAccessPage>
  );
}

function JoinPlatformDialog({
  open,
  pending,
  rejectReason,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  pending: boolean;
  rejectReason?: string;
  onOpenChange: (open: boolean) => void;
  onSubmit: (reason: string) => void;
}) {
  const { t } = useTranslation('pages');
  const [reason, setReason] = useState('');

  useEffect(() => {
    if (open) {
      setReason('');
    }
  }, [open]);

  const benefits = [
    t('studio.platform.benefits.badge', {
      defaultValue: 'The orange platform certification badge appears on your channel.',
    }),
    t('studio.platform.benefits.fee', {
      defaultValue: 'Withdrawal fees are reduced by 10 percentage points, from 35% to 25%.',
    }),
    t('studio.platform.benefits.protection', {
      defaultValue: 'Certified creators receive stronger platform protection and recommendation.',
    }),
  ];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gl-platform-join-dialog">
        <DialogTitle>
          {t('studio.platform.title', { defaultValue: 'Join the GoLive platform' })}
        </DialogTitle>
        <DialogDescription>
          {t('studio.platform.description', {
            defaultValue:
              'Submit a platform certification application. An administrator will approve or reject it with a reason.',
          })}
        </DialogDescription>
        <div className="gl-platform-benefits">
          {benefits.map((item) => (
            <span key={item}>
              <ShieldCheck size={15} />
              {item}
            </span>
          ))}
        </div>
        {rejectReason && (
          <div className="gl-platform-last-reject">
            <strong>{t('studio.platform.lastReject', { defaultValue: 'Last rejection' })}</strong>
            <p>{rejectReason}</p>
          </div>
        )}
        <label className="gl-creator-access-reason">
          <span>{t('studio.platform.reasonLabel', { defaultValue: 'Application note' })}</span>
          <textarea
            rows={5}
            maxLength={500}
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder={t('studio.platform.reasonPlaceholder', {
              defaultValue:
                'Tell admins your creator direction, schedule, and why this channel should be certified.',
            })}
          />
        </label>
        <div className="gl-platform-join-actions">
          <button type="button" className="gl-secondary-btn" onClick={() => onOpenChange(false)}>
            {t('studio.platform.cancel', { defaultValue: 'Cancel' })}
          </button>
          <button
            type="button"
            className="gl-creator-primary"
            disabled={pending || !reason.trim()}
            onClick={() => onSubmit(reason.trim())}
          >
            {pending
              ? t('studio.platform.submitting', { defaultValue: 'Submitting...' })
              : t('studio.platform.submit', { defaultValue: 'Submit application' })}
          </button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function StudioAccessPage({
  icon,
  title,
  body,
  actionLabel,
  actionDisabled,
  onAction,
  details = [],
  children,
}: {
  icon: ReactNode;
  title: string;
  body: string;
  actionLabel: string;
  actionDisabled?: boolean;
  onAction: () => void;
  details?: string[];
  children?: ReactNode;
}) {
  return (
    <div className="gl-page gl-creator-access">
      <section className="gl-creator-access-panel">
        <div className="gl-creator-access-icon">{icon}</div>
        <h1>{title}</h1>
        <p>{body}</p>
        {details.length > 0 && (
          <div className="gl-creator-access-details">
            {details.map((item) => (
              <span key={item}>{item}</span>
            ))}
          </div>
        )}
        {children}
        <button
          className="gl-creator-primary"
          type="button"
          disabled={actionDisabled}
          onClick={onAction}
        >
          {actionLabel}
        </button>
      </section>
    </div>
  );
}

function StudioLoading({ label }: { label: string }) {
  return (
    <div className="gl-page">
      <div className="gl-studio-panel gl-studio-loading">{label}</div>
    </div>
  );
}

function StudioKpi({
  icon,
  label,
  value,
  sub,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  sub: string;
}) {
  return (
    <div className="gl-creator-kpi">
      <div className="gl-creator-kpi-icon">{icon}</div>
      <span>{label}</span>
      <strong>{value}</strong>
      <small>{sub}</small>
    </div>
  );
}

function StudioAction({
  icon,
  title,
  body,
  onClick,
  className,
  disabled,
}: {
  icon: ReactNode;
  title: string;
  body: string;
  onClick: () => void;
  className?: string;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      className={cn('gl-creator-action', className)}
      disabled={disabled}
      onClick={onClick}
    >
      <span>{icon}</span>
      <strong>{title}</strong>
      <small>{body}</small>
    </button>
  );
}

function StepCard({
  number,
  active,
  done,
  locked,
  title,
  actionLabel,
  onOpen,
  children,
}: {
  number: number;
  active: boolean;
  done: boolean;
  locked: boolean;
  title: string;
  actionLabel?: string;
  onOpen: () => void;
  children: ReactNode;
}) {
  return (
    <section
      className={cn(
        'gl-creator-step',
        active && 'is-active',
        done && 'is-done',
        locked && 'is-locked',
      )}
    >
      <button type="button" className="gl-creator-step-head" disabled={locked} onClick={onOpen}>
        <span>{done ? <CheckCircle2 size={16} /> : number}</span>
        <strong>{title}</strong>
        {!active && !locked && actionLabel && <em>{actionLabel}</em>}
      </button>
      {active && <div className="gl-creator-step-body">{children}</div>}
    </section>
  );
}

function CoverPicker({
  preview,
  onChange,
}: {
  preview: string;
  onChange: (file: File | null) => void;
}) {
  const { t } = useTranslation('pages');
  return (
    <label className="gl-creator-cover-picker">
      {preview ? (
        <img src={preview} alt="" />
      ) : (
        <span>
          <ImagePlus size={22} />
          {t('createLive.addCover')}
        </span>
      )}
      <input
        type="file"
        accept="image/jpeg,image/png,image/webp,image/gif"
        onChange={(event) => onChange(event.target.files?.[0] ?? null)}
      />
    </label>
  );
}

function CategoryPicker({
  categories,
  value,
  onChange,
}: {
  categories: string[];
  value: string;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation('pages');
  return (
    <div className="gl-creator-category-picker">
      <span>{t('createLive.fields.category')}</span>
      <div
        className="gl-creator-category-list"
        role="listbox"
        aria-label={t('createLive.fields.category')}
      >
        {categories.map((item) => {
          const active = item === value;
          return (
            <button
              key={item}
              type="button"
              role="option"
              aria-selected={active}
              className={cn('gl-creator-category-chip', active && 'is-active')}
              onClick={() => onChange(item)}
            >
              {t(`createLive.categories.${categoryKey(item)}`, { defaultValue: item })}
            </button>
          );
        })}
      </div>
    </div>
  );
}

function FanClubOnlyToggle({
  checked,
  onChange,
  title,
  description,
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
  title?: string;
  description?: string;
}) {
  const { t } = useTranslation('pages');
  const resolvedTitle =
    title ?? t('studio.prepare.fanClubOnly', { defaultValue: 'Fan club exclusive live' });
  const resolvedDescription =
    description ??
    t('studio.prepare.fanClubOnlySub', {
      defaultValue: 'Only fan club members can watch and chat after joining.',
    });
  return (
    <label className={cn('gl-fan-exclusive-toggle', checked && 'is-active')}>
      <input
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span className="gl-fan-exclusive-toggle-mark">
        <LockKeyhole size={15} />
      </span>
      <span>
        <strong>{resolvedTitle}</strong>
        <small>{resolvedDescription}</small>
      </span>
    </label>
  );
}

function LiveSetupPreview({
  title,
  description,
  category,
  cover,
  status,
  fanClubOnly,
}: {
  title: string;
  description: string;
  category: string;
  cover: string;
  status: string;
  fanClubOnly: boolean;
}) {
  const { t } = useTranslation('pages');
  return (
    <div className="gl-creator-live-preview">
      <div className="gl-creator-live-preview-cover">
        {cover ? <img src={cover} alt="" /> : <Upload size={34} />}
        <span>{status}</span>
        {fanClubOnly && <FanClubExclusiveBadge compact className="gl-creator-preview-exclusive" />}
      </div>
      <div className="gl-creator-live-preview-copy">
        <strong>{title || t('createLive.defaultTitle')}</strong>
        <span>{category}</span>
        <p>
          {description ||
            t('studio.prepare.noDescription', { defaultValue: 'Description will appear here.' })}
        </p>
      </div>
    </div>
  );
}

function CheckRow({ ok, label }: { ok: boolean; label: string }) {
  return (
    <div className={cn('gl-creator-check-row', ok && 'is-ok')}>
      <span>{ok ? <CheckCircle2 size={16} /> : <Timer size={16} />}</span>
      {label}
    </div>
  );
}

function PublisherPreview({ rtmpServer }: { rtmpServer: string }) {
  const { t } = useTranslation('pages');
  return (
    <div className="gl-creator-publisher-preview">
      <PublisherLine
        label={t('studio.publisher.server', { defaultValue: 'OBS server' })}
        value={rtmpServer}
        copyLabel="OBS server"
      />
      <div className="gl-creator-key-placeholder">
        <span>{t('studio.publisher.key', { defaultValue: 'Stream key' })}</span>
        <strong>
          {t('studio.publisher.keyAfterStart', { defaultValue: 'Issued after confirmation' })}
        </strong>
      </div>
    </div>
  );
}

function ConsolePublisherPanel({
  session,
  playbackUrl,
  live,
  onRefresh,
}: {
  session: PublisherSession;
  playbackUrl?: string;
  live: boolean;
  onRefresh: () => void;
}) {
  const { t } = useTranslation('pages');
  const streamUrl = playbackUrl || session.playbackUrl || '';
  return (
    <section className="gl-creator-publisher-console">
      <div className="gl-creator-panel-head">
        <div>
          <span>{t('studio.publisher.title', { defaultValue: 'Publishing' })}</span>
          <h2>
            {live
              ? t('studio.publisher.connected', { defaultValue: 'OBS connected' })
              : t('studio.publisher.waiting', { defaultValue: 'Waiting for OBS' })}
          </h2>
        </div>
        <button
          type="button"
          className="gl-creator-icon-button"
          onClick={onRefresh}
          aria-label={t('studio.publisher.refresh', { defaultValue: 'Refresh status' })}
        >
          <ClipboardCheck size={17} />
        </button>
      </div>
      <div className="gl-creator-publisher-grid">
        <PublisherLine
          label={t('studio.publisher.server', { defaultValue: 'OBS server' })}
          value={session.rtmpServer}
          copyLabel="OBS server"
        />
        <PublisherLine
          label={t('studio.publisher.key', { defaultValue: 'Stream key' })}
          value={session.streamKey}
          secret
          copyLabel="Stream key"
        />
        {streamUrl && (
          <PublisherLine
            label={t('studio.publisher.playback', { defaultValue: 'Playback URL' })}
            value={streamUrl}
            copyLabel="Playback URL"
          />
        )}
      </div>
    </section>
  );
}

function PublisherLine({
  label,
  value,
  copyLabel,
  secret,
}: {
  label: string;
  value: string;
  copyLabel: string;
  secret?: boolean;
}) {
  const copy = async () => {
    try {
      const method = await copyText(value, copyLabel);
      toast.success(
        method === 'manual' ? `${copyLabel} opened for manual copy.` : `${copyLabel} copied.`,
      );
    } catch {
      toast.error(`Could not copy ${copyLabel.toLowerCase()}.`);
    }
  };
  return (
    <div className="gl-creator-publisher-line">
      <span>{label}</span>
      <code>{secret ? value.replace(/.(?=.{6})/g, '*') : value}</code>
      <button type="button" onClick={() => void copy()} aria-label={`Copy ${label}`}>
        <Copy size={15} />
      </button>
    </div>
  );
}

function WaitingPreview({ stream }: { stream: Stream }) {
  const { t } = useTranslation('pages');
  return (
    <div className="gl-live-console-waiting">
      {stream.cover && <LoadableImage src={stream.cover} alt="" />}
      <div>
        <Radio size={34} />
        <strong>
          {t('studio.console.waitingPreview', { defaultValue: 'Waiting for publisher' })}
        </strong>
        <span>
          {t('studio.console.waitingPreviewBody', {
            defaultValue: 'Add the stream server and key in OBS, then start streaming.',
          })}
        </span>
      </div>
    </div>
  );
}

function LiveMetadataEditor({ stream, onUpdated }: { stream: Stream; onUpdated: () => void }) {
  const { t } = useTranslation('pages');
  const uploadCover = useUploadLiveCover();
  const updateLive = useUpdateLiveMetadata();
  const [title, setTitle] = useState(stream.title);
  const [description, setDescription] = useState(stream.description ?? '');
  const [coverPreview, setCoverPreview] = useState(stream.cover ?? '');
  const [coverFile, setCoverFile] = useState<File | null>(null);

  useEffect(() => {
    return () => {
      if (coverPreview.startsWith('blob:')) URL.revokeObjectURL(coverPreview);
    };
  }, [coverPreview]);

  const pending = uploadCover.isPending || updateLive.isPending;
  const normalizedTitle = title.trim();
  const normalizedDescription = description.trim();
  const normalizedCover = coverPreview.trim();
  const dirty =
    normalizedTitle !== stream.title ||
    normalizedDescription !== (stream.description ?? '') ||
    normalizedCover !== (stream.cover ?? '') ||
    Boolean(coverFile);

  const setCover = (file: File | null) => {
    if (!file) return;
    setCoverFile(file);
    setCoverPreview(URL.createObjectURL(file));
  };

  const clearCover = () => {
    setCoverFile(null);
    setCoverPreview('');
  };

  const saveMetadata = async () => {
    if (!normalizedTitle) {
      toast.error(
        t('studio.console.metadataTitleRequired', { defaultValue: 'Title is required.' }),
      );
      return;
    }
    try {
      const cover = coverFile ? (await uploadCover.mutateAsync(coverFile)).url : normalizedCover;
      const next = await updateLive.mutateAsync({
        title: normalizedTitle,
        description: normalizedDescription,
        cover,
      });
      setTitle(next.title);
      setDescription(next.description ?? '');
      setCoverFile(null);
      setCoverPreview(next.cover ?? '');
      toast.success(t('studio.console.metadataSaved', { defaultValue: 'Live room info updated.' }));
      onUpdated();
    } catch (err) {
      toast.error(
        err instanceof Error
          ? err.message
          : t('studio.console.metadataFailed', {
              defaultValue: 'Could not update live room info.',
            }),
      );
    }
  };

  return (
    <section className="gl-creator-panel gl-live-metadata-panel">
      <div className="gl-creator-panel-head">
        <div>
          <span>{t('studio.console.metadataLabel', { defaultValue: 'Room info' })}</span>
          <h2>{t('studio.console.metadataTitle', { defaultValue: 'Live room details' })}</h2>
        </div>
        <ImagePlus size={22} />
      </div>
      <form
        className="gl-live-metadata-form"
        onSubmit={(event) => {
          event.preventDefault();
          void saveMetadata();
        }}
      >
        <div className="gl-live-metadata-grid">
          <div className="gl-live-metadata-fields">
            <label className="gl-creator-field">
              <span>{t('createLive.fields.title')}</span>
              <input
                value={title}
                maxLength={120}
                onChange={(event) => setTitle(event.target.value)}
              />
            </label>
            <label className="gl-creator-field">
              <span>{t('createLive.fields.description')}</span>
              <textarea
                value={description}
                rows={4}
                maxLength={2000}
                placeholder={t('studio.prepare.descriptionPlaceholder', {
                  defaultValue: 'Tell viewers what this live is about.',
                })}
                onChange={(event) => setDescription(event.target.value)}
              />
            </label>
          </div>
          <div className="gl-live-metadata-cover">
            <span>{t('createLive.fields.cover')}</span>
            <CoverPicker preview={coverPreview} onChange={setCover} />
            <button
              className="gl-creator-secondary"
              type="button"
              disabled={!coverPreview || pending}
              onClick={clearCover}
            >
              {t('studio.console.metadataClearCover', { defaultValue: 'Remove cover' })}
            </button>
          </div>
        </div>
        <div className="gl-live-metadata-actions">
          <button
            className="gl-creator-primary"
            type="submit"
            disabled={!dirty || !normalizedTitle || pending}
          >
            <Save size={16} />
            {pending
              ? t('studio.console.metadataSaving', { defaultValue: 'Saving...' })
              : t('studio.console.metadataSave', { defaultValue: 'Save changes' })}
          </button>
        </div>
      </form>
    </section>
  );
}

function LiveReplaySettingsPanel({ stream }: { stream: Stream }) {
  const { t } = useTranslation('pages');
  const updateReplay = useUpdateLiveReplaySettings();
  const [uploadAfterEnd, setUploadAfterEnd] = useState(Boolean(stream.replay?.uploadAfterEnd));
  const [visibility, setVisibility] = useState<ReplayVisibility>(
    stream.replay?.visibility ?? 'public',
  );

  useEffect(() => {
    setUploadAfterEnd(Boolean(stream.replay?.uploadAfterEnd));
    setVisibility(stream.replay?.visibility ?? 'public');
  }, [stream.id, stream.replay?.uploadAfterEnd, stream.replay?.visibility]);

  const dirty =
    uploadAfterEnd !== Boolean(stream.replay?.uploadAfterEnd) ||
    visibility !== (stream.replay?.visibility ?? 'public');

  const save = () => {
    updateReplay.mutate(
      { uploadAfterEnd, visibility },
      {
        onSuccess: () =>
          toast.success(
            t('studio.replay.liveSaved', { defaultValue: 'Replay upload preference saved.' }),
          ),
        onError: (err) =>
          toast.error(
            err.message ||
              t('studio.replay.liveSaveFailed', {
                defaultValue: 'Could not save replay preference.',
              }),
          ),
      },
    );
  };

  return (
    <section className="gl-creator-panel gl-live-replay-panel">
      <div className="gl-creator-panel-head">
        <div>
          <span>{t('studio.replay.liveLabel', { defaultValue: 'Replay upload' })}</span>
          <h2>{t('studio.replay.liveTitle', { defaultValue: 'After this live ends' })}</h2>
        </div>
        <PlayCircle size={22} />
      </div>
      <div className="gl-live-replay-settings">
        <button
          type="button"
          role="switch"
          aria-checked={uploadAfterEnd}
          className={cn('gl-live-replay-toggle', uploadAfterEnd && 'is-on')}
          onClick={() => setUploadAfterEnd((value) => !value)}
        >
          <span>
            <strong>
              {uploadAfterEnd
                ? t('studio.replay.uploadOn', { defaultValue: 'Upload replay after ending' })
                : t('studio.replay.uploadOff', { defaultValue: 'Do not upload replay' })}
            </strong>
            <small>
              {t('studio.replay.uploadStatus', {
                status: replayStatusLabel(stream.replay?.status ?? 'none', t),
                defaultValue: 'Current status: {{status}}',
              })}
            </small>
          </span>
          <i aria-hidden />
        </button>
        <VisibilitySelect value={visibility} onChange={setVisibility} />
        <button
          type="button"
          className="gl-creator-primary gl-live-replay-save"
          disabled={!dirty || updateReplay.isPending}
          onClick={save}
        >
          <Save size={16} />
          {updateReplay.isPending
            ? t('studio.replay.saving', { defaultValue: 'Saving...' })
            : t('studio.replay.saveLive', { defaultValue: 'Save replay settings' })}
        </button>
      </div>
    </section>
  );
}

function StudioInteractionRail({
  stream,
  messages,
  viewers,
  viewerCount,
  bulletsCount,
  onClearBullets,
  onSendChat,
  canModerate,
  onOpenModeration,
  reconnecting,
  reconnectingLabel,
}: {
  stream: Stream;
  messages: Message[];
  viewers: ReturnType<typeof useRoomRealtime>['viewers'];
  viewerCount: number;
  bulletsCount: number;
  onClearBullets: () => void;
  onSendChat: (text: string) => boolean;
  canModerate?: boolean;
  onOpenModeration?: (target: ChatModerationTarget) => void;
  reconnecting: boolean;
  reconnectingLabel: string;
}) {
  const { t } = useTranslation('pages');
  const [giftPage, setGiftPage] = useState(0);
  const [superChatPage, setSuperChatPage] = useState(0);
  const [reportTarget, setReportTarget] = useState<ReportTargetDraft | null>(null);
  const gifts = summarizeGifts(messages);
  const superChats = messages
    .filter((item): item is Extract<Message, { kind: 'super_chat' }> => item.kind === 'super_chat')
    .sort((a, b) => b.ts - a.ts);
  const giftPageCount = pageCount(gifts.items.length);
  const superChatPageCount = pageCount(superChats.length);
  const visibleGifts = pageSlice(gifts.items, giftPage);
  const visibleSuperChats = pageSlice(superChats, superChatPage);
  const latestGiftTs = gifts.items[0]?.latestTs ?? 0;
  const latestSuperChatTs = superChats[0]?.ts ?? 0;

  useEffect(() => {
    setGiftPage(0);
  }, [latestGiftTs]);

  useEffect(() => {
    setSuperChatPage(0);
  }, [latestSuperChatTs]);

  useEffect(() => {
    setGiftPage((page) => Math.min(page, giftPageCount - 1));
  }, [giftPageCount]);

  useEffect(() => {
    setSuperChatPage((page) => Math.min(page, superChatPageCount - 1));
  }, [superChatPageCount]);

  return (
    <aside className="gl-live-console-rail">
      <section className="gl-creator-panel gl-live-console-chat-panel">
        <div className="gl-creator-panel-head">
          <div>
            <span>{t('studio.console.danmu', { defaultValue: 'Danmu management' })}</span>
            <h2>{t('studio.console.chatControl', { defaultValue: 'Live chat' })}</h2>
          </div>
          <MessageSquare size={22} />
        </div>
        <div className="gl-live-console-danmu-row">
          <span>
            {t('studio.console.activeBullets', {
              count: bulletsCount,
              defaultValue: '{{count}} active bullets',
            })}
          </span>
          <button type="button" onClick={onClearBullets} disabled={bulletsCount === 0}>
            {t('studio.console.clearPreview', { defaultValue: 'Clear preview' })}
          </button>
        </div>
        <Chat
          messages={messages}
          viewers={viewers}
          viewerTotal={viewerCount}
          roomId={stream.id}
          ownerId={stream.ownerId}
          ownerName={stream.channel}
          onSendChat={onSendChat}
          canModerate={canModerate}
          onOpenModeration={onOpenModeration}
          onReportMessage={setReportTarget}
          reconnecting={reconnecting}
          reconnectingLabel={reconnectingLabel}
        />
      </section>

      <section className="gl-creator-panel">
        <div className="gl-creator-panel-head">
          <div>
            <span>{t('studio.console.gifts', { defaultValue: 'Gift summary' })}</span>
            <h2>{formatCoins(gifts.totalCoin)}</h2>
          </div>
          <Gift size={22} />
        </div>
        <div className="gl-live-console-gifts">
          {visibleGifts.length > 0 ? (
            visibleGifts.map((item) => (
              <div className="gl-live-console-gift-row" key={item.key}>
                <span>{item.icon || item.name}</span>
                <div className="gl-live-console-gift-main">
                  <strong>{item.user}</strong>
                  <small>{item.name}</small>
                </div>
                <small>x{item.count}</small>
              </div>
            ))
          ) : (
            <div className="gl-creator-empty-soft">
              {t('studio.console.noGifts', { defaultValue: 'No gifts yet.' })}
            </div>
          )}
        </div>
        {gifts.items.length > CONSOLE_ACTIVITY_PAGE_SIZE && (
          <ConsolePager
            page={giftPage}
            pageCount={giftPageCount}
            onPage={setGiftPage}
            newerLabel={t('studio.console.newerGifts', { defaultValue: 'Newer gifts' })}
            olderLabel={t('studio.console.olderGifts', { defaultValue: 'Older gifts' })}
          />
        )}
      </section>

      <section className="gl-creator-panel">
        <div className="gl-creator-panel-head">
          <div>
            <span>{t('studio.console.superChat', { defaultValue: 'Pinned SuperChat' })}</span>
            <h2>{superChats.length}</h2>
          </div>
          <Zap size={22} />
        </div>
        <div className="gl-live-console-superchats">
          {visibleSuperChats.length > 0 ? (
            visibleSuperChats.map((item) => (
              <div key={item.id}>
                <strong>{item.user}</strong>
                <span>{item.amount}</span>
                <p>{item.text}</p>
              </div>
            ))
          ) : (
            <div className="gl-creator-empty-soft">
              {t('studio.console.noSuperChat', { defaultValue: 'No SuperChat pinned.' })}
            </div>
          )}
        </div>
        {superChats.length > CONSOLE_ACTIVITY_PAGE_SIZE && (
          <ConsolePager
            page={superChatPage}
            pageCount={superChatPageCount}
            onPage={setSuperChatPage}
            newerLabel={t('studio.console.newerSuperChats', {
              defaultValue: 'Newer SuperChats',
            })}
            olderLabel={t('studio.console.olderSuperChats', {
              defaultValue: 'Older SuperChats',
            })}
          />
        )}
      </section>
      <ReportDialog
        open={Boolean(reportTarget)}
        target={reportTarget}
        onOpenChange={(open) => {
          if (!open) setReportTarget(null);
        }}
      />
    </aside>
  );
}

function pageCount(total: number): number {
  return Math.max(1, Math.ceil(total / CONSOLE_ACTIVITY_PAGE_SIZE));
}

function pageSlice<T>(items: T[], page: number): T[] {
  const start = page * CONSOLE_ACTIVITY_PAGE_SIZE;
  return items.slice(start, start + CONSOLE_ACTIVITY_PAGE_SIZE);
}

function ConsolePager({
  page,
  pageCount: totalPages,
  onPage,
  newerLabel,
  olderLabel,
}: {
  page: number;
  pageCount: number;
  onPage: (page: number) => void;
  newerLabel: string;
  olderLabel: string;
}) {
  return (
    <div className="gl-live-console-pager">
      <button
        type="button"
        aria-label={newerLabel}
        disabled={page <= 0}
        onClick={() => onPage(Math.max(0, page - 1))}
      >
        <ChevronLeft size={15} strokeWidth={2.6} />
      </button>
      <span>
        {page + 1} / {totalPages}
      </span>
      <button
        type="button"
        aria-label={olderLabel}
        disabled={page >= totalPages - 1}
        onClick={() => onPage(Math.min(totalPages - 1, page + 1))}
      >
        <ChevronRight size={15} strokeWidth={2.6} />
      </button>
    </div>
  );
}

function ConsoleMuteUserDialog({
  target,
  currentUserId,
  muted,
  statePending,
  pending,
  onOpenChange,
  onMute,
  onUnmute,
}: {
  target: ChatModerationTarget | null;
  currentUserId?: string;
  muted: boolean;
  statePending: boolean;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onMute: (duration: MuteUserPayload['durationMinutes']) => void;
  onUnmute: () => void;
}) {
  const { t } = useTranslation('pages');
  const targetRole = target?.role ?? 'viewer';
  const blocked =
    !target ||
    target.userId === currentUserId ||
    targetRole === 'owner' ||
    targetRole === 'moderator';
  const durations: MuteUserPayload['durationMinutes'][] = [5, 10, 30, 60];
  const statusText = blocked
    ? t('liveRoom.moderation.blocked', {
        defaultValue: 'Creators, moderators, and yourself cannot be muted.',
      })
    : statePending
      ? t('liveRoom.moderation.checkingMute', { defaultValue: 'Checking mute status...' })
      : muted
        ? t('liveRoom.moderation.alreadyMuted', {
            defaultValue: 'This user is currently muted. You can unmute them.',
          })
        : t('liveRoom.moderation.pickDuration', { defaultValue: 'Choose mute duration' });

  return (
    <Dialog open={Boolean(target)} onOpenChange={onOpenChange}>
      <DialogContent className="gl-mute-dialog">
        <DialogTitle>{t('liveRoom.moderation.title', { defaultValue: 'Mute user' })}</DialogTitle>
        <div className="gl-mute-target">
          <ShieldCheck size={22} />
          <div>
            <strong>{target?.user ?? ''}</strong>
            <span>{statusText}</span>
          </div>
        </div>
        {muted && !blocked ? (
          <button
            type="button"
            className="gl-mute-unmute-btn"
            disabled={pending || statePending}
            onClick={onUnmute}
          >
            {t('liveRoom.moderation.unmuteAction', { defaultValue: 'Unmute' })}
          </button>
        ) : (
          <div className="gl-mute-duration-grid">
            {durations.map((duration) => (
              <button
                key={duration}
                type="button"
                disabled={blocked || pending || statePending}
                onClick={() => onMute(duration)}
              >
                {t('liveRoom.moderation.minutes', {
                  count: duration,
                  defaultValue: '{{count}} minutes',
                })}
              </button>
            ))}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

function StatusMetric({ label, value }: { label: string; value: string }) {
  return (
    <span className="gl-live-console-metric">
      <small>{label}</small>
      <strong>{value}</strong>
    </span>
  );
}

function useElapsed(startedAt: string | undefined, enabled: boolean): string {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!enabled) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [enabled]);
  if (!startedAt || !enabled) return '00:00:00';
  const seconds = Math.max(0, Math.floor((now - new Date(startedAt).getTime()) / 1000));
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  return [h, m, s].map((item) => String(item).padStart(2, '0')).join(':');
}

function summarizeGifts(messages: Message[]) {
  const map = new Map<
    string,
    {
      key: string;
      user: string;
      name: string;
      icon?: string;
      count: number;
      totalCoin: number;
      latestTs: number;
    }
  >();
  let totalCoin = 0;
  for (const item of messages) {
    if (item.kind !== 'gift') continue;
    const key = `${item.userId || item.user}:${item.giftName}`;
    const current = map.get(key) ?? {
      key,
      user: item.user,
      name: item.giftName,
      icon: item.giftIcon,
      count: 0,
      totalCoin: 0,
      latestTs: item.ts,
    };
    current.count += item.count ?? 1;
    current.totalCoin += item.totalCoin ?? 0;
    current.latestTs = Math.max(current.latestTs, item.ts);
    totalCoin += item.totalCoin ?? 0;
    if (item.user) current.user = item.user;
    if (item.giftIcon) current.icon = item.giftIcon;
    map.set(key, current);
  }
  const items = Array.from(map.values()).sort((a, b) => {
    if (a.latestTs !== b.latestTs) return b.latestTs - a.latestTs;
    return b.totalCoin - a.totalCoin;
  });
  return { items, totalCoin };
}

function matchingStoredSession(streamId: string): PublisherSession | null {
  const session = loadPublisherSession();
  return session?.streamId === streamId ? session : null;
}

function rtmpServer(): string {
  return (import.meta.env.VITE_RTMP_BASE || 'rtmp://localhost/live').replace(/\/$/, '');
}

function formatCoins(value: number): string {
  return `${Math.round(value).toLocaleString()} coins`;
}
