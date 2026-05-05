import { cn } from '@/lib/cn';

export interface SkeletonProps {
  className?: string;
  style?: React.CSSProperties;
}

export function Skeleton({ className, style }: SkeletonProps) {
  return <div className={cn('gl-skel-block', className)} style={style} />;
}

export function LiveCardSkeleton() {
  return (
    <div className="gl-skel">
      <div className="gl-skel-cover" />
      <div className="gl-skel-meta">
        <div className="gl-skel-avatar" />
        <div className="flex-1">
          <div className="gl-skel-line is-title" style={{ width: '85%' }} />
          <div className="gl-skel-line" style={{ width: '55%', marginTop: 8 }} />
        </div>
      </div>
    </div>
  );
}
