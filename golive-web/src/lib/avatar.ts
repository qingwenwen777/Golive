// White text reads at 4.5:1 or better on each of these.
const AVATAR_COLORS = [
  '#b42318',
  '#c2410c',
  '#a16207',
  '#4d7c0f',
  '#15803d',
  '#0f766e',
  '#0e7490',
  '#0369a1',
  '#4338ca',
  '#7e22ce',
  '#a21caf',
  '#be185d',
];

export function avatarColor(name: string): string {
  let hash = 2166136261;
  for (const ch of name.trim().toLowerCase()) {
    hash ^= ch.codePointAt(0) ?? 0;
    hash = Math.imul(hash, 16777619);
  }
  return AVATAR_COLORS[(hash >>> 0) % AVATAR_COLORS.length];
}

const LETTER_OR_DIGIT = /[\p{L}\p{N}]/u;

/** The first letter or digit of the name ("🌸 Sakura" → "S", "星野" → "星"). */
export function avatarInitial(name: string): string {
  const chars = Array.from(name.trim());
  const first = chars.find((ch) => LETTER_OR_DIGIT.test(ch)) ?? chars[0] ?? '';
  return first.toLocaleUpperCase();
}
