import { useEffect, useMemo, useState, type MouseEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { Check, CheckCircle2, Clock3, ListPlus, Pencil, PlayCircle, Trash2, X } from 'lucide-react';
import { toast } from 'sonner';
import { useReserveAppointment } from '@/api/room';
import { Avatar } from '@/components/Avatar';
import { LoadableImage } from '@/components/LoadableImage';
import { cn } from '@/lib/cn';
import { isInLibrary, saveToLibrary, WATCH_LATER_KEY } from '@/lib/liveLibrary';
import { useCoverHoverStyle } from '@/hooks/useCoverHoverStyle';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useIsAuthed } from '@/stores/useAuthStore';
import type { AppointmentItem } from '@/api/room';
import type { Stream } from '@/types/stream';

export interface AppointmentCardProps {
  appointment: AppointmentItem;
  to?: string;
  onStart?: () => void;
  onEdit?: () => void;
  onDelete?: () => void;
  onDeleteRecord?: () => void;
  pending?: boolean;
  showChannel?: boolean;
  managementMode?: boolean;
}

export function AppointmentCard({
  appointment,
  to,
  onStart,
  onEdit,
  onDelete,
  onDeleteRecord,
  pending = false,
  showChannel = true,
  managementMode = false,
}: AppointmentCardProps) {
  const { t, i18n } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const reserveAppointment = useReserveAppointment(appointment.id);
  const [reserved, setReserved] = useState(appointment.reserved);
  const [waitingCount, setWaitingCount] = useState(
    appointment.waitingCount ?? appointment.reservationCount,
  );
  const [savedLater, setSavedLater] = useState(() =>
    isInLibrary(WATCH_LATER_KEY, appointment.roomId),
  );
  const scheduled = useMemo(
    () =>
      new Intl.DateTimeFormat(i18n.language, {
        year: 'numeric',
        month: 'numeric',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      }).format(new Date(appointment.scheduledAt)),
    [appointment.scheduledAt, i18n.language],
  );
  const category =
    appointment.category && appointment.category !== 'Scheduled'
      ? appointment.category
      : 'Just Chatting';
  const categoryLabel = t(`createLive.categories.${categoryKey(category)}`, {
    defaultValue: category,
  });
  const channelName = appointment.channel || `Creator ${appointment.ownerId.slice(0, 8)}`;
  const appointmentLabel = t('liveRoom.scheduledBadge', { defaultValue: 'Appointment' });
  const hoverStyle = useCoverHoverStyle(
    appointment.cover,
    channelName || appointment.title || appointment.id,
  );

  useEffect(() => {
    setReserved(appointment.reserved);
    setWaitingCount(appointment.waitingCount ?? appointment.reservationCount);
    setSavedLater(isInLibrary(WATCH_LATER_KEY, appointment.roomId));
  }, [
    appointment.reserved,
    appointment.reservationCount,
    appointment.roomId,
    appointment.waitingCount,
  ]);

  const reserveLabel = reserved
    ? t('liveRoom.scheduledReservedByYou', { defaultValue: '你已预约' })
    : t('liveRoom.quickReserve', { defaultValue: '快速预约' });
  const watchLaterLabel = savedLater
    ? t('liveRoom.saved', { defaultValue: '已保存' })
    : t('liveRoom.quickWatchLater', { defaultValue: '添加到稍后观看' });
  const waitingLabel = t('liveRoom.appointmentWaiting', {
    count: waitingCount,
    defaultValue: '{{count}} 人正在等待',
  });
  const premiereLabel = t('liveRoom.appointmentPremiereTime', {
    time: scheduled,
    defaultValue: '首播时间：{{time}}',
  });

  const stopQuickAction = (event: MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    event.stopPropagation();
  };

  const handleQuickReserve = (event: MouseEvent<HTMLButtonElement>) => {
    stopQuickAction(event);
    if (reserved || reserveAppointment.isPending) return;
    if (!isAuthed) {
      openLogin();
      return;
    }
    reserveAppointment.mutate(undefined, {
      onSuccess: (next) => {
        setReserved(true);
        setWaitingCount(next.waitingCount ?? waitingCount);
        toast.success(t('liveRoom.appointmentReserved', { defaultValue: '已预约直播。' }));
      },
      onError: (err) => {
        toast.error(
          err.message ||
            t('liveRoom.appointmentReserveFailed', { defaultValue: '预约状态更新失败。' }),
        );
      },
    });
  };

  const handleWatchLater = (event: MouseEvent<HTMLButtonElement>) => {
    stopQuickAction(event);
    if (savedLater) return;
    saveToLibrary(WATCH_LATER_KEY, appointmentToStream(appointment, waitingCount));
    setSavedLater(true);
    toast.success(t('liveRoom.savedWatchLater', { defaultValue: '已保存到稍后观看。' }));
  };

  const main = (
    <>
      <div className={cn('gl-appointment-cover', appointment.cover && 'has-image')}>
        {appointment.cover ? (
          <LoadableImage src={appointment.cover} alt="" loading="lazy" />
        ) : (
          <span>{channelName.slice(0, 1).toUpperCase()}</span>
        )}
        {!managementMode && (
          <div className="gl-appointment-starting-badge">
            <Clock3 size={12} />
            {t('liveRoom.appointmentStartingSoon', { defaultValue: '即将开始' })}
          </div>
        )}
        {managementMode && (
          <div className="gl-appointment-scheduled">
            <Clock3 size={13} />
            {scheduled}
          </div>
        )}
      </div>
      <div className="gl-appointment-body">
        {showChannel && <Avatar name={channelName} src={appointment.avatar} size={44} />}
        <div className="gl-appointment-copy">
          <div className="gl-appointment-head">
            <h3>{appointment.title}</h3>
            {appointment.verified && <CheckCircle2 size={15} />}
          </div>
          {showChannel && <div className="gl-appointment-channel">{channelName}</div>}
          <div className="gl-appointment-meta">
            {managementMode ? (
              <>
                {appointmentLabel} · {categoryLabel}
              </>
            ) : (
              <>
                <span>{waitingLabel}</span>
                <span>·</span>
                <span>{premiereLabel}</span>
              </>
            )}
          </div>
        </div>
      </div>
    </>
  );

  const content = to ? (
    <Link to={to} className="gl-appointment-link">
      {main}
    </Link>
  ) : (
    <div className="gl-appointment-main">{main}</div>
  );

  return (
    <article
      className={cn(
        'gl-appointment-card',
        !managementMode && 'gl-video-hover-card',
        managementMode && 'is-management',
      )}
      style={managementMode ? undefined : hoverStyle}
    >
      {content}
      {!managementMode && (
        <div className="gl-appointment-quick-actions" aria-hidden={false}>
          <button
            type="button"
            className={cn('gl-appointment-quick-btn', reserved && 'is-complete')}
            disabled={reserveAppointment.isPending}
            aria-label={reserveLabel}
            title={reserveLabel}
            onClick={handleQuickReserve}
          >
            {reserved ? <Check size={20} /> : <Clock3 size={20} />}
          </button>
          <button
            type="button"
            className={cn('gl-appointment-quick-btn', savedLater && 'is-complete')}
            aria-label={watchLaterLabel}
            title={watchLaterLabel}
            onClick={handleWatchLater}
          >
            {savedLater ? <Check size={20} /> : <ListPlus size={21} />}
          </button>
        </div>
      )}
      {managementMode && (
        <div className="gl-appointment-actions">
          {appointment.canStart && (
            <button type="button" className="gl-retry-btn" disabled={pending} onClick={onStart}>
              <PlayCircle size={15} />
              {t('appointments.start', { defaultValue: 'Start live' })}
            </button>
          )}
          <button type="button" className="gl-secondary-btn" disabled={pending} onClick={onEdit}>
            <Pencil size={15} />
            {t('appointments.edit', { defaultValue: 'Edit' })}
          </button>
          {appointment.status === 'scheduled' && (
            <button
              type="button"
              className="gl-secondary-btn"
              disabled={pending}
              onClick={onDelete}
            >
              <X size={15} />
              {t('appointments.cancelAppointment', { defaultValue: '取消预约' })}
            </button>
          )}
          <button
            type="button"
            className="gl-secondary-btn is-danger"
            disabled={pending || appointment.status === 'live'}
            onClick={onDeleteRecord}
          >
            <Trash2 size={15} />
            {t('appointments.deleteRecord', { defaultValue: '彻底删除' })}
          </button>
        </div>
      )}
    </article>
  );
}

function categoryKey(category: string): string {
  return category.toLowerCase().replace(/\s+/g, '');
}

function appointmentToStream(appointment: AppointmentItem, waitingCount: number): Stream {
  return {
    id: appointment.roomId,
    title: appointment.title,
    description: appointment.description,
    channel: appointment.channel,
    channelId: appointment.channelId,
    verified: appointment.verified,
    avatar: appointment.avatar,
    cover: appointment.cover,
    viewers: waitingCount,
    duration: '',
    category: appointment.category,
    categoryJa: appointment.categoryJa,
    startedAt: appointment.scheduledAt,
    endedAt: appointment.endedAt,
    isLive: false,
    ownerId: appointment.ownerId,
    status: appointment.status,
    subscriberCount: appointment.reservationCount,
  };
}
