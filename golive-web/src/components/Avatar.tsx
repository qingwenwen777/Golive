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
      ) : null}
    </div>
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
