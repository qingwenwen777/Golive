import { useLayoutEffect, useState, type CSSProperties } from 'react';
import { cn } from '@/lib/cn';
import { isLoadableImageReady, LoadableImage } from '@/components/LoadableImage';

export interface AvatarProps {
  name: string;
  src?: string;
  size?: number;
  ring?: string;
  className?: string;
}

export function Avatar({ name, src, size = 36, ring, className }: AvatarProps) {
  const [failed, setFailed] = useState(false);
  const imageSrc = normalizeAvatarSrc(src);
  const [loaded, setLoaded] = useState(() => isLoadableImageReady(imageSrc));
  const showImage = Boolean(imageSrc && !failed);
  const fallbackLabel = avatarFallbackLabel(name);
  const fallbackColor = avatarFallbackColor(name);
  const style: CSSProperties = {
    width: size,
    height: size,
    borderRadius: '50%',
    display: 'inline-flex',
    position: 'relative',
    alignItems: 'center',
    justifyContent: 'center',
    overflow: 'hidden',
    color: 'white',
    fontWeight: 600,
    fontSize: size * 0.42,
    letterSpacing: 0,
    background: showImage ? 'var(--gl-avatar-placeholder)' : fallbackColor,
    flexShrink: 0,
    ...(ring ? { boxShadow: `0 0 0 2px ${ring}, 0 0 0 4px var(--gl-avatar-placeholder)` } : null),
  };

  useLayoutEffect(() => {
    setFailed(false);
    setLoaded(isLoadableImageReady(imageSrc));
  }, [imageSrc]);

  return (
    <div
      className={cn(
        'gl-avatar',
        showImage && !loaded && 'is-loading',
        !showImage && 'is-empty',
        className,
      )}
      style={style}
      aria-label={name}
    >
      {showImage ? (
        <LoadableImage
          src={imageSrc}
          alt=""
          className="gl-avatar-img"
          onReady={() => setLoaded(true)}
          onError={() => setFailed(true)}
        />
      ) : (
        <span className="gl-avatar-fallback" aria-hidden="true">
          {fallbackLabel}
        </span>
      )}
    </div>
  );
}

const FALLBACK_COLORS = [
  '#2563eb',
  '#0f766e',
  '#c2410c',
  '#7c3aed',
  '#be123c',
  '#047857',
  '#b45309',
  '#0369a1',
];

function avatarFallbackLabel(name: string): string {
  const clean = name.trim();
  if (!clean) return '?';

  const words = clean.split(/[\s._@-]+/).filter(Boolean);
  const initials = words
    .map((word) => word.match(/[a-z0-9]/i)?.[0] ?? '')
    .filter(Boolean)
    .slice(0, 2)
    .join('');
  if (initials) return initials.toUpperCase();

  return Array.from(clean.replace(/\s+/g, ''))[0]?.toUpperCase() ?? '?';
}

function avatarFallbackColor(name: string): string {
  const seed = name.trim() || 'golive';
  let hash = 0;
  for (const char of seed) {
    hash = (hash * 31 + char.charCodeAt(0)) | 0;
  }
  return FALLBACK_COLORS[Math.abs(hash) % FALLBACK_COLORS.length];
}

function normalizeAvatarSrc(src: string | undefined): string {
  if (!src) return '';
  if (/^(https?:)?\/\//i.test(src) || src.startsWith('data:') || src.startsWith('blob:')) {
    return src;
  }
  if (src.startsWith('/')) return src;
  return `/${src.replace(/^\/+/, '')}`;
}
