import { forwardRef } from 'react';
import type { LucideIcon, LucideProps } from 'lucide-react';
import {
  Menu,
  Search,
  Mic,
  Plus,
  Bell,
  House,
  Library,
  PlaySquare,
  FileText,
  Radio,
  History,
  Clock,
  CalendarClock,
  Heart,
  ChevronRight,
  MoreVertical,
  BadgeCheck,
  Eye,
  Sun,
  Moon,
  Wallet,
  ShieldCheck,
} from 'lucide-react';

export interface IconSet {
  Menu: LucideIcon;
  Search: LucideIcon;
  Mic: LucideIcon;
  Plus: LucideIcon;
  Bell: LucideIcon;
  Home: LucideIcon;
  Library: LucideIcon;
  Subs: LucideIcon;
  Live: LucideIcon;
  FileText: LucideIcon;
  History: LucideIcon;
  Clock: LucideIcon;
  CalendarClock: LucideIcon;
  Heart: LucideIcon;
  ChevronRight: LucideIcon;
  More: LucideIcon;
  BadgeCheck: LucideIcon;
  Eye: LucideIcon;
  Sun: LucideIcon;
  Moon: LucideIcon;
  Wallet: LucideIcon;
  ShieldCheck: LucideIcon;
  WatchLater: LucideIcon;
}

const WatchLater = forwardRef<SVGSVGElement, LucideProps>(
  (
    {
      color = 'currentColor',
      size = 24,
      strokeWidth = 2,
      absoluteStrokeWidth,
      ...props
    },
    ref,
  ) => {
    const computedStrokeWidth =
      absoluteStrokeWidth && typeof size === 'number'
        ? (Number(strokeWidth) * 24) / size
        : strokeWidth;

    return (
      <svg
        ref={ref}
        xmlns="http://www.w3.org/2000/svg"
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="none"
        stroke={color}
        strokeWidth={computedStrokeWidth}
        strokeLinecap="round"
        strokeLinejoin="round"
        {...props}
      >
        <path d="M16 6H3" />
        <path d="M11 12H3" />
        <path d="M16 18H3" />
      </svg>
    );
  },
);
WatchLater.displayName = 'WatchLater';

export const Icons: IconSet = {
  Menu,
  Search,
  Mic,
  Plus,
  Bell,
  Home: House,
  Library,
  Subs: PlaySquare,
  FileText,
  Live: Radio,
  History,
  Clock,
  CalendarClock,
  Heart,
  ChevronRight,
  More: MoreVertical,
  BadgeCheck,
  Eye,
  Sun,
  Moon,
  Wallet,
  ShieldCheck,
  WatchLater,
};
