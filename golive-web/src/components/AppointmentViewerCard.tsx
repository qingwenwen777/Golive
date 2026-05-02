import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useIsAuthed } from '@/stores/useAuthStore';
import { AppointmentCard } from '@/components/AppointmentCard';
import { useReserveAppointment, useUnreserveAppointment, type AppointmentItem } from '@/api/room';

export function AppointmentViewerCard({
  appointment,
  to,
  compact = false,
}: {
  appointment: AppointmentItem;
  to?: string;
  compact?: boolean;
}) {
  const { t } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const reserve = useReserveAppointment(appointment.id);
  const unreserve = useUnreserveAppointment(appointment.id);
  const pending = reserve.isPending || unreserve.isPending;

  return (
    <AppointmentCard
      appointment={appointment}
      to={to}
      compact={compact}
      viewerMode
      pending={pending}
      onReserve={() => {
        if (!isAuthed) {
          openLogin();
          return;
        }
        reserve.mutate(undefined, {
          onSuccess: () => toast.success(t('appointments.reserved', { defaultValue: 'Reservation saved.' })),
          onError: (err) => toast.error(err.message || t('appointments.reserveFailed', { defaultValue: 'Could not reserve this appointment.' })),
        });
      }}
      onUnreserve={() => {
        if (!isAuthed) {
          openLogin();
          return;
        }
        unreserve.mutate(undefined, {
          onSuccess: () => toast.success(t('appointments.unreserved', { defaultValue: 'Reservation removed.' })),
          onError: (err) => toast.error(err.message || t('appointments.reserveFailed', { defaultValue: 'Could not update this reservation.' })),
        });
      }}
    />
  );
}
