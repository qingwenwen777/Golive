import { BadgeCheck } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/cn';

export function VerifiedBadge({
  size = 16,
  className,
}: {
  size?: number;
  className?: string;
}) {
  const { t } = useTranslation('common');
  return (
    <BadgeCheck
      size={size}
      className={cn('gl-platform-verified-badge', className)}
      role="img"
      aria-label={t('badges.platformVerified', { defaultValue: 'Platform verified creator' })}
    />
  );
}
