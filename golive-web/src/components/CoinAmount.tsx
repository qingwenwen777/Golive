import { Coins } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/cn';
import { formatNumber } from '@/lib/format';

interface CoinAmountProps {
  value: number;
  iconSize?: number;
  className?: string;
}

// A coin price or amount shown as the coin glyph plus the number, the way gift
// prices are shown. Screen readers hear "2,000 coins".
export function CoinAmount({ value, iconSize = 14, className }: CoinAmountProps) {
  const { t, i18n } = useTranslation('common');
  const amount = formatNumber(value, i18n.resolvedLanguage);
  return (
    <span className={cn('gl-coin-amount', className)}>
      <Coins size={iconSize} aria-hidden="true" />
      <span aria-hidden="true">{amount}</span>
      <span className="sr-only">{t('account.coins', { amount, defaultValue: '{{amount}} coins' })}</span>
    </span>
  );
}
