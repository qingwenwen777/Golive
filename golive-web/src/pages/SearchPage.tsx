import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import {
  Bell,
  CalendarClock,
  CloudOff,
  Heart,
  Inbox,
  MessageCircle,
  Search as SearchIcon,
  UserPlus,
} from 'lucide-react';
import { useFollow, useUnfollow, type AppointmentItem } from '@/api/room';
import { useSearchResults, type SearchCreator } from '@/api/search';
import type { ChannelPost } from '@/api/posts';
import { Avatar } from '@/components/Avatar';
import { FanClubExclusiveBadge } from '@/components/FanClubExclusiveBadge';
import { LiveBadge } from '@/components/LiveBadge';
import { LoadableImage } from '@/components/LoadableImage';
import { VerifiedBadge } from '@/components/VerifiedBadge';
import { cn } from '@/lib/cn';
import { streamChannelName, type Stream } from '@/types/stream';
import { useAuthModalStore } from '@/stores/useAuthModalStore';
import { useIsAuthed } from '@/stores/useAuthStore';

type SearchFilter = 'all' | 'creators' | 'live' | 'replays' | 'appointments' | 'posts';

const FILTERS: SearchFilter[] = ['all', 'creators', 'live', 'replays', 'appointments', 'posts'];

export default function SearchPage() {
  const { t } = useTranslation('pages');
  const [params] = useSearchParams();
  const query = params.get('q')?.trim() ?? '';
  const [filter, setFilter] = useState<SearchFilter>('all');
  const results = useSearchResults(query, 10, query.length > 0);
  const data = results.data;
  useEffect(() => {
    setFilter('all');
  }, [query]);
  const counts = useMemo(
    () => ({
      all: data?.total ?? 0,
      creators: data?.creators.length ?? 0,
      live: data?.live.length ?? 0,
      replays: data?.replays.length ?? 0,
      appointments: data?.appointments.length ?? 0,
      posts: data?.posts.length ?? 0,
    }),
    [data],
  );

  return (
    <div className="gl-page gl-search-page">
      <div
        className="gl-search-filterbar"
        role="tablist"
        aria-label={t('search.filterAria', { defaultValue: 'Search filters' })}
      >
        {FILTERS.map((item) => (
          <button
            key={item}
            type="button"
            className={filter === item ? 'is-active' : undefined}
            onClick={() => setFilter(item)}
          >
            {t(`search.filters.${item}`)}
            {counts[item] > 0 && <span>{counts[item]}</span>}
          </button>
        ))}
      </div>

      {!query ? (
        <SearchEmpty
          icon={<SearchIcon size={54} strokeWidth={1.5} />}
          title={t('search.startTitle', { defaultValue: '搜索' })}
        />
      ) : results.isPending ? (
        <SearchSkeleton />
      ) : results.isError ? (
        <SearchEmpty
          icon={<CloudOff size={56} strokeWidth={1.5} />}
          title={t('home.errorTitle')}
          sub={t('home.errorSub')}
          action={
            <button type="button" className="gl-retry-btn" onClick={() => results.refetch()}>
              {t('home.retry')}
            </button>
          }
        />
      ) : !data || data.total === 0 ? (
        <SearchEmpty
          icon={<Inbox size={56} strokeWidth={1.5} />}
          title={t('search.emptyTitle', { defaultValue: '没有找到相关内容' })}
          sub={t('search.emptySub', { defaultValue: '换个关键词试试。' })}
        />
      ) : (
        <div className="gl-search-results" style={{ opacity: results.isFetching ? 0.72 : 1 }}>
          {(filter === 'all' || filter === 'creators') && (
            <SearchSection
              title={t('search.creators', { defaultValue: '主播' })}
              visible={filter === 'creators' || data.creators.length > 0}
              empty={filter === 'creators' && data.creators.length === 0}
            >
              {data.creators.map((creator) => (
                <CreatorResultRow key={creator.id} creator={creator} />
              ))}
            </SearchSection>
          )}

          {(filter === 'all' || filter === 'live') && (
            <SearchSection
              title={t('search.live', { defaultValue: '正在直播' })}
              visible={filter === 'live' || data.live.length > 0}
              empty={filter === 'live' && data.live.length === 0}
            >
              {data.live.map((stream) => (
                <StreamResultRow key={stream.id} stream={stream} kind="live" />
              ))}
            </SearchSection>
          )}

          {(filter === 'all' || filter === 'replays') && (
            <SearchSection
              title={t('search.replays', { defaultValue: '直播回放' })}
              visible={filter === 'replays' || data.replays.length > 0}
              empty={filter === 'replays' && data.replays.length === 0}
            >
              {data.replays.map((stream) => (
                <StreamResultRow key={stream.id} stream={stream} kind="replay" />
              ))}
            </SearchSection>
          )}

          {(filter === 'all' || filter === 'appointments') && (
            <SearchSection
              title={t('search.appointments', { defaultValue: '直播预告' })}
              visible={filter === 'appointments' || data.appointments.length > 0}
              empty={filter === 'appointments' && data.appointments.length === 0}
            >
              {data.appointments.map((appointment) => (
                <AppointmentResultRow key={appointment.id} appointment={appointment} />
              ))}
            </SearchSection>
          )}

          {(filter === 'all' || filter === 'posts') && (
            <SearchSection
              title={t('search.posts', { defaultValue: '帖子' })}
              visible={filter === 'posts' || data.posts.length > 0}
              empty={filter === 'posts' && data.posts.length === 0}
            >
              {data.posts.map((post) => (
                <PostResultRow key={post.id} post={post} />
              ))}
            </SearchSection>
          )}
        </div>
      )}
    </div>
  );
}

