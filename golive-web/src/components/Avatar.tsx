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
        <DefaultAvatarIcon size={size} />
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

function avatarFallbackColor(name: string): string {
  const seed = name.trim() || 'golive';
  let hash = 0;
  for (const char of seed) {
    hash = (hash * 31 + char.charCodeAt(0)) | 0;
  }
  return FALLBACK_COLORS[Math.abs(hash) % FALLBACK_COLORS.length];
}

function DefaultAvatarIcon({ size }: { size: number }) {
  const iconSize = Math.round(size * 0.74);
  return (
    <svg
      className="gl-avatar-fallback"
      width={iconSize}
      height={iconSize}
      viewBox="0 0 64 64"
      fill="none"
      aria-hidden="true"
      focusable="false"
    >
      <circle className="gl-avatar-fallback-head" cx="32" cy="23" r="12" />
      <path className="gl-avatar-fallback-body" d="M12 56c2.8-12.4 10.5-19 20-19s17.2 6.6 20 19" />
      <path
        className="gl-avatar-fallback-glow"
        d="M20 45c3.4-4 7.4-6 12-6 4.4 0 8.3 1.9 11.8 5.7"
      />
    </svg>
  );
}

function normalizeAvatarSrc(src: string | undefined): string {
  if (!src) return '';
  if (/^(https?:)?\/\//i.test(src) || src.startsWith('data:') || src.startsWith('blob:')) {
    return src;
  }
  if (src.startsWith('/')) return src;
  return `/${src.replace(/^\/+/, '')}`;
}
