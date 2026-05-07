/* eslint-disable react-refresh/only-export-components */
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ImgHTMLAttributes,
} from 'react';
import { cn } from '@/lib/cn';

type ImageStatus = 'idle' | 'loading' | 'loaded' | 'error';
type PreferredImageFormat = 'avif' | 'webp';

const READY_CACHE_LIMIT = 600;
const readyImageSrcs = new Set<string>();
const optimizedImageSrcs = new Map<string, string>();
const missingOptimizedImageSrcs = new Set<string>();
const missingOptimizedFormatScopes = new Set<string>();
const generatedPlaceholderStyles = new Map<string, CSSProperties>();
const DEFAULT_LAZY_ROOT_MARGIN = '600px 0px';
const DEFAULT_LAZY_THRESHOLD = 0.01;
const DEFAULT_FORMATS: readonly PreferredImageFormat[] = ['avif', 'webp'];

export interface LoadableImageProps extends ImgHTMLAttributes<HTMLImageElement> {
  src?: string;
  autoFormat?: boolean;
  lazyRootMargin?: string;
  lazyThreshold?: number | number[];
  onReady?: () => void;
  placeholderSrc?: string;
  preferredFormats?: readonly PreferredImageFormat[];
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

function thresholdKey(threshold: number | number[]) {
  return Array.isArray(threshold) ? threshold.join(',') : String(threshold);
}

function cssUrl(value: string) {
  return `url("${value.replace(/"/g, '\\"')}")`;
}

function imageHash(value: string) {
  let hash = 2166136261;
  for (let i = 0; i < value.length; i += 1) {
    hash ^= value.charCodeAt(i);
    hash = Math.imul(hash, 16777619);
  }
  return hash >>> 0;
}

function hsl(hash: number, offset: number, saturationBase = 48, lightnessBase = 42) {
  const hue = (hash + offset) % 360;
  const saturation = saturationBase + ((hash >>> 8) % 18);
  const lightness = lightnessBase + ((hash >>> 16) % 16);
  return `hsl(${hue} ${saturation}% ${lightness}%)`;
}

export function generatedImagePlaceholderStyle(src: string): CSSProperties {
  const cached = generatedPlaceholderStyles.get(src);
  if (cached) return cached;

  const hash = imageHash(src);
  const x1 = 18 + (hash % 56);
  const y1 = 12 + ((hash >>> 5) % 58);
  const x2 = 24 + ((hash >>> 11) % 52);
  const y2 = 28 + ((hash >>> 17) % 48);
  const base = hsl(hash, 0, 38, 36);
  const accent = hsl(hash, 72 + ((hash >>> 3) % 76), 52, 48);
  const depth = hsl(hash, 196 + ((hash >>> 9) % 62), 42, 32);
  const glow = hsl(hash, 124 + ((hash >>> 15) % 48), 58, 56);

  const style: CSSProperties = {
    backgroundColor: base,
    backgroundImage: [
      `radial-gradient(circle at ${x1}% ${y1}%, ${glow} 0%, transparent 38%)`,
      `radial-gradient(circle at ${x2}% ${y2}%, ${accent} 0%, transparent 44%)`,
      `linear-gradient(135deg, ${base} 0%, ${depth} 100%)`,
    ].join(', '),
    backgroundPosition: 'center',
    backgroundRepeat: 'no-repeat',
    backgroundSize: '150% 150%, 135% 135%, cover',
  };

  if (generatedPlaceholderStyles.size >= READY_CACHE_LIMIT) {
    const oldest = generatedPlaceholderStyles.keys().next().value;
    if (oldest) generatedPlaceholderStyles.delete(oldest);
  }
  generatedPlaceholderStyles.set(src, style);
  return style;
}

function canProbeFormatCandidates(src: string) {
  if (typeof navigator !== 'undefined' && /jsdom/i.test(navigator.userAgent)) return false;
  if (/^(data|blob):/i.test(src) || /\.(?:svg|gif|webp|avif)(?:[?#]|$)/i.test(src)) return false;
  if (!/\.(?:jpe?g|png)(?:[?#]|$)/i.test(src)) return false;
  if (!/^https?:\/\//i.test(src)) return true;
  if (typeof window === 'undefined') return false;
  try {
    return new URL(src, window.location.href).origin === window.location.origin;
  } catch {
    return false;
  }
}

function formatCandidate(src: string, format: PreferredImageFormat) {
  return src.replace(/\.(jpe?g|png)(?=([?#]|$))/i, `.${format}`);
}

function formatCandidates(src: string, formats: readonly PreferredImageFormat[]) {
  if (!canProbeFormatCandidates(src)) return [];
  return formats
    .map((format) => formatCandidate(src, format))
    .filter(
      (candidate) =>
        candidate !== src &&
        !missingOptimizedImageSrcs.has(candidate) &&
        !missingOptimizedFormatScopes.has(formatScope(candidate)),
    );
}

function formatScope(src: string) {
  try {
    const url =
      typeof window === 'undefined'
        ? new URL(src, 'http://localhost')
        : new URL(src, window.location.href);
    const directory = url.pathname.slice(0, url.pathname.lastIndexOf('/') + 1);
    const format = url.pathname.match(/\.(avif|webp)$/i)?.[1]?.toLowerCase() ?? '';
    return `${url.origin}${directory}|${format}`;
  } catch {
    const directory = src.slice(0, src.lastIndexOf('/') + 1);
    const format = src.match(/\.(avif|webp)(?:[?#]|$)/i)?.[1]?.toLowerCase() ?? '';
    return `${directory}|${format}`;
  }
}

function rememberOptimizedImage(original: string, optimized: string) {
  if (optimizedImageSrcs.size >= READY_CACHE_LIMIT) {
    const oldest = optimizedImageSrcs.keys().next().value;
    if (oldest) optimizedImageSrcs.delete(oldest);
  }
  optimizedImageSrcs.set(original, optimized);
}

export function LoadableImage({
  src,
  className,
  autoFormat = true,
  lazyRootMargin = DEFAULT_LAZY_ROOT_MARGIN,
  lazyThreshold = DEFAULT_LAZY_THRESHOLD,
  onLoad,
  onError,
  onReady,
  placeholderSrc,
  preferredFormats = DEFAULT_FORMATS,
  decoding = 'async',
  style,
  srcSet,
  ...props
}: LoadableImageProps) {
  const imageRef = useRef<HTMLImageElement | null>(null);
  const notifiedReadySrc = useRef<string | null>(null);
  const [status, setStatus] = useState<ImageStatus>(() => initialStatus(src));
  const [placeholderFailed, setPlaceholderFailed] = useState(false);
  const [resolvedSrc, setResolvedSrc] = useState<string | undefined>(() =>
    src ? (optimizedImageSrcs.get(src) ?? src) : undefined,
  );
  const [shouldLoad, setShouldLoad] = useState(() => {
    if (!src) return false;
    if (isLoadableImageReady(src)) return true;
    if (props.loading !== 'lazy') return true;
    if (typeof window === 'undefined') return true;
    return !('IntersectionObserver' in window);
  });
  const lazyThresholdKey = thresholdKey(lazyThreshold);
  const formatKey = preferredFormats.join(',');
  const hasUsablePlaceholderSrc = Boolean(placeholderSrc && !placeholderFailed);
  const generatedPlaceholderStyle = useMemo(
    () => (src && !hasUsablePlaceholderSrc ? generatedImagePlaceholderStyle(src) : undefined),
    [hasUsablePlaceholderSrc, src],
  );
  const effectiveResolvedSrc = useMemo(() => {
    if (!src) return undefined;
    if (!resolvedSrc) return src;
    if (resolvedSrc === src || optimizedImageSrcs.get(src) === resolvedSrc) return resolvedSrc;
    return src;
  }, [resolvedSrc, src]);

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

  useEffect(() => {
    setPlaceholderFailed(false);
  }, [placeholderSrc, src]);

  useEffect(() => {
    if (!src) {
      setResolvedSrc(undefined);
      return;
    }
    if (!autoFormat || srcSet) {
      setResolvedSrc(src);
      return;
    }
    const cached = optimizedImageSrcs.get(src);
    if (cached) {
      setResolvedSrc(cached);
      return;
    }
    const candidates = formatCandidates(src, preferredFormats);
    if (candidates.length === 0) {
      setResolvedSrc(src);
      return;
    }

    let cancelled = false;
    const probe = (index: number) => {
      if (cancelled) return;
      const candidate = candidates[index];
      if (!candidate) {
        setResolvedSrc(src);
        return;
      }
      const image = new Image();
      image.decoding = 'async';
      image.onload = () => {
        if (cancelled) return;
        rememberOptimizedImage(src, candidate);
        setResolvedSrc(candidate);
      };
      image.onerror = () => {
        missingOptimizedImageSrcs.add(candidate);
        missingOptimizedFormatScopes.add(formatScope(candidate));
        probe(index + 1);
      };
      image.src = candidate;
    };
    setResolvedSrc(undefined);
    probe(0);
    return () => {
      cancelled = true;
    };
  }, [autoFormat, formatKey, preferredFormats, src, srcSet]);

  useEffect(() => {
    if (!src) {
      setShouldLoad(false);
      return;
    }
    if (isLoadableImageReady(src) || props.loading !== 'lazy') {
      setShouldLoad(true);
      return;
    }
    if (typeof window === 'undefined' || !('IntersectionObserver' in window)) {
      setShouldLoad(true);
      return;
    }
    setShouldLoad(false);
  }, [props.loading, src]);

  useEffect(() => {
    if (!src || shouldLoad) return;
    const node = imageRef.current;
    if (!node || typeof window === 'undefined' || !('IntersectionObserver' in window)) {
      setShouldLoad(true);
      return;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting || entry.intersectionRatio > 0)) {
          setShouldLoad(true);
          observer.disconnect();
        }
      },
      { rootMargin: lazyRootMargin, threshold: lazyThreshold },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [lazyRootMargin, lazyThreshold, lazyThresholdKey, shouldLoad, src]);

  const displaySrc = shouldLoad
    ? effectiveResolvedSrc
    : hasUsablePlaceholderSrc
      ? placeholderSrc
      : undefined;
  const isDisplayingPlaceholderSrc = Boolean(
    displaySrc && placeholderSrc && displaySrc === placeholderSrc && displaySrc !== resolvedSrc,
  );
  const imageStyle = useMemo<CSSProperties | undefined>(() => {
    if (status === 'loaded') return style;
    if (hasUsablePlaceholderSrc && placeholderSrc) {
      return {
        backgroundImage: cssUrl(placeholderSrc),
        backgroundPosition: 'center',
        backgroundSize: 'cover',
        ...style,
      };
    }
    if (generatedPlaceholderStyle) {
      return {
        ...generatedPlaceholderStyle,
        ...style,
      };
    }
    return {
      ...style,
    };
  }, [generatedPlaceholderStyle, hasUsablePlaceholderSrc, placeholderSrc, status, style]);

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
    if (displaySrc && !isDisplayingPlaceholderSrc && image?.complete && image.naturalWidth > 0) {
      markReady(src);
    }
  }, [displaySrc, isDisplayingPlaceholderSrc, markReady, src]);

  if (!src) return null;

  return (
    <img
      {...props}
      ref={imageRef}
      decoding={decoding}
      src={displaySrc}
      srcSet={displaySrc && !isDisplayingPlaceholderSrc ? srcSet : undefined}
      style={imageStyle}
      className={cn(
        'gl-loadable-img',
        status === 'loading' && 'is-loading',
        status === 'loaded' && 'is-loaded',
        status === 'error' && 'is-error',
        hasUsablePlaceholderSrc && status !== 'loaded' && 'has-placeholder',
        !hasUsablePlaceholderSrc &&
          generatedPlaceholderStyle &&
          status !== 'loaded' &&
          'has-generated-placeholder',
        props.loading === 'lazy' && !shouldLoad && 'is-lazy-pending',
        className,
      )}
      onLoad={(event) => {
        if (isDisplayingPlaceholderSrc) {
          return;
        }
        if (displaySrc) {
          markReady(src);
        }
        onLoad?.(event);
      }}
      onError={(event) => {
        if (isDisplayingPlaceholderSrc) {
          setPlaceholderFailed(true);
          setStatus('loading');
          return;
        }
        notifiedReadySrc.current = null;
        setStatus('error');
        onError?.(event);
      }}
    />
  );
}
