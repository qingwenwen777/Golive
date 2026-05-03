import { Crown, Gem, Sparkles, ShieldCheck } from 'lucide-react';
import { cn } from '@/lib/cn';
import { normalizeLevelInfo, userLevelToneClass } from '@/lib/userLevel';
import type { UserLevelInfo } from '@/types/user';

type UserLevelBadgeSize = 'compact' | 'default' | 'hero';

export function UserLevelBadge({
  levelInfo,
  level,
  size = 'default',
  className,
  title,
}: {
  levelInfo?: UserLevelInfo | null;
  level?: number;
  size?: UserLevelBadgeSize;
  className?: string;
  title?: string;
}) {
  const info = normalizeLevelInfo(levelInfo);
  const resolvedLevel = Math.max(1, Math.min(info.maxLevel, Math.floor(level ?? info.level)));
  const Icon = levelIcon(resolvedLevel);

  return (
    <span
      className={cn(
        'gl-user-level-badge',
        userLevelToneClass(resolvedLevel),
        resolvedLevel >= 80 && 'is-elite',
        size === 'compact' && 'is-compact',
        size === 'hero' && 'is-hero',
        className,
      )}
      title={title ?? `Lv.${resolvedLevel}`}
    >
      <Icon size={size === 'hero' ? 15 : size === 'compact' ? 11 : 13} strokeWidth={2.5} />
      <span>Lv.{resolvedLevel}</span>
    </span>
  );
}

function levelIcon(level: number) {
  if (level >= 80) return Gem;
  if (level >= 50) return Crown;
  if (level >= 20) return Sparkles;
  return ShieldCheck;
}
