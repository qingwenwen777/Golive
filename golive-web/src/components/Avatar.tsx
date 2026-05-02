import { useEffect, useState, type CSSProperties } from 'react';
import { cn } from '@/lib/cn';
import { LoadableImage } from '@/components/LoadableImage';

export interface AvatarProps {
  name: string;
  src?: string;
  size?: number;
  ring?: string;
  className?: string;
}

function hashHue(name: string): number {
  let x = 0;
  for (let i = 0; i < name.length; i++) {
    x = (x * 31 + name.charCodeAt(i)) & 0xffff;
  }
  return x % 360;
}

function computeInitials(name: string): string {
  return name
    .replace(/[^a-zA-Z぀-ヿ一-鿿 ]/g, '')
    .split(/\s+/)
    .map((w) => w[0])
    .filter((c): c is string => Boolean(c))
    .slice(0, 2)
    .join('')
    .toUpperCase();
}

export function Avatar({ name, src, size = 36, ring, className }: AvatarProps) {
  const [failed, setFailed] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const hue = hashHue(name);
  const initials = computeInitials(name);
  const bg1 = `hsl(${hue}, 65%, 55%)`;
  const bg2 = `hsl(${(hue + 40) % 360}, 70%, 45%)`;
  const imageSrc = normalizeAvatarSrc(src);
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
    background: `linear-gradient(135deg, ${bg1}, ${bg2})`,
    flexShrink: 0,
    ...(ring ? { boxShadow: `0 0 0 2px ${ring}, 0 0 0 4px ${bg1}` } : null),
  };

  useEffect(() => {
    setFailed(false);
    setLoaded(false);
  }, [imageSrc]);

  return (
    <div
      className={cn('gl-avatar', imageSrc && !failed && !loaded && 'is-loading', className)}
      style={style}
      aria-label={name}
    >
      {imageSrc && !failed ? (
        <LoadableImage
          src={imageSrc}
          alt=""
          className="gl-avatar-img"
          onLoad={() => setLoaded(true)}
          onError={() => setFailed(true)}
        />
      ) : (
        initials
      )}
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
