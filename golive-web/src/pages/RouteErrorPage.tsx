import { useEffect } from 'react';
import { isRouteErrorResponse, Link, useRouteError } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import NotFoundPage from '@/pages/NotFoundPage';
import { isChunkLoadError } from '@/lib/chunkLoadError';

const CHUNK_RELOAD_KEY = 'golive-chunk-reload-at';
const CHUNK_RELOAD_INTERVAL_MS = 60_000;

function reloadOnceForNewBuild() {
  try {
    const last = Number(window.sessionStorage.getItem(CHUNK_RELOAD_KEY) || 0);
    if (Date.now() - last < CHUNK_RELOAD_INTERVAL_MS) return;
    window.sessionStorage.setItem(CHUNK_RELOAD_KEY, String(Date.now()));
  } catch {
    return;
  }
  window.location.reload();
}

export default function RouteErrorPage() {
  const error = useRouteError();
  const { t } = useTranslation('pages');
  const notFound = isRouteErrorResponse(error) && error.status === 404;
  const staleBuild = !notFound && isChunkLoadError(error);

  useEffect(() => {
    if (staleBuild) reloadOnceForNewBuild();
    else if (!notFound) console.error(error);
  }, [error, notFound, staleBuild]);

  if (notFound) return <NotFoundPage />;

  return (
    <div
      role="alert"
      className="flex min-h-[60vh] flex-col items-center justify-center gap-2 p-10 text-center"
    >
      <h1 className="text-2xl font-bold">
        {staleBuild
          ? t('routeError.updatedTitle', { defaultValue: 'GoLive has been updated' })
          : t('routeError.title', { defaultValue: 'This page hit an error' })}
      </h1>
      <p className="text-text-secondary">
        {staleBuild
          ? t('routeError.updatedBody', {
              defaultValue: 'Reload to get the new version of this page.',
            })
          : t('routeError.body', {
              defaultValue: "Reloading usually fixes it. If it doesn't, head back home.",
            })}
      </p>
      <div className="mt-4 flex items-center gap-4">
        <button type="button" className="gl-retry-btn" onClick={() => window.location.reload()}>
          {t('routeError.reload', { defaultValue: 'Reload' })}
        </button>
        <Link to="/" className="text-blue hover:underline">
          {t('notFound.back', { defaultValue: 'Back to home' })}
        </Link>
      </div>
    </div>
  );
}
