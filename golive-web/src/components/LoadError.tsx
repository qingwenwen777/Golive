import type { ReactNode } from 'react';
import { CloudOff } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/cn';
import { httpStatus } from '@/lib/httpError';

interface LoadErrorProps {
  /** What failed. Sections default to "Couldn't load this section". */
  title?: string;
  error?: unknown;
  onRetry: () => unknown;
  /** True while the retry is in flight (pass the query's isFetching). */
  retrying?: boolean;
  /** 'page' replaces the whole page; 'section' sits under a section heading. */
  variant?: 'page' | 'section';
  className?: string;
  /** Extra actions shown after Retry, such as a link home. */
  children?: ReactNode;
}

// LoadError is what a page or section shows when its request failed, so a
// failure never passes for "nothing here yet" or "this live has ended".
export function LoadError({
  title,
  error,
  onRetry,
  retrying = false,
  variant = 'section',
  className,
  children,
}: LoadErrorProps) {
  const { t } = useTranslation('common');
  const page = variant === 'page';
  const hint =
    httpStatus(error) === undefined
      ? t('loadError.networkHint', { defaultValue: 'Check your connection and try again.' })
      : t('loadError.serverHint', {
          defaultValue: 'Something went wrong on our end. Try again in a moment.',
        });

  return (
    <div
      className={cn(page ? 'gl-error' : 'gl-load-error', className)}
      role={page ? 'alert' : undefined}
    >
      <CloudOff size={page ? 64 : 28} strokeWidth={1.5} aria-hidden />
      <div className="gl-load-error-text">
        <div className="gl-empty-title">
          {title ?? t('loadError.title', { defaultValue: "Couldn't load this section" })}
        </div>
        <div className="gl-empty-sub">{hint}</div>
      </div>
      <div className="gl-load-error-actions">
        {/* aria-disabled rather than disabled, so keyboard focus stays put. */}
        <button
          type="button"
          className="gl-retry-btn"
          aria-disabled={retrying || undefined}
          onClick={retrying ? undefined : () => void onRetry()}
        >
          {retrying
            ? t('loadError.retrying', { defaultValue: 'Retrying…' })
            : t('loadError.retry', { defaultValue: 'Retry' })}
        </button>
        {children}
      </div>
    </div>
  );
}
