import type { Gift } from '@/types/gift';

type GiftNameTranslator = (key: string, options: { defaultValue: string }) => string;

export function localizedGiftName(
  gift: Pick<Gift, 'id' | 'name' | 'nameJa'>,
  language?: string,
  translate?: GiftNameTranslator,
): string {
  const fallback =
    language?.toLowerCase().startsWith('ja') && gift.nameJa?.trim() ? gift.nameJa : gift.name;
  if (translate) {
    return translate(`giftCatalog.names.${gift.id}`, { defaultValue: fallback });
  }
  return fallback;
}
