import { AppointmentCard } from '@/components/AppointmentCard';
import type { AppointmentItem } from '@/api/room';

export function AppointmentViewerCard({
  appointment,
  to,
}: {
  appointment: AppointmentItem;
  to?: string;
}) {
  return <AppointmentCard appointment={appointment} to={to} />;
}
