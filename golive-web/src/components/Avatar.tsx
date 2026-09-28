import { useLayoutEffect, useState, type CSSProperties } from 'react';
import { cn } from '@/lib/cn';
import { isLoadableImageReady, LoadableImage } from '@/components/LoadableImage';
import { avatarColor, avatarInitial } from '@/lib/avatar';

export interface AvatarProps {
  name: string;
  src?: string;
  size?: number;
  ring?: string;
  className?: string;
}

// Without a photo, an avatar is the name's first letter on a colour picked
// from the name, drawn here rather than fetched from an avatar service.
export function Avatar({ name, src, size = 36, ring, className }: AvatarProps) {
  const imageSrc = normalizeAvatarSrc(src);
  const [failed, setFailed] = useState(false);
  const [loaded, setLoaded] = useState(() => isLoadableImageReady(imageSrc));
  const showImage = Boolean(imageSrc) && !failed;
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
    fontSize: Math.round(size * 0.42),
    lineHeight: 1,
    letterSpacing: 0,
    background: showImage ? 'var(--gl-avatar-placeholder)' : avatarColor(name),
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
        !showImage && 'is-initial',
        className,
      )}
      style={style}
      role="img"
      aria-label={name}
    >
      {showImage ? (
        <LoadableImage
          src={imageSrc}
          alt=""
          autoFormat={false}
          className="gl-avatar-img"
          onReady={() => setLoaded(true)}
          onError={() => setFailed(true)}
        />
      ) : (
        <span aria-hidden="true">{avatarInitial(name)}</span>
      )}
    </div>
  );
}

// Avatars that older accounts and rooms got from DiceBear are generated
// defaults, not pictures anyone chose; draw the initial instead.
const GENERATED_AVATAR = /^https?:\/\/api\.dicebear\.com\//i;

function normalizeAvatarSrc(src: string | undefined): string {
  const value = src?.trim() ?? '';
  if (!value || GENERATED_AVATAR.test(value)) return '';
  if (/^(https?:)?\/\//i.test(value) || value.startsWith('data:') || value.startsWith('blob:')) {
    return value;
  }
  if (value.startsWith('/')) return value;
  return `/${value.replace(/^\/+/, '')}`;
}