function SearchSection({
  title,
  visible,
  empty,
  children,
}: {
  title: string;
  visible: boolean;
  empty: boolean;
  children: ReactNode;
}) {
  const { t } = useTranslation('pages');
  if (!visible) return null;
  return (
    <section className="gl-search-section">
      <h2>{title}</h2>
      {empty ? (
        <div className="gl-search-soft-empty">
          {t('search.noResults', { defaultValue: 'No results' })}
        </div>
      ) : (
        children
      )}
    </section>
  );
}

function CreatorResultRow({ creator }: { creator: SearchCreator }) {
  const { t } = useTranslation('pages');
  const isAuthed = useIsAuthed();
  const openLogin = useAuthModalStore((s) => s.openLogin);
  const queryClient = useQueryClient();
  const follow = useFollow(creator.channelId);
  const unfollow = useUnfollow(creator.channelId);
  const pending = follow.isPending || unfollow.isPending;
  const channelPath = `/channel/${encodeURIComponent(creator.channelId)}`;

  const toggleFollow = () => {
    if (creator.self) return;
    if (!isAuthed) {
      openLogin();
      return;
    }
    const mutation = creator.following ? unfollow : follow;
    mutation.mutate(undefined, {
      onSettled: () => {
        void queryClient.invalidateQueries({ queryKey: ['search-results'] });
        void queryClient.invalidateQueries({ queryKey: ['search-suggestions'] });
      },
    });
  };

  return (
    <article className="gl-search-result-row is-creator">
      <Link className="gl-search-creator-link" to={channelPath}>
        <Avatar name={creator.name} src={creator.avatar} size={116} />
      </Link>
      <div className="gl-search-result-main">
        <Link className="gl-search-title" to={channelPath}>
          {creator.name}
          {creator.verified && <VerifiedBadge size={16} />}
        </Link>
        <div className="gl-search-meta">
          {creator.username && <span>@{creator.username}</span>}
          <span>
            {t('search.subscribers', {
              count: creator.subscriberCount,
              defaultValue: '{{count}} subscribers',
            })}
          </span>
          {creator.live && (
            <span className="is-live-dot">{t('search.liveDot', { defaultValue: 'Live now' })}</span>
          )}
        </div>
        {creator.lastTitle && <p>{creator.lastTitle}</p>}
      </div>
      <button
        type="button"
        className={cn('gl-search-follow-btn', creator.following && 'is-following')}
        disabled={pending || creator.self}
        onClick={toggleFollow}
      >
        {creator.self ? (
          t('account.yourChannel', { defaultValue: '你的频道' })
        ) : creator.following ? (
          <>
            <Bell size={18} />
            {t('home.recommendations.subscribed')}
          </>
        ) : (
          <>
            <UserPlus size={18} />
            {t('home.recommendations.subscribe')}
          </>
        )}
      </button>
    </article>
  );
}

function StreamResultRow({ stream, kind }: { stream: Stream; kind: 'live' | 'replay' }) {
  const { t, i18n } = useTranslation('pages');
  const channelName = streamChannelName(stream);
  const title = i18n.language === 'ja' ? (stream.titleJa ?? stream.title) : stream.title;
  const category =
    i18n.language === 'ja' ? (stream.categoryJa ?? stream.category) : stream.category;
  const isLive = kind === 'live';

  return (
    <article className="gl-search-result-row">
      <Link className="gl-search-thumb" to={`/live/${encodeURIComponent(stream.id)}`}>
        {stream.cover ? (
          <LoadableImage src={stream.cover} alt="" loading="lazy" />
        ) : (
          <span>{title.slice(0, 1)}</span>
        )}
        <span className={cn('gl-search-thumb-badge', isLive && 'is-live')}>
          {isLive ? <LiveBadge /> : t('liveRoom.replay.badge', { defaultValue: '回放' })}
        </span>
        {stream.fanClubOnly && (
          <FanClubExclusiveBadge compact className="gl-search-exclusive-badge" />
        )}
        {!isLive && <span className="gl-search-duration">{stream.duration}</span>}
      </Link>
      <div className="gl-search-result-main">
        <Link className="gl-search-title" to={`/live/${encodeURIComponent(stream.id)}`}>
          {title}
        </Link>
        <div className="gl-search-meta">
          <span>
            {isLive
              ? t('home.watching', { count: stream.viewers, defaultValue: '{{count}} watching' })
              : t('search.views', { count: stream.viewers, defaultValue: '{{count}} views' })}
          </span>
          <span>{category}</span>
          {!isLive && stream.endedAt && <span>{formatDate(stream.endedAt, i18n.language)}</span>}
        </div>
        <Link
          className="gl-search-channel-line"
          to={`/channel/${encodeURIComponent(stream.channelId || channelName)}`}
        >
          <Avatar name={channelName} src={stream.avatar} size={28} />
          <span>{channelName}</span>
          {stream.verified && <VerifiedBadge size={14} />}
        </Link>
        {stream.description && <p>{stream.description}</p>}
      </div>
    </article>
  );
}

