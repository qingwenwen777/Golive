import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

export default function NotFoundPage() {
  const { t } = useTranslation('pages');
  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center p-10">
      <h1 className="text-3xl font-bold">{t('notFound.title')}</h1>
      <p className="mt-2 text-text-secondary">{t('notFound.subtitle')}</p>
      <Link to="/" className="mt-6 text-blue hover:underline">
        {t('notFound.back')}
      </Link>
    </div>
  );
}
