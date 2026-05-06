import { useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  LIKED_STREAMS_KEY,
  WATCH_HISTORY_KEY,
  WATCH_LATER_KEY,
  clearLibrary,
  readLibrary,
} from '@/lib/liveLibrary';
import { useSyncUserLibrary, userLibraryQueryKey } from '@/api/library';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';

const syncedUsers = new Set<string>();

export function LibrarySyncBootstrap() {
  const queryClient = useQueryClient();
  const isAuthed = useIsAuthed();
  const userId = useAuthStore((s) => s.user?.id);
  const syncHistory = useSyncUserLibrary(WATCH_HISTORY_KEY);
  const syncWatchLater = useSyncUserLibrary(WATCH_LATER_KEY);
  const syncLiked = useSyncUserLibrary(LIKED_STREAMS_KEY);

  useEffect(() => {
    if (!isAuthed || !userId || syncedUsers.has(userId)) return;
    syncedUsers.add(userId);

    const syncLocalLibraries = async () => {
      const tasks = [
        { key: WATCH_HISTORY_KEY, sync: syncHistory.mutateAsync },
        { key: WATCH_LATER_KEY, sync: syncWatchLater.mutateAsync },
        { key: LIKED_STREAMS_KEY, sync: syncLiked.mutateAsync },
      ];

      await Promise.allSettled(
        tasks.map(async ({ key, sync }) => {
          const items = readLibrary(key);
          if (items.length === 0) return;
          await sync(items);
          clearLibrary(key);
        }),
      );

      for (const key of [WATCH_HISTORY_KEY, WATCH_LATER_KEY, LIKED_STREAMS_KEY]) {
        void queryClient.invalidateQueries({ queryKey: userLibraryQueryKey(key) });
      }
    };

    void syncLocalLibraries();
  }, [
    isAuthed,
    queryClient,
    syncHistory.mutateAsync,
    syncLiked.mutateAsync,
    syncWatchLater.mutateAsync,
    userId,
  ]);

  return null;
}
