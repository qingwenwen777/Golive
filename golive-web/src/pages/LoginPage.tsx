import { Link, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useRecommendedRooms } from '@/api/room';
import { LoadableImage } from '@/components/LoadableImage';
import { AuthPanel } from '@/features/auth/AuthPanel';
import { useLangStore } from '@/stores/useLangStore';
import { streamChannelName } from '@/types/stream';
import { isPlaceholderName } from '@/types/user';

export default function LoginPage() {
  const navigate = useNavigate();

  return (
    <div className="gl-auth-page">
      <div className="gl-auth-page-inner">
        <AuthPanel
          onAuthenticated={(resp) =>
            navigate(
              resp.user.banned
                ? '/account-banned'
                : resp.user.role === 'admin'
                  ? '/admin/dashboard'
                  : '/',
            )
          }
        />
        <LoginShowcase />
      </div>
    </div>
  );
}

// Next to the form: the most-watched stream that is live right now (anyone
// can watch it without signing in), or, when nothing is live, the mascot.
function LoginShowcase() {
  const { t } = useTranslation('pages');
  const { t: tc } = useTranslation('common');
  const lang = useLangStore((s) => s.lang);
  const rooms = useRecommendedRooms({ size: 1 });
  const stream = rooms.data?.items.find((item) => item.isLive === true || item.status === 'live');

  if (rooms.isPending) return <div className="gl-auth-preview is-loading" aria-hidden="true" />;
  if (!stream) {
    return (
      <aside className="gl-auth-preview is-brand">
        <img src="/golive-mascot.webp" alt="" width={200} height={200} />
        <h2>
          {t('login.showcase.title', {
            defaultValue: 'Live streams, chat, and replays in one place',
          })}
        </h2>
        <p>
          {t('login.showcase.body', {
            defaultValue:
              'Watch without an account. Sign in to chat, follow channels and send gifts.',
          })}
        </p>
      </aside>
    );
  }

  const title = lang === 'ja' ? (stream.titleJa ?? stream.title) : stream.title;
  const channelName = streamChannelName(stream);
  return (
    <Link
      to={`/live/${encodeURIComponent(stream.id)}`}
      className="gl-auth-preview"
      aria-label={t('login.showcase.watch', { title, defaultValue: 'Watch {{title}}' })}
    >
      {stream.cover ? <LoadableImage src={stream.cover} alt="" /> : null}
      <div className="gl-auth-preview-shade" />
      <div className="gl-auth-preview-top">
        <span className="gl-live-pill">{tc('live', { defaultValue: 'LIVE' })}</span>
        <span>{t('liveRoom.watching', { count: stream.viewers })}</span>
      </div>
      <div className="gl-auth-preview-bottom">
        <div className="gl-auth-preview-title">{title}</div>
        {!isPlaceholderName(channelName) && (
          <div className="gl-auth-preview-meta">{channelName}</div>
        )}
      </div>
    </Link>
  );
}
