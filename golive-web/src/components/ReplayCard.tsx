import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { Avatar } from '@/components/Avatar';
import { FanClubExclusiveBadge } from '@/components/FanClubExclusiveBadge';
import { LoadableImage } from '@/components/LoadableImage';
import { useCoverHoverStyle } from '@/hooks/useCoverHoverStyle';
import { cn } from '@/lib/cn';
import { isPlaceholderChannelName, streamChannelName } from '@/types/stream';

/**
 * Minimal shape needed to render a replay (VOD) card. Both `HotReplayItem`
 * (home page) and `ChannelHistoryItem` (channel page) satisfy this structurally.
 */
export interface ReplayCardData {
  id: string;
  title: string;
  titleJa?: string;
  cover?: string;
  category?: string;
  categoryJa?: string;
  duration?: string;
  channel?: string;
  ownerId?: string;
  avatar?: string;
  fanClubOnly?: boolean;
}

export interface ReplayCardProps {
  replay: ReplayCardData;
  /** Fallback channel name when the item itself doesn't carry channel identity. */
  channelName?: string;
  /** Fallback avatar when the item itself doesn't carry one (e.g. channel page). */
  channelAvatar?: string;
  /** Eager-load the cover image for above-the-fold cards. */
  priority?: boolean;
}

/**
 * A fully clickable replay card matching the home page "Popular replays" style.
 * Clicking anywhere on the card opens the replay player at `/live/:id`.
 */
export function ReplayCard({ replay, channelName, channelAvatar, priority }: ReplayCardProps) {
  const { t, i18n } = useTranslation('pages');
  const resolvedChannelName =
    channelName ?? streamChannelName({ channel: replay.channel ?? '', ownerId: replay.ownerId });
  const hasChannelName = !isPlaceholderChannelName(resolvedChannelName);
  const title = i18n.language === 'ja' ? (replay.titleJa ?? replay.title) : replay.title;
  const category =
    i18n.language === 'ja' ? (replay.categoryJa ?? replay.category) : replay.category;
  const hoverStyle = useCoverHoverStyle(replay.cover, resolvedChannelName || title || replay.id);
  const avatarSrc = replay.avatar || channelAvatar;
  const replayBadge = t('liveRoom.replay.badge', { defaultValue: 'Replay' });

  return (
    <Link
      className="gl-home-replay-card gl-video-hover-card"
      style={hoverStyle}
      to={`/live/${encodeURIComponent(replay.id)}`}
    >
      <div className={cn('gl-home-replay-cover', replay.cover ? 'has-image' : 'is-placeholder')}>
        {replay.cover ? (
          <LoadableImage src={replay.cover} alt="" loading={priority ? 'eager' : 'lazy'} />
        ) : null}
        <div className="gl-home-replay-top">
          <div className="gl-home-replay-badge">{replayBadge}</div>
          {replay.fanClubOnly && (
            <FanClubExclusiveBadge compact className="gl-home-replay-exclusive-badge" />
          )}
        </div>
        {replay.duration ? <div className="gl-home-replay-duration">{replay.duration}</div> : null}
      </div>
      <div className="gl-home-replay-body">
        <Avatar name={hasChannelName ? resolvedChannelName : title} src={avatarSrc} size={44} />
        <div className="gl-home-replay-copy">
          <div className="gl-home-replay-title" title={title}>
            {title}
          </div>
          <div className="gl-home-replay-channel">
            {hasChannelName ? (
              resolvedChannelName
            ) : (
              <span className="gl-card-chan-skeleton" aria-hidden="true" />
            )}
          </div>
          <div className="gl-home-replay-meta">
            {replayBadge}
            {category ? ` · ${category}` : ''}
          </div>
        </div>
      </div>
    </Link>
  );
}