function AppointmentResultRow({ appointment }: { appointment: AppointmentItem }) {
  const { t, i18n } = useTranslation('pages');
  return (
    <article className="gl-search-result-row">
      <Link className="gl-search-thumb" to={`/live/${encodeURIComponent(appointment.roomId)}`}>
        {appointment.cover ? (
          <LoadableImage src={appointment.cover} alt="" loading="lazy" />
        ) : (
          <span>{appointment.title.slice(0, 1)}</span>
        )}
        <span className="gl-search-thumb-badge is-appointment">
          <CalendarClock size={14} />
          {t('search.appointmentBadge', { defaultValue: '预告' })}
        </span>
        {appointment.fanClubOnly && (
          <FanClubExclusiveBadge compact className="gl-search-exclusive-badge" />
        )}
      </Link>
      <div className="gl-search-result-main">
        <Link className="gl-search-title" to={`/live/${encodeURIComponent(appointment.roomId)}`}>
          {appointment.title}
        </Link>
        <div className="gl-search-meta">
          <span>{formatDate(appointment.scheduledAt, i18n.language)}</span>
          <span>
            {t('search.reservations', {
              count: appointment.reservationCount,
              defaultValue: '{{count}} reserved',
            })}
          </span>
          <span>{appointment.category}</span>
        </div>
        <Link
          className="gl-search-channel-line"
          to={`/channel/${encodeURIComponent(appointment.channelId || appointment.ownerId)}`}
        >
          <Avatar name={appointment.channel} src={appointment.avatar} size={28} />
          <span>{appointment.channel}</span>
          {appointment.verified && <VerifiedBadge size={14} />}
        </Link>
        {appointment.description && <p>{appointment.description}</p>}
      </div>
    </article>
  );
}

function PostResultRow({ post }: { post: ChannelPost }) {
  const { t, i18n } = useTranslation('pages');
  const channelPath = `/channel/${encodeURIComponent(post.channelId)}`;
  const image = post.images[0];

  return (
    <article className="gl-search-result-row is-post">
      <Link className="gl-search-post-author" to={channelPath}>
        <Avatar name={post.author.name} src={post.author.avatar} size={44} />
      </Link>
      <div className="gl-search-result-main">
        <Link className="gl-search-channel-line is-post-author" to={channelPath}>
          <span>{post.author.name}</span>
          {post.author.verified && <VerifiedBadge size={14} />}
          <small>{formatDate(post.createdAt, i18n.language)}</small>
        </Link>
        <Link className="gl-search-post-content" to={channelPath}>
          {post.content || t('search.imagePost', { defaultValue: 'Image post' })}
        </Link>
        <div className="gl-search-post-actions">
          <span>
            <Heart size={16} />
            {post.likeCount.toLocaleString()}
          </span>
          <span>
            <MessageCircle size={16} />
            {post.commentCount.toLocaleString()}
          </span>
        </div>
      </div>
      {image && (
        <Link className="gl-search-post-image" to={channelPath}>
          <LoadableImage src={image} alt="" loading="lazy" />
        </Link>
      )}
    </article>
  );
}

function SearchSkeleton() {
  return (
    <div className="gl-search-results" aria-busy="true">
      {Array.from({ length: 6 }).map((_, index) => (
        <div className="gl-search-result-row is-loading" key={index}>
          <span />
          <div>
            <i />
            <i />
            <i />
          </div>
        </div>
      ))}
    </div>
  );
}

function SearchEmpty({
  icon,
  title,
  sub,
  action,
}: {
  icon: ReactNode;
  title: string;
  sub?: string;
  action?: ReactNode;
}) {
  return (
    <div className="gl-empty gl-search-empty">
      {icon}
      <div className="gl-empty-title">{title}</div>
      {sub && <div className="gl-empty-sub">{sub}</div>}
      {action}
    </div>
  );
}

function formatDate(value: string, locale: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return new Intl.DateTimeFormat(locale, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}
