import i18next from 'i18next';

// Shared number and date formatting. Everything formats in the app language
// (not the browser's), and bad input returns '' instead of throwing.

export function appLocale(): string {
  return i18next.resolvedLanguage || i18next.language || 'en-US';
}

const numberFormats = new Map<string, Intl.NumberFormat>();
const dateFormats = new Map<string, Intl.DateTimeFormat>();
const relativeFormats = new Map<string, Intl.RelativeTimeFormat>();

function numberFormat(locale: string, options: Intl.NumberFormatOptions): Intl.NumberFormat {
  const key = `${locale}|${JSON.stringify(options)}`;
  let format = numberFormats.get(key);
  if (!format) {
    format = new Intl.NumberFormat(locale, options);
    numberFormats.set(key, format);
  }
  return format;
}

function dateFormat(locale: string, options: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  const key = `${locale}|${JSON.stringify(options)}`;
  let format = dateFormats.get(key);
  if (!format) {
    format = new Intl.DateTimeFormat(locale, options);
    dateFormats.set(key, format);
  }
  return format;
}

function relativeFormat(locale: string): Intl.RelativeTimeFormat {
  let format = relativeFormats.get(locale);
  if (!format) {
    format = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
    relativeFormats.set(locale, format);
  }
  return format;
}

function finite(value: number | null | undefined): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

/** Exact, grouped: 12,480. Use for balances, prices and anything a user counts. */
export function formatNumber(value: number | null | undefined, locale = appLocale()): string {
  return numberFormat(locale, { maximumFractionDigits: 0 }).format(finite(value));
}

/**
 * Audience sizes (viewers, followers, likes…): exact below 10,000, compact
 * above, e.g. "9,311", "12K", "233K", "1.2M" (zh/ja: "1.2万", "23万").
 */
export function formatCount(value: number | null | undefined, locale = appLocale()): string {
  const n = finite(value);
  if (Math.abs(n) < 10_000) return formatNumber(n, locale);
  return numberFormat(locale, { notation: 'compact' }).format(n);
}

export type DateInput = string | number | Date | null | undefined;

export function toDate(value: DateInput): Date | null {
  if (value === null || value === undefined || value === '') return null;
  const date = value instanceof Date ? value : new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

/** "Sep 28, 9:04 PM"; the year is added when it isn't the current year. */
export function formatDateTime(value: DateInput, locale = appLocale(), now = new Date()): string {
  const date = toDate(value);
  if (!date) return '';
  const options: Intl.DateTimeFormatOptions = {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  };
  if (date.getFullYear() !== now.getFullYear()) options.year = 'numeric';
  return dateFormat(locale, options).format(date);
}

/** "Sep 28" (or "Sep 28, 2025" outside the current year). */
export function formatDate(value: DateInput, locale = appLocale(), now = new Date()): string {
  const date = toDate(value);
  if (!date) return '';
  const options: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric' };
  if (date.getFullYear() !== now.getFullYear()) options.year = 'numeric';
  return dateFormat(locale, options).format(date);
}

const RELATIVE_STEPS: Array<[Intl.RelativeTimeFormatUnit, number]> = [
  ['second', 60],
  ['minute', 60],
  ['hour', 24],
  ['day', 7],
];

/**
 * "just now", "5 minutes ago", "in 3 hours", "yesterday", "in 2 days". Beyond
 * a week it falls back to formatDateTime, which is easier to place than
 * "5 weeks ago".
 */
export function formatRelativeTime(value: DateInput, locale = appLocale(), now = new Date()): string {
  const date = toDate(value);
  if (!date) return '';
  let delta = (date.getTime() - now.getTime()) / 1000;
  if (Math.abs(delta) < 45) return relativeFormat(locale).format(0, 'second');
  for (const [unit, size] of RELATIVE_STEPS) {
    if (Math.abs(delta) < size) return relativeFormat(locale).format(Math.round(delta), unit);
    delta /= size;
  }
  return formatDateTime(date, locale, now);
}
