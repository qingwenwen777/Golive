import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { CheckCircle2, Clock3, Pencil, PlayCircle, Trash2, X } from 'lucide-react';
import { Avatar } from '@/components/Avatar';
import { LoadableImage } from '@/components/LoadableImage';
import { cn } from '@/lib/cn';
import type { AppointmentItem } from '@/api/room';

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
  const channelName = appointment.channel || `Creator ${appointment.ownerId.slice(0, 8)}`;
  const appointmentLabel = t('liveRoom.scheduledBadge', { defaultValue: 'Appointment' });

  const main = (
    <>
      <div className={cn('gl-appointment-cover', appointment.cover && 'has-image')}>
        {appointment.cover ? (
          <LoadableImage src={appointment.cover} alt="" loading="lazy" />
        ) : (
          <span>{channelName.slice(0, 1).toUpperCase()}</span>
        )}
        <div className="gl-appointment-scheduled">
          <Clock3 size={13} />
          {scheduled}
        </div>
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
            {appointmentLabel} · {categoryLabel}
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
    <article className={cn('gl-appointment-card', managementMode && 'is-management')}>
      {content}
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
