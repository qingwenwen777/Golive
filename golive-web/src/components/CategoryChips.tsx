import { useTranslation } from 'react-i18next';
import { useLangStore } from '@/stores/useLangStore';
import { CATEGORIES_EN, CATEGORIES_JA } from '@/constants/catalog';
import { cn } from '@/lib/cn';

export interface CategoryChipsProps {
  active: string;
  onPick: (key: string) => void;
}

export function CategoryChips({ active, onPick }: CategoryChipsProps) {
  const { t } = useTranslation('pages');
  const lang = useLangStore((s) => s.lang);
  const displayList = lang === 'ja' ? CATEGORIES_JA : CATEGORIES_EN;

  const items = displayList.map((label, i) => ({
    key: CATEGORIES_EN[i] ?? label,
    label: i === 0 ? t('home.allCategory') : label,
  }));

  return (
    <div className="gl-chips" role="tablist">
      {items.map((it) => {
        const isActive = active === it.key || (active === 'all' && it.key === 'All');
        return (
          <button
            key={it.key}
            role="tab"
            aria-selected={isActive}
            onClick={() => onPick(it.key)}
            className={cn('gl-chip', isActive && 'is-active')}
          >
            {it.label}
          </button>
        );
      })}
    </div>
  );
}
