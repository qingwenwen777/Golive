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
  const primarySrc = normalizeAvatarSrc(src);
  const fallbackSrc = defaultAvatarSrc(name);
  const [primaryFailed, setPrimaryFailed] = useState(false);
  const [fallbackFailed, setFallbackFailed] = useState(false);
  const imageSrc = primarySrc && !primaryFailed ? primarySrc : fallbackSrc;
  const [loaded, setLoaded] = useState(() => isLoadableImageReady(imageSrc));
  const showImage = Boolean(imageSrc && !(imageSrc === fallbackSrc && fallbackFailed));
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
    background: 'var(--gl-avatar-placeholder)',
    flexShrink: 0,
    ...(ring ? { boxShadow: `0 0 0 2px ${ring}, 0 0 0 4px var(--gl-avatar-placeholder)` } : null),
  };

  useLayoutEffect(() => {
    setPrimaryFailed(false);
    setFallbackFailed(false);
  }, [fallbackSrc, primarySrc]);

  useLayoutEffect(() => {
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
          onError={() => {
            if (imageSrc === fallbackSrc) {
              setFallbackFailed(true);
            } else {
              setPrimaryFailed(true);
            }
          }}
        />
      ) : null}
    </div>
  );
}

const DEFAULT_AVATAR_BASE = 'https://api.dicebear.com/7.x/avataaars/svg?seed=';

function defaultAvatarSrc(name: string): string {
  const seed = name.trim() || 'golive';
  return DEFAULT_AVATAR_BASE + encodeURIComponent(seed);
}

function normalizeAvatarSrc(src: string | undefined): string {
  if (!src) return '';
  if (/^(https?:)?\/\//i.test(src) || src.startsWith('data:') || src.startsWith('blob:')) {
    return src;
  }
  if (src.startsWith('/')) return src;
  return `/${src.replace(/^\/+/, '')}`;
}
