import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Icons } from '@/components/Icons';
import { Avatar } from '@/components/Avatar';
import { LiveBadge } from '@/components/LiveBadge';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { copyText } from '@/lib/clipboard';
import { isInLibrary, removeFromLibrary, saveToLibrary, WATCH_LATER_KEY } from '@/lib/liveLibrary';
import { useLangStore } from '@/stores/useLangStore';
import { streamChannelName, type Stream } from '@/types/stream';

export interface LiveCardProps {
  stream: Stream;
  onClick?: (stream: Stream) => void;
  priority?: boolean;
}

function initialsOf(name: string): string {
  const trimmed = (name ?? '').trim();
  if (!trimmed) return '?';
  const parts = trimmed.split(/\s+/).slice(0, 2);
  return parts.map((p) => p[0]?.toUpperCase() ?? '').join('') || trimmed[0].toUpperCase();
}

function gradientFor(seed: string): string {
  let h = 0;
  for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) >>> 0;
  const a = h % 360;
  const b = (a + 60) % 360;
  return `linear-gradient(135deg, hsl(${a} 70% 45%), hsl(${b} 70% 35%))`;
}

export function LiveCard({ stream, onClick, priority }: LiveCardProps) {
  const navigate = useNavigate();
  const { t } = useTranslation('pages');
  const lang = useLangStore((s) => s.lang);

  const title = lang === 'ja' ? (stream.titleJa ?? stream.title) : stream.title;
  const category = lang === 'ja' ? (stream.categoryJa ?? stream.category) : stream.category;
  const channelName = streamChannelName(stream);
  const isLive = stream.isLive === true || stream.status === 'live';
  const [saved, setSaved] = useState(() => isInLibrary(WATCH_LATER_KEY, stream.id));

  useEffect(() => {
    setSaved(isInLibrary(WATCH_LATER_KEY, stream.id));
  }, [stream.id]);

  const handleOpen = () => {
    if (!isLive) return;
    if (onClick) onClick(stream);
    else navigate(`/live/${stream.id}`);
  };

  const toggleWatchLater = () => {
    if (saved) {
      removeFromLibrary(WATCH_LATER_KEY, stream.id);
      setSaved(false);
      toast.success(t('liveRoom.removedWatchLater', { defaultValue: 'Removed from Watch later.' }));
      return;
    }
    saveToLibrary(WATCH_LATER_KEY, stream);
    setSaved(true);
    toast.success(t('liveRoom.savedWatchLater', { defaultValue: 'Saved to Watch later.' }));
  };

  const copyLink = async () => {
    const href = `${window.location.origin}/live/${stream.id}`;
    try {
      const method = await copyText(href, t('liveRoom.copyTarget', { defaultValue: 'live room link' }));
      if (method === 'manual') {
        toast.info(t('liveRoom.copyManual', { defaultValue: 'Live room link opened for manual copy.' }));
      } else {
        toast.success(t('liveRoom.copySuccess', { defaultValue: 'Live room link copied.' }));
      }
    } catch {
      toast.error(t('liveRoom.copyFailed', { defaultValue: 'Could not copy the live room link.' }));
    }
  };

  const openChannel = () => {
    const key = stream.channelId || channelName || stream.channel;
    navigate(`/channel/${encodeURIComponent(key)}`);
  };

  return (
    <div
      className={`gl-card${isLive ? '' : ' is-ended'}`}
      tabIndex={isLive ? 0 : -1}
      role={isLive ? 'link' : 'article'}
      aria-disabled={!isLive}
      onClick={handleOpen}
      onKeyDown={(e) => {
        if (isLive && e.key === 'Enter') handleOpen();
      }}
    >
      <div className="gl-card-cover relative overflow-hidden rounded-card">
        <div
          aria-hidden
          className="absolute inset-0 flex items-center justify-center"
          style={{
            background: gradientFor(channelName || stream.title || stream.id),
          }}
        >
          <span className="text-3xl font-bold text-white/90 drop-shadow">
            {initialsOf(channelName || stream.title)}
          </span>
        </div>
        {stream.cover ? (
          <img
            src={stream.cover}
            alt=""
            loading={priority ? 'eager' : 'lazy'}
            decoding="async"
            className="absolute inset-0 h-full w-full object-cover"
            onError={(e) => {
              // Hide broken image and let the gradient placeholder show.
              (e.currentTarget as HTMLImageElement).style.display = 'none';
            }}
          />
        ) : null}
        <div className="gl-card-gloss" />
        <div className="gl-card-top">
          {isLive ? <LiveBadge /> : <span className="gl-dur-pill">Offline</span>}
        </div>
        <div className="gl-card-bottom">
          <span className="gl-view-pill">
            <Icons.Eye size={14} />
            <span>{stream.viewers.toLocaleString()}</span>
          </span>
          <span className="gl-dur-pill">{stream.duration}</span>
        </div>
        <div className="gl-card-hover-outline" />
      </div>
      <div className="gl-card-meta">
        <Avatar name={channelName} src={stream.avatar} size={36} />
        <div className="gl-card-text">
          <div className="gl-card-title">{title}</div>
          <div className="gl-card-chan">
            <span className="truncate">{channelName}</span>
            {stream.verified && (
              <Icons.BadgeCheck size={14} className="shrink-0 text-text-secondary" />
            )}
          </div>
          <div className="gl-card-sub">
            <span>
              {isLive
                ? t('home.watching', { count: stream.viewers, defaultValue: '{{count}} watching' })
                : 'Not live now'}
            </span>
            <span>·</span>
            <span className="truncate">{category}</span>
          </div>
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              className="gl-icon-btn gl-card-more"
              aria-label="More actions"
              onClick={(e) => e.stopPropagation()}
            >
              <Icons.More size={18} />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            align="end"
            className="w-48"
            onClick={(e) => e.stopPropagation()}
          >
            <DropdownMenuItem onSelect={toggleWatchLater}>
              {saved ? 'Remove from Watch later' : 'Save to Watch later'}
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={copyLink}>Copy live link</DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={openChannel}>Open channel</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  );
}
