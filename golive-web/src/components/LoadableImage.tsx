/* eslint-disable react-refresh/only-export-components */
import { useCallback, useLayoutEffect, useRef, useState, type ImgHTMLAttributes } from 'react';
import { cn } from '@/lib/cn';

type ImageStatus = 'idle' | 'loading' | 'loaded' | 'error';
const READY_CACHE_LIMIT = 600;
const readyImageSrcs = new Set<string>();

export interface LoadableImageProps extends ImgHTMLAttributes<HTMLImageElement> {
  src?: string;
  onReady?: () => void;
}

export function isLoadableImageReady(src: string | undefined): boolean {
  return Boolean(src && readyImageSrcs.has(src));
}

function rememberReadyImage(src: string) {
  if (readyImageSrcs.has(src)) return;
  if (readyImageSrcs.size >= READY_CACHE_LIMIT) {
    const oldest = readyImageSrcs.values().next().value;
    if (oldest) readyImageSrcs.delete(oldest);
  }
  readyImageSrcs.add(src);
}

function initialStatus(src: string | undefined): ImageStatus {
  if (!src) return 'idle';
  return isLoadableImageReady(src) ? 'loaded' : 'loading';
}

export function LoadableImage({
  src,
  className,
  onLoad,
  onError,
  onReady,
  ...props
}: LoadableImageProps) {
  const imageRef = useRef<HTMLImageElement | null>(null);
  const notifiedReadySrc = useRef<string | null>(null);
  const [status, setStatus] = useState<ImageStatus>(() => initialStatus(src));

  const markReady = useCallback(
    (readySrc: string) => {
      rememberReadyImage(readySrc);
      setStatus('loaded');
      if (notifiedReadySrc.current !== readySrc) {
        notifiedReadySrc.current = readySrc;
        onReady?.();
      }
    },
    [onReady],
  );

  useLayoutEffect(() => {
    if (!src) {
      notifiedReadySrc.current = null;
      setStatus('idle');
      return;
    }

    if (isLoadableImageReady(src)) {
      markReady(src);
      return;
    }

    notifiedReadySrc.current = null;
    setStatus('loading');

    const image = imageRef.current;
    if (image?.complete && image.naturalWidth > 0) {
      markReady(src);
    }
  }, [markReady, src]);

  if (!src) return null;

  return (
    <img
      {...props}
      ref={imageRef}
      src={src}
      className={cn(
        'gl-loadable-img',
        status === 'loaded' && 'is-loaded',
        status === 'error' && 'is-error',
        className,
      )}
      onLoad={(event) => {
        markReady(src);
        onLoad?.(event);
      }}
      onError={(event) => {
        notifiedReadySrc.current = null;
        setStatus('error');
        onError?.(event);
      }}
    />
  );
}
