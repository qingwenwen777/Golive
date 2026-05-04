import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import {
  Bell,
  CalendarClock,
  CheckCircle2,
  Clock3,
  Pencil,
  PlayCircle,
  Tag,
  Trash2,
  UserPlus,
  X,
} from 'lucide-react';
import { Avatar } from '@/components/Avatar';
import { LoadableImage } from '@/components/LoadableImage';
import { cn } from '@/lib/cn';
import type { AppointmentItem } from '@/api/room';

export interface AppointmentCardProps {
  appointment: AppointmentItem;
  to?: string;
  compact?: boolean;
  viewerMode?: boolean;
  onReserve?: () => void;
  onUnreserve?: () => void;
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
  compact = false,
  viewerMode = false,
  onReserve,
  onUnreserve,
  onStart,
  onEdit,
  onDelete,
  onDeleteRecord,
  pending = false,
  showChannel = true,
  managementMode = false,
}: AppointmentCardProps) {
  const { t, i18n } = useTranslation('pages');
  const scheduled = useMemo(
    () =>
      new Intl.DateTimeFormat(i18n.language, {
        month: 'short',
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

  const main = (
    <>
      <div className={cn('gl-appointment-cover', compact && 'is-compact')}>
        {appointment.cover ? <LoadableImage src={appointment.cover} alt="" /> : <span />}
        <div className="gl-appointment-cover-overlay">
          <span className={cn('gl-appointment-status', `is-${appointment.status}`)}>
            {statusLabel(appointment.status, t)}
          </span>
          <div className="gl-appointment-scheduled">
            <Clock3 size={13} />
            {scheduled}
          </div>
        </div>
      </div>
      <div className="gl-appointment-copy">
        <div className="gl-appointment-head">
          <h3>{appointment.title}</h3>
          {appointment.verified && <CheckCircle2 size={15} />}
        </div>
        {showChannel && (
          <div className="gl-appointment-channel">
            <Avatar name={appointment.channel} src={appointment.avatar} size={24} />
            <span>{appointment.channel || `Creator ${appointment.ownerId.slice(0, 8)}`}</span>
          </div>
        )}
        {appointment.description && <p>{appointment.description}</p>}
        <div className="gl-appointment-meta">
          <span>
            <Tag size={13} />
            {categoryLabel}
          </span>
          <span>
            <Bell size={13} />
            {t('appointments.reservationCount', {
              count: appointment.reservationCount,
              defaultValue: '{{count}} reserved',
            })}
          </span>
          <span>
            <CalendarClock size={13} />
            {appointment.canStart
              ? t('appointments.startWindow', { defaultValue: 'Start available now' })
              : t('appointments.waitingWindow', { defaultValue: 'Waiting to start' })}
          </span>
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
        compact && 'is-compact',
        managementMode && 'is-management',
      )}
    >
      {content}
      <div className="gl-appointment-actions">
        {viewerMode ? (
          appointment.reserved ? (
            <button
              type="button"
              className="gl-secondary-btn"
              disabled={pending}
              onClick={onUnreserve}
            >
              <X size={15} />
              {t('appointments.cancelReserve', { defaultValue: 'Cancel' })}
            </button>
          ) : (
            <button type="button" className="gl-retry-btn" disabled={pending} onClick={onReserve}>
              <UserPlus size={15} />
              {t('appointments.reserve', { defaultValue: 'Reserve' })}
            </button>
          )
        ) : (
          <>
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
          </>
        )}
      </div>
    </article>
  );
}

function statusLabel(status: string, t: ReturnType<typeof useTranslation>['t']): string {
  switch (status) {
    case 'scheduled':
      return t('appointments.status.scheduled', { defaultValue: 'Scheduled' });
    case 'live':
      return t('appointments.status.live', { defaultValue: 'Live' });
    case 'completed':
      return t('appointments.status.completed', { defaultValue: 'Completed' });
    case 'expired':
      return t('appointments.status.expired', { defaultValue: 'Expired' });
    case 'canceled':
      return t('appointments.status.canceled', { defaultValue: 'Canceled' });
    default:
      return status;
  }
}

function categoryKey(category: string): string {
  return category.toLowerCase().replace(/\s+/g, '');
}
