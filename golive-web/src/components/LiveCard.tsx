import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Flag } from 'lucide-react';
import { Icons } from '@/components/Icons';
import { Avatar } from '@/components/Avatar';
import { LiveBadge } from '@/components/LiveBadge';
import { LoadableImage } from '@/components/LoadableImage';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ReportDialog, type ReportTargetDraft } from '@/features/reporting/ReportDialog';
import { copyText } from '@/lib/clipboard';
import { cn } from '@/lib/cn';
import { isInLibrary, removeFromLibrary, saveToLibrary, WATCH_LATER_KEY } from '@/lib/liveLibrary';
import { useCoverHoverStyle } from '@/hooks/useCoverHoverStyle';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useIsAuthed } from '@/stores/useAuthStore';
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
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);

  const title = lang === 'ja' ? (stream.titleJa ?? stream.title) : stream.title;
  const category = lang === 'ja' ? (stream.categoryJa ?? stream.category) : stream.category;
  const channelName = streamChannelName(stream);
  const isLive = stream.isLive === true || stream.status === 'live';
  const hasReplay = stream.status === 'ended' && Boolean(stream.replay?.canWatch);
  const isScheduled = stream.status === 'scheduled' || stream.status === 'publishing';
  const canOpen = isLive || hasReplay || isScheduled;
  const [saved, setSaved] = useState(() => isInLibrary(WATCH_LATER_KEY, stream.id));
  const [reportTarget, setReportTarget] = useState<ReportTargetDraft | null>(null);
  const hoverStyle = useCoverHoverStyle(stream.cover, channelName || title || stream.id);

  useEffect(() => {
    setSaved(isInLibrary(WATCH_LATER_KEY, stream.id));
  }, [stream.id]);

  const handleOpen = () => {
    if (!canOpen) return;
    if (onClick) onClick(stream);
    else navigate(`/live/${encodeURIComponent(stream.id)}`);
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
      const method = await copyText(
        href,
        t('liveRoom.copyTarget', { defaultValue: 'live room link' }),
      );
      if (method === 'manual') {
        toast.info(
          t('liveRoom.copyManual', { defaultValue: 'Live room link opened for manual copy.' }),
        );
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

  const openReport = () => {
    const target: ReportTargetDraft = {
      targetType: 'room',
      targetId: stream.id,
      targetUrl: `${window.location.origin}/live/${stream.id}`,
      roomId: stream.id,
      channelId: stream.channelId || stream.channel,
      targetOwnerId: stream.ownerId,
      targetOwnerName: channelName,
      targetTitle: title,
      targetText: stream.description,
    };
    if (!isAuthed) {
      openLogin(() => setReportTarget(target));
      return;
    }
    setReportTarget(target);
  };

  return (
    <>
      <div
        className={cn(
          'gl-card gl-video-hover-card',
          !canOpen && 'is-ended',
          hasReplay && 'is-replay',
        )}
        style={hoverStyle}
        tabIndex={canOpen ? 0 : -1}
        role={canOpen ? 'link' : 'article'}
        aria-disabled={!canOpen}
        onClick={handleOpen}
        onKeyDown={(e) => {
          if (canOpen && e.key === 'Enter') handleOpen();
        }}
      >
        <div
          className={`gl-card-cover relative overflow-hidden rounded-card${stream.cover ? 'has-image' : ''}`}
        >
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
            <LoadableImage
              src={stream.cover}
              alt=""
              loading={priority ? 'eager' : 'lazy'}
              decoding="async"
              className="absolute inset-0 h-full w-full object-cover"
            />
          ) : null}
          <div className="gl-card-gloss" />
          <div className="gl-card-top">
            {isLive ? (
              <LiveBadge />
            ) : (
              <span className="gl-dur-pill">
                {hasReplay
                  ? t('liveRoom.replay.badge', { defaultValue: 'Replay' })
                  : isScheduled
                    ? t('liveRoom.appointmentStartingSoon', { defaultValue: 'Starting soon' })
                    : t('library.status.offline', { defaultValue: 'Offline' })}
              </span>
            )}
          </div>
        </div>
        <div className="gl-card-meta">
          <Avatar name={channelName} src={stream.avatar} size={44} />
          <div className="gl-card-text">
            <div className="gl-card-title" title={title}>
              {title}
            </div>
            <div className="gl-card-chan">
              <span className="truncate">{channelName}</span>
              {stream.verified && (
                <Icons.BadgeCheck size={14} className="shrink-0 text-text-secondary" />
              )}
            </div>
            <div className="gl-card-sub">
              <span>
                {isLive
                  ? t('home.watching', {
                      count: stream.viewers,
                      defaultValue: '{{count}} watching',
                    })
                  : hasReplay
                    ? t('liveRoom.replay.badge', { defaultValue: 'Replay' })
                    : isScheduled
                      ? t('liveRoom.appointmentWaiting', {
                          count: stream.viewers,
                          defaultValue: '{{count}} waiting',
                        })
                      : t('library.status.offline', { defaultValue: 'Offline' })}
              </span>
              <span>·</span>
              <span className="truncate">{category}</span>
            </div>
          </div>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                className="gl-icon-btn gl-card-more"
                aria-label={t('liveCard.menu.more', { defaultValue: 'More actions' })}
                onClick={(e) => e.stopPropagation()}
              >
                <Icons.More size={18} />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-48" onClick={(e) => e.stopPropagation()}>
              <DropdownMenuItem onSelect={toggleWatchLater}>
                {saved
                  ? t('liveCard.menu.removeWatchLater', { defaultValue: 'Remove from Watch later' })
                  : t('liveCard.menu.saveWatchLater', { defaultValue: 'Save to Watch later' })}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={copyLink}>
                {t('liveCard.menu.copyLiveLink', { defaultValue: 'Copy live link' })}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem onSelect={openChannel}>
                {t('liveCard.menu.openChannel', { defaultValue: 'Open channel' })}
              </DropdownMenuItem>
              <DropdownMenuItem className="gl-menu-danger" onSelect={openReport}>
                <Flag size={15} />
                {t('liveCard.menu.report', { defaultValue: 'Report' })}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
      <ReportDialog
        open={Boolean(reportTarget)}
        target={reportTarget}
        onOpenChange={(open) => {
          if (!open) setReportTarget(null);
        }}
      />
    </>
  );
}
