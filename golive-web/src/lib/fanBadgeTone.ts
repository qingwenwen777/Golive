export function fanBadgeToneClass(level: number) {
  const safeLevel = Number.isFinite(level) ? Math.max(1, Math.min(99, Math.floor(level))) : 1;
  const tone = safeLevel <= 20 ? Math.ceil(safeLevel / 5) : 4 + Math.ceil((safeLevel - 20) / 10);
  return `is-tone-${tone}`;
}
