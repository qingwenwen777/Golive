import { Sparkles } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/cn';

export function FanClubExclusiveBadge({
  compact = false,
  className,
}: {
  compact?: boolean;
  className?: string;
}) {
  const { t } = useTranslation('pages');
  return (
    <span className={cn('gl-fan-exclusive-badge', compact && 'is-compact', className)}>
      <Sparkles size={compact ? 12 : 14} />
      {t('liveRoom.fanClubExclusive.badge', { defaultValue: 'Members only' })}
    </span>
  );
}
