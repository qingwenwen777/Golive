import type { LucideIcon } from 'lucide-react';
import {
  Menu,
  Search,
  Mic,
  Plus,
  Bell,
  House,
  Library,
  PlaySquare,
  Radio,
  History,
  Clock,
  Heart,
  ChevronRight,
  MoreVertical,
  BadgeCheck,
  Eye,
  Sun,
  Moon,
  Wallet,
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
  History: LucideIcon;
  Clock: LucideIcon;
  Heart: LucideIcon;
  ChevronRight: LucideIcon;
  More: LucideIcon;
  BadgeCheck: LucideIcon;
  Eye: LucideIcon;
  Sun: LucideIcon;
  Moon: LucideIcon;
  Wallet: LucideIcon;
}

export const Icons: IconSet = {
  Menu,
  Search,
  Mic,
  Plus,
  Bell,
  Home: House,
  Library,
  Subs: PlaySquare,
  Live: Radio,
  History,
  Clock,
  Heart,
  ChevronRight,
  More: MoreVertical,
  BadgeCheck,
  Eye,
  Sun,
  Moon,
  Wallet,
};
