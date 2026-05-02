import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Navigate, NavLink, Outlet, useNavigate, useParams } from 'react-router-dom';
import {
  BarChart3,
  CheckCircle2,
  ClipboardCheck,
  Copy,
  CalendarClock,
  Eye,
  Gift,
  ImagePlus,
  ListChecks,
  MessageSquare,
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
  Clock3,
  ChevronLeft,
  ChevronRight,
} from 'lucide-react';
import { toast } from 'sonner';
import { useMe } from '@/api/auth';
import { useSubmitCreatorApplication } from '@/api/creator';
import {
  useCancelAppointment,
  useCreateAppointment,
  useCreatorAnalytics,
  useGoLive,
  useRoom,
  useRooms,
  useStartAppointment,
  useStudioAppointments,
  useStopLive,
  useUpdateAppointment,
  useUpdateLiveMetadata,
  useUploadLiveCover,
  type AppointmentItem,
} from '@/api/room';
import { Avatar } from '@/components/Avatar';
import { AppointmentCard } from '@/components/AppointmentCard';
import { Chat } from '@/features/live-room/Chat';
import { BettingPanel } from '@/features/live-room/BettingPanel';
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
        rejectReason={user?.livePermissionRejectReason}
        applying={apply.isPending}
        onApply={(reason) => {
          apply.mutate({ reason }, {
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
              <div className={`gl-creator-latest-cover${latest.cover ? ' has-image' : ''}`}>
                {latest.cover && <LoadableImage src={latest.cover} alt="" />}
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
  const appointments = useStudioAppointments(Boolean(user), 1, 5);
  const dueAppointment = appointments.data?.items.find((item) => item.status === 'scheduled' && item.canStart);

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
    if (dueAppointment) {
      toast.info(t('studio.prepare.appointmentRequired', { defaultValue: 'A scheduled appointment is ready. Please start from Live appointments.' }));
      navigate('/studio/appointments');
      return;
    }
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
          {dueAppointment && (
            <div className="gl-creator-confirm">
              <strong>{t('studio.prepare.appointmentRequiredTitle', { defaultValue: 'Start from your appointment' })}</strong>
              <span>{dueAppointment.title}</span>
            </div>
          )}
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

const APPOINTMENT_STATS_PAGE_SIZE = 100;
const APPOINTMENT_LIST_PAGE_SIZE = 4;

export function CreatorAppointmentsPage() {
  const { t } = useTranslation('pages');
  const { user } = useStudioUser();
  const channelKey = currentChannelKey(user);
  const [appointmentPage, setAppointmentPage] = useState(1);
  const appointmentStats = useStudioAppointments(Boolean(channelKey), 1, APPOINTMENT_STATS_PAGE_SIZE);
  const appointments = useStudioAppointments(Boolean(channelKey), appointmentPage, APPOINTMENT_LIST_PAGE_SIZE);
  const createAppointment = useCreateAppointment();
  const uploadCover = useUploadLiveCover();
  const [editing, setEditing] = useState<AppointmentItem | null>(null);
  const updateAppointment = useUpdateAppointment(editing?.id ?? '');
  const [scheduledAt, setScheduledAt] = useState(() => toLocalDateTimeInput(new Date(Date.now() + 60 * 60 * 1000)));
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [coverFile, setCoverFile] = useState<File | null>(null);
  const [coverPreview, setCoverPreview] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    setAppointmentPage(1);
  }, [channelKey]);

  useEffect(() => {
    if (!editing) return;
    setTitle(editing.title);
    setDescription(editing.description ?? '');
    setScheduledAt(toLocalDateTimeInput(editing.scheduledAt));
    setCoverPreview(editing.cover ?? '');
    setCoverFile(null);
    setError('');
  }, [editing]);

  useEffect(() => {
    return () => {
      if (coverPreview.startsWith('blob:')) URL.revokeObjectURL(coverPreview);
    };
  }, [coverPreview]);

  const clearDraft = () => {
    setEditing(null);
    setTitle('');
    setDescription('');
    setScheduledAt(toLocalDateTimeInput(new Date(Date.now() + 60 * 60 * 1000)));
    setCoverFile(null);
    setCoverPreview('');
    setError('');
  };

  const saveAppointment = async () => {
    if (!user) return;
    const scheduled = new Date(scheduledAt);
    const normalizedTitle = title.trim();
    const normalizedDescription = description.trim();
    if (Number.isNaN(scheduled.getTime())) {
      setError(t('studio.appointments.invalidTime', { defaultValue: 'Please pick a valid future time.' }));
      return;
    }
    if (scheduled.getTime() <= Date.now()) {
      setError(t('studio.appointments.invalidTime', { defaultValue: 'Please pick a valid future time.' }));
      return;
    }
    if (!normalizedTitle || !normalizedDescription || !coverPreview.trim()) {
      setError(t('studio.appointments.formIncomplete', { defaultValue: 'Start time, title, cover, and description are required.' }));
      return;
    }
    setError('');
    try {
      const cover = coverFile ? (await uploadCover.mutateAsync(coverFile)).url : coverPreview;
      const payload = {
        scheduledAt: scheduled.toISOString(),
        title: normalizedTitle,
        description: normalizedDescription,
        cover,
        channelName: userDisplayName(user),
        avatar: user.avatar,
      };
      if (!editing) {
        await createAppointment.mutateAsync(payload);
        toast.success(t('studio.appointments.created', { defaultValue: 'Appointment published.' }));
        setAppointmentPage(1);
      } else {
        await updateAppointment.mutateAsync(payload);
        toast.success(t('studio.appointments.updated', { defaultValue: 'Appointment updated.' }));
      }
      clearDraft();
      void appointments.refetch();
      void appointmentStats.refetch();
    } catch (err) {
      setError(err instanceof Error ? err.message : t('studio.appointments.saveFailed', { defaultValue: 'Could not save the appointment.' }));
    }
  };

  const items = appointments.data?.items ?? [];
  const statsItems = appointmentStats.data?.items ?? items;
  const total = appointmentStats.data?.total ?? appointments.data?.total ?? 0;
  const appointmentPageCount = Math.max(1, Math.ceil((appointments.data?.total ?? 0) / APPOINTMENT_LIST_PAGE_SIZE));
  const upcoming = statsItems.filter((item) => item.status === 'scheduled').length;
  const live = statsItems.filter((item) => item.status === 'live').length;
  const completed = statsItems.filter((item) => item.status === 'completed').length;

  useEffect(() => {
    if (!appointments.data) return;
    const nextPageCount = Math.max(1, Math.ceil(appointments.data.total / APPOINTMENT_LIST_PAGE_SIZE));
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

      <section className="gl-creator-dashboard-grid gl-appointments-grid">
        <div className="gl-creator-panel gl-appointment-form-panel">
          <div className="gl-creator-panel-head">
            <div>
              <span>{t('studio.appointments.formLabel', { defaultValue: 'Live appointments' })}</span>
              <h2>{editing ? t('studio.appointments.editTitle', { defaultValue: 'Edit appointment' }) : t('studio.appointments.createTitle', { defaultValue: 'Create appointment' })}</h2>
            </div>
            <button type="button" className="gl-creator-secondary" onClick={clearDraft} disabled={!title && !description && !coverPreview && !editing}>
              {t('studio.appointments.reset', { defaultValue: 'Reset' })}
            </button>
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
                  <input value={title} maxLength={120} onChange={(event) => setTitle(event.target.value)} />
                </label>
              </div>
              <label className="gl-creator-field">
                <span>{t('createLive.fields.description')}</span>
                <textarea
                  value={description}
                  rows={4}
                  maxLength={2000}
                  placeholder={t('studio.appointments.descriptionPlaceholder', { defaultValue: 'Tell viewers what this appointment is about.' })}
                  onChange={(event) => setDescription(event.target.value)}
                />
              </label>
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
          <button type="button" className="gl-creator-primary gl-appointment-submit" onClick={() => void saveAppointment()} disabled={createAppointment.isPending || updateAppointment.isPending || uploadCover.isPending}>
            <Save size={16} />
            {editing
              ? t('studio.appointments.saveEdit', { defaultValue: 'Save changes' })
              : t('studio.appointments.publish', { defaultValue: 'Publish appointment' })}
          </button>
        </div>

        <div className="gl-creator-panel">
          <div className="gl-creator-panel-head">
            <div>
              <span>{t('studio.appointments.listLabel', { defaultValue: 'My appointments' })}</span>
              <h2>{t('studio.appointments.listTitle', { defaultValue: 'All appointment states' })}</h2>
            </div>
            <Bell size={22} />
          </div>
          <div className="gl-appointment-list-wrap">
            <div className="gl-appointment-list">
              {appointments.isPending ? (
                <div className="gl-creator-empty-soft">{t('studio.loading', { defaultValue: 'Loading studio...' })}</div>
              ) : items.length === 0 ? (
                <div className="gl-creator-empty-soft">{t('studio.appointments.empty', { defaultValue: 'No appointments yet.' })}</div>
              ) : (
                items.map((item) => (
                  <AppointmentStudioRow
                    key={item.id}
                    item={item}
                    onEdit={() => setEditing(item)}
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
    </div>
  );
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
  const mutable = item.status === 'scheduled';
  const pending = startAppointment.isPending || cancelAppointment.isPending;

  return (
    <AppointmentCard
      appointment={item}
      to={item.status === 'scheduled' ? `/live/${encodeURIComponent(item.roomId)}` : undefined}
      compact
      managementMode
      pending={pending}
      onStart={() => {
        startAppointment.mutate(undefined, {
          onSuccess: (stream) => {
            savePublisherSession(stream);
            toast.success(t('studio.appointments.started', { defaultValue: 'Appointment live started.' }));
            navigate(`/studio/live/${encodeURIComponent(stream.id)}`);
          },
          onError: (err) => toast.error(err.message || t('studio.appointments.startFailed', { defaultValue: 'Could not start the appointment.' })),
        });
      }}
      onEdit={() => {
        if (!mutable) {
          toast.info(t('studio.appointments.locked', { defaultValue: 'This appointment can no longer be edited.' }));
          return;
        }
        onEdit();
      }}
      onDelete={() => {
        if (!mutable) {
          toast.info(t('studio.appointments.locked', { defaultValue: 'This appointment can no longer be edited.' }));
          return;
        }
        cancelAppointment.mutate(undefined, {
          onSuccess: () => {
            toast.success(t('studio.appointments.deleted', { defaultValue: 'Appointment deleted.' }));
            onUpdated();
          },
          onError: (err) => toast.error(err.message || t('studio.appointments.deleteFailed', { defaultValue: 'Could not delete the appointment.' })),
        });
      }}
    />
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
  const selected = useMemo(() => parseLocalDateTimeValue(value) ?? new Date(Date.now() + 60 * 60 * 1000), [value]);
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
    <div className="gl-appointment-datetime">
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
          <div className="gl-appointment-calendar-head">
            <button type="button" aria-label={t('studio.appointments.previousMonth', { defaultValue: 'Previous month' })} onClick={() => setViewMonth(addMonths(viewMonth, -1))}>
              <ChevronLeft size={17} />
            </button>
            <strong>{formatMonthLabel(viewMonth, i18n.language)}</strong>
            <button type="button" aria-label={t('studio.appointments.nextMonth', { defaultValue: 'Next month' })} onClick={() => setViewMonth(addMonths(viewMonth, 1))}>
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
          <div className="gl-appointment-time-row">
            <label>
              <span>{t('studio.appointments.hour', { defaultValue: 'Hour' })}</span>
              <select value={selectedHour} onChange={(event) => onChange(replaceTimePart(value, event.target.value, selectedMinute))}>
                {Array.from({ length: 24 }).map((_, index) => {
                  const hour = pad2(index);
                  return <option key={hour} value={hour}>{hour}</option>;
                })}
              </select>
            </label>
            <label>
              <span>{t('studio.appointments.minute', { defaultValue: 'Minute' })}</span>
              <select value={selectedMinute} onChange={(event) => onChange(replaceTimePart(value, selectedHour, event.target.value))}>
                {minuteOptions(selectedMinute).map((minute) => (
                  <option key={minute} value={minute}>{minute}</option>
                ))}
              </select>
            </label>
          </div>
          <div className="gl-appointment-datetime-actions">
            <button type="button" className="gl-creator-secondary" onClick={() => onChange(toLocalDateTimeInput(new Date(Date.now() + 60 * 60 * 1000)))}>
              {t('studio.appointments.oneHourLater', { defaultValue: '1 hour later' })}
            </button>
            <button type="button" className="gl-creator-primary" onClick={() => setOpen(false)}>
              {t('studio.appointments.done', { defaultValue: 'Done' })}
            </button>
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
    <div className="gl-history-pager gl-appointment-pager" aria-label={t('appointments.pagination', { defaultValue: 'Appointment pagination' })}>
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
        <span className="gl-appointment-pager-current">{page} / {pageCount}</span>
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
  return Array.from({ length: 42 }, (_, index) => new Date(start.getFullYear(), start.getMonth(), start.getDate() + index));
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
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
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
        clearPublisherSession();
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
          <LiveMetadataEditor
            stream={stream}
            onUpdated={() => {
              void room.refetch();
              void liveRooms.refetch();
            }}
          />
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
      <NavLink to="/studio/appointments">{t('studio.tabs.appointments', { defaultValue: 'Live appointments' })}</NavLink>
      <NavLink to="/studio/replay">{t('studio.tabs.replay', { defaultValue: 'Data replay' })}</NavLink>
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
      actionDisabled={pending || applying || (!pending && !reason.trim())}
      onAction={() => onApply(reason.trim())}
      details={[
        t('studio.permission.reviewTime', { defaultValue: 'Estimated review time: within 1 business day.' }),
        t('studio.permission.notice', { defaultValue: 'Keep your channel name, avatar, and cover ready for review.' }),
        rejected
          ? t('studio.permission.rejected.reason', {
              reason: rejectReason || t('studio.permission.rejected.reasonFallback', { defaultValue: 'Channel readiness did not meet the current creator policy.' }),
              defaultValue: 'Rejected reason: {{reason}}',
            })
          : t('studio.permission.progress', { defaultValue: 'Progress: submitted -> manual review -> result.' }),
      ]}
    >
      {!pending && (
        <label className="gl-creator-access-reason">
          <span>{t('studio.permission.reasonLabel', { defaultValue: 'Why do you want to go live?' })}</span>
          <textarea
            value={reason}
            rows={5}
            maxLength={500}
            onChange={(event) => setReason(event.target.value)}
            placeholder={t('studio.permission.reasonPlaceholder', {
              defaultValue: 'Tell admins your live content plan and why this channel needs live access.',
            })}
          />
        </label>
      )}
    </StudioAccessPage>
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
      {stream.cover && <LoadableImage src={stream.cover} alt="" />}
      <div>
        <Radio size={34} />
        <strong>{t('studio.console.waitingPreview', { defaultValue: 'Waiting for publisher' })}</strong>
        <span>{t('studio.console.waitingPreviewBody', { defaultValue: 'Add the stream server and key in OBS, then start streaming.' })}</span>
      </div>
    </div>
  );
}

function LiveMetadataEditor({
  stream,
  onUpdated,
}: {
  stream: Stream;
  onUpdated: () => void;
}) {
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
      toast.error(t('studio.console.metadataTitleRequired', { defaultValue: 'Title is required.' }));
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
          : t('studio.console.metadataFailed', { defaultValue: 'Could not update live room info.' }),
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
      <section className="gl-creator-panel gl-live-console-chat-panel">
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
