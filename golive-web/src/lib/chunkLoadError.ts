// A lazy route's chunk fails to load when a deploy replaced the hashed files
// that an open tab still points at.
export function isChunkLoadError(error: unknown): boolean {
  const message = error instanceof Error ? `${error.name} ${error.message}` : String(error ?? '');
  return /Failed to fetch dynamically imported module|error loading dynamically imported module|Importing a module script failed|ChunkLoadError|Loading chunk \S+ failed/i.test(
    message,
  );
}
