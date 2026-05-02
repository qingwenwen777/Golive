import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Navigate, NavLink, Outlet, useNavigate, useParams } from 'react-router-dom';
import {
  BarChart3,
  CheckCircle2,
  ClipboardCheck,
  Copy,
  Eye,
  Gift,
  ImagePlus,
  ListChecks,
  MessageSquare,
  PlayCircle,
  Radio,
  Send,
  ShieldCheck,
  Square,
  Timer,
  Trophy,
  Upload,
  Users,
  Zap,
} from 'lucide-react';
import { toast } from 'sonner';
import { useMe } from '@/api/auth';
import { useSubmitCreatorApplication } from '@/api/creator';
import {
  useCreatorAnalytics,
  useGoLive,
  useRoom,
  useRooms,
  useStopLive,
  useUploadLiveCover,
} from '@/api/room';
import { Avatar } from '@/components/Avatar';
import { Chat } from '@/features/live-room/Chat';
import { BettingPanel } from '@/features/live-room/BettingPanel';
import { Player } from '@/features/live-room/Player';
import { useRoomRealtime } from '@/features/live-room/useRoomRealtime';
import {
  LIVE_SESSION_STORAGE_KEY,
  loadPublisherSession,
  publisherSessionFromStream,
  savePublisherSession,
  type PublisherSession,
} from '@/features/creator/CreateLiveDialog';
import { CATEGORIES_EN } from '@/constants/catalog';
import { copyText } from '@/lib/clipboard';
import { cn } from '@/lib/cn';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useAuthHydrated, useAuthStore, useIsAuthed } from '@/stores/useAuthStore';
import type { Message } from '@/types/message';
import type { Stream } from '@/types/stream';
import { userDisplayName, type User } from '@/types/user';

const DEFAULT_CATEGORY = 'Just Chatting';

function categoryKey(category: string): string {
  return category.toLowerCase().replace(/\s+/g, '');
}

