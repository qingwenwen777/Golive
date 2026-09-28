import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Flag } from 'lucide-react';
import { Icons } from '@/components/Icons';
import { Avatar } from '@/components/Avatar';
import { FanClubExclusiveBadge } from '@/components/FanClubExclusiveBadge';
import { LiveBadge } from '@/components/LiveBadge';
import { LoadableImage } from '@/components/LoadableImage';
import { ShareDialog } from '@/components/ShareDialog';
import { VerifiedBadge } from '@/components/VerifiedBadge';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ReportDialog, type ReportTargetDraft } from '@/features/reporting/ReportDialog';
import { cn } from '@/lib/cn';
import { removeFromLibrary, saveToLibrary, WATCH_LATER_KEY } from '@/lib/liveLibrary';
import {
  useLibraryMembership,
  useRemoveUserLibraryItem,
  useSaveUserLibraryItem,
} from '@/api/library';
import { useCoverHoverStyle } from '@/hooks/useCoverHoverStyle';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useIsAuthed } from '@/stores/useAuthStore';
import { useLangStore } from '@/stores/useLangStore';
import { streamChannelName, type Stream } from '@/types/stream';
import { isPlaceholderName } from '@/types/user';

export interface LiveCardProps {
  stream: Stream;
  onClick?: (stream: Stream) => void;
  priority?: boolean;
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
  const hasChannelName = !isPlaceholderName(channelName);
  const isLive = stream.isLive === true || stream.status === 'live';
  const hasReplay = stream.status === 'ended' && Boolean(stream.replay?.canWatch);
  const isScheduled = stream.status === 'scheduled' || stream.status === 'publishing';
  const canOpen = isLive || hasReplay || isScheduled;
  const watchLaterMembership = useLibraryMembership(WATCH_LATER_KEY, stream.id, isAuthed);
  const saveWatchLater = useSaveUserLibraryItem(WATCH_LATER_KEY);
  const removeWatchLater = useRemoveUserLibraryItem(WATCH_LATER_KEY);
  const [reportTarget, setReportTarget] = useState<ReportTargetDraft | null>(null);
  const [shareOpen, setShareOpen] = useState(false);
  const hoverStyle = useCoverHoverStyle(
    stream.cover,
    (hasChannelName ? channelName : title) || stream.id,
  );

  const saved = watchLaterMembership.isMember;

  const handleOpen = () => {
    if (!canOpen) return;
    if (onClick) onClick(stream);
    else navigate(`/live/${encodeURIComponent(stream.id)}`);
  };

  const toggleWatchLater = () => {
    if (saved) {
      if (isAuthed) removeWatchLater.mutate(stream.id);
      else {
        removeFromLibrary(WATCH_LATER_KEY, stream.id);
        watchLaterMembership.setLocalMember(false);
      }
      toast.success(t('liveRoom.removedWatchLater', { defaultValue: 'Removed from Watch later.' }));
      return;
    }
    if (isAuthed) saveWatchLater.mutate({ stream });
    else {
      saveToLibrary(WATCH_LATER_KEY, stream);
      watchLaterMembership.setLocalMember(true);
    }
    toast.success(t('liveRoom.savedWatchLater', { defaultValue: 'Saved to Watch later.' }));
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
          className={cn(
            'gl-card-cover relative overflow-hidden rounded-card',
            stream.cover ? 'has-image' : 'is-placeholder',
          )}
        >
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
            <span className="gl-card-top-left">
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
              {stream.fanClubOnly && <FanClubExclusiveBadge compact />}
            </span>
          </div>
        </div>
        <div className="gl-card-meta">
          <Avatar name={hasChannelName ? channelName : title} src={stream.avatar} size={44} />
          <div className="gl-card-text">
            <div className="gl-card-title" title={title}>
              {title}
            </div>
            <div className="gl-card-chan">
              {hasChannelName ? (
                <span className="truncate">{channelName}</span>
              ) : (
                <span className="gl-card-chan-skeleton" aria-hidden="true" />
              )}
              {stream.verified && <VerifiedBadge size={14} />}
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
                <Icons.WatchLater size={15} />
                {saved
                  ? t('liveCard.menu.removeWatchLater', { defaultValue: 'Remove from Watch later' })
                  : t('liveCard.menu.saveWatchLater', { defaultValue: 'Save to Watch later' })}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => setShareOpen(true)}>
                {t('liveCard.menu.shareLiveLink', { defaultValue: 'Share' })}
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
      <ShareDialog
        open={shareOpen}
        onOpenChange={setShareOpen}
        title={title}
        url={`${window.location.origin}/live/${encodeURIComponent(stream.id)}`}
        description={stream.description}
        previewImage={stream.cover}
        previewKicker={t('shareDialog.liveKicker', { defaultValue: 'Live stream' })}
        previewMeta={`${channelName}${t('shareDialog.metaSeparator', { defaultValue: ' · ' })}${
          isLive
            ? t('home.watching', { count: stream.viewers, defaultValue: '{{count}} watching' })
            : hasReplay
              ? t('liveRoom.replay.badge', { defaultValue: 'Replay' })
              : isScheduled
                ? t('liveRoom.appointmentWaiting', {
                    count: stream.viewers,
                    defaultValue: '{{count}} waiting',
                  })
                : t('library.status.offline', { defaultValue: 'Offline' })
        }`}
      />
    </>
  );
}
