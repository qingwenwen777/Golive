import { useEffect, useState, type ImgHTMLAttributes } from 'react';
import { cn } from '@/lib/cn';

type ImageStatus = 'idle' | 'loading' | 'loaded' | 'error';

export interface LoadableImageProps extends ImgHTMLAttributes<HTMLImageElement> {
  src?: string;
}

export function LoadableImage({
  src,
  className,
  onLoad,
  onError,
  ...props
}: LoadableImageProps) {
  const [status, setStatus] = useState<ImageStatus>(() => (src ? 'loading' : 'idle'));

  useEffect(() => {
    setStatus(src ? 'loading' : 'idle');
  }, [src]);

  if (!src) return null;

  return (
    <img
      {...props}
      src={src}
      className={cn(
        'gl-loadable-img',
        status === 'loaded' && 'is-loaded',
        status === 'error' && 'is-error',
        className,
      )}
      onLoad={(event) => {
        setStatus('loaded');
        onLoad?.(event);
      }}
      onError={(event) => {
        setStatus('error');
        onError?.(event);
      }}
    />
  );
}