function currentChannelKey(user: User | null | undefined): string {
  return user?.id ?? '';
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
  const liveRooms = useRooms({ size: 100 });
  const storedSession = useMemo(() => loadPublisherSession(), []);

  const stream = useMemo(() => {
    const items = liveRooms.data?.items ?? [];
    const byStoredSession = storedSession
      ? items.find((item) => item.id === storedSession.streamId)
      : undefined;
    if (byStoredSession) return byStoredSession;

    const byOwner = user?.id
      ? items.find((item) => item.ownerId === user.id && isStreamLive(item))
      : undefined;
    if (byOwner) return byOwner;

    if (storedSession?.streamId) return { id: storedSession.streamId };
    return null;
  }, [liveRooms.data?.items, storedSession, user?.id]);

  return { stream };
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

  if (!hydrated || (isAuthed && me.isPending && !user)) {
    return <StudioLoading label={t('studio.loading', { defaultValue: 'Loading studio...' })} />;
  }

  if (!isAuthed) {
    return (
      <StudioAccessPage
        icon={<Radio size={24} />}
        title={t('studio.auth.title', { defaultValue: 'Sign in to open Creator Studio' })}
        body={t('studio.auth.body', {
          defaultValue: 'Your dashboard, stream setup, and analytics are tied to your GoLive account.',
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
        applying={apply.isPending}
        onApply={() => {
          apply.mutate(undefined, {
            onSuccess: (resp) => toast.success(resp.message),
            onError: (err) =>
              toast.error(err.message || t('createLive.errors.applicationFailed')),
          });
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
  const channelKey = currentChannelKey(user);
  const analytics = useCreatorAnalytics(channelKey, Boolean(channelKey));
  const data = analytics.data;
  const latest = data?.history[0];

  return (
    <div className="gl-creator-overview">
      <section className="gl-creator-kpis" aria-label={t('studio.overview.kpis')}>
        <StudioKpi
          icon={<Eye size={18} />}
          label={t('studio.overview.cards.online', { defaultValue: 'Current audience' })}
          value={(data?.peakViewers ?? 0).toLocaleString()}
          sub={t('studio.overview.cards.onlineSub', { defaultValue: 'peak audience' })}
        />
        <StudioKpi
          icon={<Users size={18} />}
          label={t('studio.overview.cards.subscribers', { defaultValue: 'Subscribers' })}
          value={(data?.subscriberCount ?? 0).toLocaleString()}
          sub={t('studio.overview.cards.subscribersSub', { defaultValue: 'channel total' })}
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
              <h2>{t('studio.overview.quickHeading', { defaultValue: 'Prepare your next live' })}</h2>
            </div>
            <ListChecks size={22} />
          </div>
          <div className="gl-creator-actions-grid">
            <StudioAction
              icon={<PlayCircle size={18} />}
              title={t('studio.actions.prepare', { defaultValue: 'Open stream setup' })}
              body={t('studio.actions.prepareSub', { defaultValue: 'Title, cover, category, and OBS checks.' })}
              onClick={() => navigate('/studio/prepare')}
            />
            <StudioAction
              icon={<BarChart3 size={18} />}
              title={t('studio.actions.analytics', { defaultValue: 'Review analytics' })}
              body={t('studio.actions.analyticsSub', { defaultValue: 'Revenue, viewers, and finished live reports.' })}
              onClick={() => navigate(`/studio/analytics/${encodeURIComponent(channelKey)}`)}
            />
            <StudioAction
              icon={<Users size={18} />}
              title={t('studio.actions.channel', { defaultValue: 'View channel' })}
              body={t('studio.actions.channelSub', { defaultValue: 'Check how viewers see your creator page.' })}
              onClick={() => navigate(`/channel/${encodeURIComponent(channelKey)}`)}
            />
          </div>
        </div>

        <div className="gl-creator-panel">
          <div className="gl-creator-panel-head">
            <div>
              <span>{t('studio.overview.recentTitle', { defaultValue: 'Latest result' })}</span>
              <h2>{latest?.title ?? t('studio.overview.noRecent', { defaultValue: 'No stream yet' })}</h2>
            </div>
            <Trophy size={22} />
          </div>
          {analytics.isPending ? (
            <div className="gl-creator-muted-line">{t('studio.loading', { defaultValue: 'Loading studio...' })}</div>
          ) : latest ? (
            <div className="gl-creator-latest">
              <div className="gl-creator-latest-cover">
                {latest.cover && <img src={latest.cover} alt="" />}
                <span>{latest.duration}</span>
              </div>
              <div className="gl-creator-latest-meta">
                <span>{t('studio.overview.latestPeak', { count: latest.peakViewers, defaultValue: '{{count}} peak viewers' })}</span>
                <span>{formatCoins(latest.revenueCoin)}</span>
              </div>
            </div>
          ) : (
            <div className="gl-creator-empty-soft">
              {t('studio.overview.emptyRecent', { defaultValue: 'Start a live and the recap will appear here.' })}
            </div>
          )}
        </div>
      </section>
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
  const [title, setTitle] = useState(() => t('createLive.defaultTitle'));
  const [description, setDescription] = useState('');
  const [category, setCategory] = useState(DEFAULT_CATEGORY);
  const [coverFile, setCoverFile] = useState<File | null>(null);
  const [coverPreview, setCoverPreview] = useState('');
  const [step, setStep] = useState(1);
  const [obsChecked, setObsChecked] = useState(false);
  const categories = CATEGORIES_EN.filter((item) => item !== 'All');

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

  const startLive = async () => {
    if (!done3 || !user) return;
    try {
      const uploadedCover = coverFile ? (await uploadCover.mutateAsync(coverFile)).url : coverPreview;
      goLive.mutate(
        {
          title: title.trim(),
          description: description.trim(),
          category,
          cover: uploadedCover,
          channelName: userDisplayName(user),
          avatar: user.avatar,
        },
        {
          onSuccess: (stream) => {
            savePublisherSession(stream);
            toast.success(t('studio.prepare.created', { defaultValue: 'Live room created. Open OBS and begin publishing.' }));
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
          onOpen={() => setStep(1)}
        >
          <label className="gl-creator-field">
            <span>{t('createLive.fields.title')}</span>
            <input value={title} maxLength={120} onChange={(event) => setTitle(event.target.value)} />
          </label>
          <label className="gl-creator-field">
            <span>{t('createLive.fields.description')}</span>
            <textarea
              value={description}
              rows={4}
              maxLength={2000}
              placeholder={t('studio.prepare.descriptionPlaceholder', { defaultValue: 'Tell viewers what this live is about.' })}
              onChange={(event) => setDescription(event.target.value)}
            />
          </label>
          <button className="gl-creator-primary" type="button" disabled={!done1} onClick={() => setStep(2)}>
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
          <CategoryPicker
            categories={categories}
            value={category}
            onChange={setCategory}
          />
          <CoverPicker
            preview={coverPreview}
            onChange={(file) => {
              setCoverFile(file);
              setCoverPreview(file ? URL.createObjectURL(file) : '');
            }}
          />
          <button className="gl-creator-primary" type="button" disabled={!done2} onClick={() => setStep(3)}>
            {t('studio.prepare.next', { defaultValue: 'Next' })}
          </button>
        </StepCard>

        <StepCard
          number={3}
          active={step === 3}
          done={done3}
          locked={!done2}
          title={t('studio.prepare.steps.check', { defaultValue: 'Check publishing and permission' })}
          onOpen={() => done2 && setStep(3)}
        >
          <div className="gl-creator-check-list">
            <CheckRow
              ok={user?.livePermissionStatus === 'approved'}
              label={t('studio.prepare.permissionApproved', { defaultValue: 'Creator permission approved' })}
            />
            <CheckRow
              ok={obsChecked}
              label={t('studio.prepare.obsReady', { defaultValue: 'OBS setup reviewed' })}
            />
          </div>
          <PublisherPreview rtmpServer={rtmpServer()} />
          <button className="gl-creator-secondary" type="button" onClick={() => setObsChecked(true)}>
            <ClipboardCheck size={16} />
            {t('studio.prepare.runCheck', { defaultValue: 'Mark OBS check complete' })}
          </button>
          <button className="gl-creator-primary" type="button" disabled={!done3} onClick={() => setStep(4)}>
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
            <strong>{t('studio.prepare.readyTitle', { defaultValue: 'Ready to create the live room' })}</strong>
            <span>{t('studio.prepare.readyBody', { defaultValue: 'After confirmation, your stream key is issued and the live control console opens.' })}</span>
          </div>
          <button className="gl-creator-start" type="button" disabled={!done3 || starting} onClick={() => void startLive()}>
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
  const ownsStream = Boolean(stream?.ownerId && currentUser?.id && stream.ownerId === currentUser.id);
  const [ended, setEnded] = useState(false);
  const realtime = useRoomRealtime(id, Boolean(id && stream), {
    onLiveEnded: () => setEnded(true),
  });
  const elapsed = useElapsed(stream?.startedAt, roomIsLive && !ended);
  const viewerCount = realtime.viewerCount > 0 ? realtime.viewerCount : (stream?.viewers ?? 0);
  const session = useMemo(() => {
    if (!stream) return null;
    return publisherSessionFromStream(stream) ?? matchingStoredSession(stream.id);
  }, [stream]);

  useEffect(() => {
    if (!id) return;
    void room.refetch();
    void liveRooms.refetch();
    const timer = window.setInterval(() => {
      void room.refetch();
      void liveRooms.refetch();
    }, 3000);
    return () => window.clearInterval(timer);
  }, [id, liveRooms.refetch, room.refetch]);

  useEffect(() => {
    if (!stream?.streamKey || !ownsStream) return;
    savePublisherSession(stream);
  }, [ownsStream, stream]);

  useEffect(() => {
    if (!streamEnded) return;
    localStorage.removeItem(LIVE_SESSION_STORAGE_KEY);
  }, [streamEnded]);

  if (room.isPending && liveRooms.isPending && !stream) {
    return <StudioLoading label={t('studio.loading', { defaultValue: 'Loading studio...' })} />;
  }

  if ((room.isError && !directoryStream) || !stream) {
    return (
      <StudioAccessPage
        icon={<Radio size={24} />}
        title={t('studio.console.notFound', { defaultValue: 'Live room unavailable' })}
        body={t('studio.console.notFoundBody', { defaultValue: 'The live room could not be loaded.' })}
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
        body={t('studio.console.ownerOnlyBody', { defaultValue: 'Only the channel owner can control this live.' })}
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
          defaultValue: 'This live has already finished. Open the studio overview to start a new one.',
        })}
        actionLabel={t('notFound.back')}
        onAction={() => navigate('/studio/overview')}
      />
    );
  }

  const stop = () => {
    stopLive.mutate(undefined, {
      onSuccess: () => {
        localStorage.removeItem(LIVE_SESSION_STORAGE_KEY);
        toast.success(t('studio.console.ended', { defaultValue: 'Live ended.' }));
        navigate('/studio/overview');
      },
      onError: () => toast.error(t('studio.console.endFailed', { defaultValue: 'Could not end the live.' })),
    });
  };

  return (
    <div className="gl-page gl-live-console">
      <header className="gl-live-console-status">
        <div className="gl-live-console-status-main">
          <span className={cn('gl-live-console-pill', roomIsLive ? 'is-live' : 'is-waiting')}>
            <span />
            {roomIsLive ? t('studio.console.live', { defaultValue: 'LIVE' }) : t('studio.console.waiting', { defaultValue: 'Waiting' })}
          </span>
          <StatusMetric label={t('studio.console.duration', { defaultValue: 'Duration' })} value={elapsed} />
          <StatusMetric label={t('studio.console.online', { defaultValue: 'Online' })} value={viewerCount.toLocaleString()} />
          <StatusMetric label={t('studio.console.bitrate', { defaultValue: 'Bitrate' })} value={roomIsLive ? '5.8 Mbps' : '--'} />
          <StatusMetric label={t('studio.console.dropped', { defaultValue: 'Dropped frames' })} value={roomIsLive ? '0.1%' : '--'} />
          <StatusMetric label={t('studio.console.latency', { defaultValue: 'Latency' })} value={roomIsLive ? '2.4s' : '--'} />
        </div>
        <button type="button" className="gl-owner-end-live" disabled={stopLive.isPending} onClick={stop}>
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
        </main>
        <StudioInteractionRail
          stream={stream}
          messages={realtime.messages}
          viewers={realtime.viewers}
          viewerCount={viewerCount}
          bulletsCount={realtime.bullets.length}
          onClearBullets={() => realtime.bullets.forEach((bullet) => realtime.clearBullet(bullet.id))}
          onSendChat={realtime.sendChat}
          reconnecting={realtime.readyState !== 'open'}
          reconnectingLabel={realtime.readyState === 'reconnecting' ? t('studio.console.reconnecting', { count: realtime.retryCount, defaultValue: 'Reconnecting #{{count}}' }) : t('studio.console.disconnected', { defaultValue: 'Disconnected' })}
        />
      </div>
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
        <h1>{t('studio.title', { name: userDisplayName(user), defaultValue: '{{name}} workspace' })}</h1>
      </div>
    </header>
  );
}

function StudioTabs() {
  const { t } = useTranslation('pages');
  return (
    <nav className="gl-creator-tabs" aria-label={t('studio.tabs.label', { defaultValue: 'Creator Studio sections' })}>
      <NavLink to="/studio/overview">{t('studio.tabs.overview', { defaultValue: 'Overview' })}</NavLink>
      <NavLink to="/studio/prepare">{t('studio.tabs.prepare', { defaultValue: 'Stream setup' })}</NavLink>
      <NavLink to="/studio/replay">{t('studio.tabs.replay', { defaultValue: 'Data replay' })}</NavLink>
    </nav>
  );
}

function StudioPermissionPage({
  status,
  applying,
  onApply,
}: {
  status: User['livePermissionStatus'];
  applying: boolean;
  onApply: () => void;
}) {
  const { t } = useTranslation('pages');
  const pending = status === 'pending';
  const rejected = status === 'rejected';
  const title = pending
    ? t('studio.permission.pending.title', { defaultValue: 'Application under review' })
    : rejected
      ? t('studio.permission.rejected.title', { defaultValue: 'Application was rejected' })
      : t('studio.permission.none.title', { defaultValue: 'Apply for creator access' });
  const body = pending
    ? t('studio.permission.pending.body', { defaultValue: 'We are checking your channel and account status.' })
    : rejected
      ? t('studio.permission.rejected.body', { defaultValue: 'Reason: your channel information needs another review before live access can be enabled.' })
      : t('studio.permission.none.body', { defaultValue: 'Creator access is required before opening the streaming workspace.' });

  return (
    <StudioAccessPage
      icon={pending ? <Timer size={24} /> : rejected ? <ShieldCheck size={24} /> : <Send size={24} />}
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
      actionDisabled={pending || applying}
      onAction={onApply}
      details={[
        t('studio.permission.reviewTime', { defaultValue: 'Estimated review time: within 1 business day.' }),
        t('studio.permission.notice', { defaultValue: 'Keep your channel name, avatar, and cover ready for review.' }),
        rejected
          ? t('studio.permission.rejected.reason', { defaultValue: 'Rejected reason: channel readiness did not meet the current creator policy.' })
          : t('studio.permission.progress', { defaultValue: 'Progress: submitted -> manual review -> result.' }),
      ]}
    />
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
}: {
  icon: ReactNode;
  title: string;
  body: string;
  actionLabel: string;
  actionDisabled?: boolean;
  onAction: () => void;
  details?: string[];
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
        <button className="gl-creator-primary" type="button" disabled={actionDisabled} onClick={onAction}>
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

function StudioKpi({ icon, label, value, sub }: { icon: ReactNode; label: string; value: string; sub: string }) {
  return (
    <div className="gl-creator-kpi">
      <div className="gl-creator-kpi-icon">{icon}</div>
      <span>{label}</span>
      <strong>{value}</strong>
      <small>{sub}</small>
    </div>
  );
}

function StudioAction({ icon, title, body, onClick }: { icon: ReactNode; title: string; body: string; onClick: () => void }) {
  return (
    <button type="button" className="gl-creator-action" onClick={onClick}>
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
  onOpen,
  children,
}: {
  number: number;
  active: boolean;
  done: boolean;
  locked: boolean;
  title: string;
  onOpen: () => void;
  children: ReactNode;
}) {
  return (
    <section className={cn('gl-creator-step', active && 'is-active', done && 'is-done', locked && 'is-locked')}>
      <button type="button" className="gl-creator-step-head" disabled={locked} onClick={onOpen}>
        <span>{done ? <CheckCircle2 size={16} /> : number}</span>
        <strong>{title}</strong>
      </button>
      {active && <div className="gl-creator-step-body">{children}</div>}
    </section>
  );
}

function CoverPicker({ preview, onChange }: { preview: string; onChange: (file: File | null) => void }) {
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
      <div className="gl-creator-category-list" role="listbox" aria-label={t('createLive.fields.category')}>
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

function LiveSetupPreview({
  title,
  description,
  category,
  cover,
  status,
}: {
  title: string;
  description: string;
  category: string;
  cover: string;
  status: string;
}) {
  const { t } = useTranslation('pages');
  return (
    <div className="gl-creator-live-preview">
      <div className="gl-creator-live-preview-cover">
        {cover ? <img src={cover} alt="" /> : <Upload size={34} />}
        <span>{status}</span>
      </div>
      <div className="gl-creator-live-preview-copy">
        <strong>{title || t('createLive.defaultTitle')}</strong>
        <span>{category}</span>
        <p>{description || t('studio.prepare.noDescription', { defaultValue: 'Description will appear here.' })}</p>
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
      <PublisherLine label={t('studio.publisher.server', { defaultValue: 'OBS server' })} value={rtmpServer} copyLabel="OBS server" />
      <div className="gl-creator-key-placeholder">
        <span>{t('studio.publisher.key', { defaultValue: 'Stream key' })}</span>
        <strong>{t('studio.publisher.keyAfterStart', { defaultValue: 'Issued after confirmation' })}</strong>
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
        <button type="button" className="gl-creator-icon-button" onClick={onRefresh} aria-label={t('studio.publisher.refresh', { defaultValue: 'Refresh status' })}>
          <ClipboardCheck size={17} />
        </button>
      </div>
      <div className="gl-creator-publisher-grid">
        <PublisherLine label={t('studio.publisher.server', { defaultValue: 'OBS server' })} value={session.rtmpServer} copyLabel="OBS server" />
        <PublisherLine label={t('studio.publisher.key', { defaultValue: 'Stream key' })} value={session.streamKey} secret copyLabel="Stream key" />
        {streamUrl && <PublisherLine label={t('studio.publisher.playback', { defaultValue: 'Playback URL' })} value={streamUrl} copyLabel="Playback URL" />}
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
      toast.success(method === 'manual' ? `${copyLabel} opened for manual copy.` : `${copyLabel} copied.`);
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
      {stream.cover && <img src={stream.cover} alt="" />}
      <div>
        <Radio size={34} />
        <strong>{t('studio.console.waitingPreview', { defaultValue: 'Waiting for publisher' })}</strong>
        <span>{t('studio.console.waitingPreviewBody', { defaultValue: 'Add the stream server and key in OBS, then start streaming.' })}</span>
      </div>
    </div>
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
  reconnecting: boolean;
  reconnectingLabel: string;
}) {
  const { t } = useTranslation('pages');
  const gifts = summarizeGifts(messages);
  const superChats = messages.filter((item) => item.kind === 'super_chat').slice(-3).reverse();

  return (
    <aside className="gl-live-console-rail">
      <section className="gl-creator-panel">
        <div className="gl-creator-panel-head">
          <div>
            <span>{t('studio.console.danmu', { defaultValue: 'Danmu management' })}</span>
            <h2>{t('studio.console.chatControl', { defaultValue: 'Live chat' })}</h2>
          </div>
          <MessageSquare size={22} />
        </div>
        <div className="gl-live-console-danmu-row">
          <span>{t('studio.console.activeBullets', { count: bulletsCount, defaultValue: '{{count}} active bullets' })}</span>
          <button type="button" onClick={onClearBullets} disabled={bulletsCount === 0}>
            {t('studio.console.clearPreview', { defaultValue: 'Clear preview' })}
          </button>
        </div>
        <Chat
          messages={messages}
          viewers={viewers}
          viewerTotal={viewerCount}
          ownerId={stream.ownerId}
          ownerName={stream.channel}
          onSendChat={onSendChat}
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
          {gifts.items.length > 0 ? (
            gifts.items.map((item) => (
              <div className="gl-live-console-gift-row" key={item.name}>
                <span>{item.icon || item.name}</span>
                <strong>{item.name}</strong>
                <small>x{item.count}</small>
              </div>
            ))
          ) : (
            <div className="gl-creator-empty-soft">{t('studio.console.noGifts', { defaultValue: 'No gifts yet.' })}</div>
          )}
        </div>
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
          {superChats.length > 0 ? (
            superChats.map((item) => (
              <div key={item.id}>
                <strong>{item.user}</strong>
                <span>{item.amount}</span>
                <p>{item.text}</p>
              </div>
            ))
          ) : (
            <div className="gl-creator-empty-soft">{t('studio.console.noSuperChat', { defaultValue: 'No SuperChat pinned.' })}</div>
          )}
        </div>
      </section>
    </aside>
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
  const map = new Map<string, { name: string; icon?: string; count: number; totalCoin: number }>();
  for (const item of messages) {
    if (item.kind !== 'gift') continue;
    const current = map.get(item.giftName) ?? {
      name: item.giftName,
      icon: item.giftIcon,
      count: 0,
      totalCoin: 0,
    };
    current.count += item.count ?? 1;
    current.totalCoin += item.totalCoin ?? 0;
    if (item.giftIcon) current.icon = item.giftIcon;
    map.set(item.giftName, current);
  }
  const items = Array.from(map.values()).sort((a, b) => b.totalCoin - a.totalCoin).slice(0, 5);
  return { items, totalCoin: items.reduce((sum, item) => sum + item.totalCoin, 0) };
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
