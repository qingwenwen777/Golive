import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { http } from '@/lib/axios';
import {
  LIKED_STREAMS_KEY,
  MAX_LIBRARY_ITEMS,
  WATCH_HISTORY_KEY,
  WATCH_LATER_KEY,
  isInLibrary,
  readLibrary,
  syncLibraryWithLiveRooms,
  type LibraryStream,
} from '@/lib/liveLibrary';
import type { Stream } from '@/types/stream';

export type UserLibraryType = 'history' | 'watch_later' | 'liked';
export type LibraryStamp = 'savedAt' | 'watchedAt';

export interface UserLibraryResp {
  items: LibraryStream[];
}

interface ServerLibraryItem {
  roomId: string;
  savedAt?: string;
  watchedAt?: string;
}

export function libraryTypeFromStorageKey(storageKey: string): UserLibraryType {
  if (storageKey === WATCH_HISTORY_KEY) return 'history';
  if (storageKey === WATCH_LATER_KEY) return 'watch_later';
  if (storageKey === LIKED_STREAMS_KEY) return 'liked';
  return 'watch_later';
}

export function userLibraryQueryKey(storageKey: string) {
  return ['user-library', libraryTypeFromStorageKey(storageKey)] as const;
}

export function defaultLibraryStamp(storageKey: string): LibraryStamp {
  return storageKey === WATCH_HISTORY_KEY ? 'watchedAt' : 'savedAt';
}

export function useUserLibrary(storageKey: string, enabled: boolean) {
  const type = libraryTypeFromStorageKey(storageKey);
  return useQuery<UserLibraryResp, Error>({
    queryKey: userLibraryQueryKey(storageKey),
    queryFn: async ({ signal }) => {
      const { data } = await http.get<UserLibraryResp>(`/rooms/library/${type}`, { signal });
      return { items: data.items ?? [] };
    },
    enabled,
    staleTime: 20_000,
    retry: 0,
  });
}

export function useLibraryItems(storageKey: string, isAuthed: boolean, liveStreams?: Stream[]) {
  const query = useUserLibrary(storageKey, isAuthed);
  const [localItems, setLocalItems] = useState<LibraryStream[]>(() => readLibrary(storageKey));

  useEffect(() => {
    if (isAuthed) return;
    if (liveStreams) {
      setLocalItems(syncLibraryWithLiveRooms(storageKey, liveStreams));
      return;
    }
    setLocalItems(readLibrary(storageKey));
  }, [isAuthed, liveStreams, storageKey]);

  const items = isAuthed ? (query.data?.items ?? []) : localItems;

  return {
    items,
    setLocalItems,
    isPending: isAuthed && query.isPending,
    query,
  };
}

export function useLibraryMembership(storageKey: string, streamId: string, isAuthed: boolean) {
  const query = useUserLibrary(storageKey, isAuthed);
  const [localMember, setLocalMember] = useState(() => isInLibrary(storageKey, streamId));

  useEffect(() => {
    setLocalMember(isInLibrary(storageKey, streamId));
  }, [storageKey, streamId, isAuthed]);

  const serverMember = Boolean(query.data?.items.some((item) => item.id === streamId));
  return {
    isMember: isAuthed ? serverMember : localMember,
    isPending: isAuthed && query.isPending,
    setLocalMember,
  };
}

export function useSaveUserLibraryItem(storageKey: string) {
  const qc = useQueryClient();
  const queryKey = userLibraryQueryKey(storageKey);
  const type = libraryTypeFromStorageKey(storageKey);
  return useMutation<
    UserLibraryResp,
    Error,
    { stream: Stream; stamp?: LibraryStamp; at?: string },
    { prev?: UserLibraryResp }
  >({
    mutationFn: async ({ stream, stamp, at }) => {
      const { data } = await http.post<UserLibraryResp>(
        `/rooms/library/${type}`,
        toServerLibraryItem(stream, stamp ?? defaultLibraryStamp(storageKey), at),
      );
      return { items: data.items ?? [] };
    },
    onMutate: async ({ stream, stamp, at }) => {
      await qc.cancelQueries({ queryKey });
      const prev = qc.getQueryData<UserLibraryResp>(queryKey);
      qc.setQueryData<UserLibraryResp>(queryKey, {
        items: upsertLibraryItem(
          prev?.items ?? [],
          stream,
          stamp ?? defaultLibraryStamp(storageKey),
          at,
        ),
      });
      return { prev };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(queryKey, ctx.prev);
    },
    onSuccess: (data) => {
      qc.setQueryData<UserLibraryResp>(queryKey, data);
    },
  });
}

export function useRemoveUserLibraryItem(storageKey: string) {
  const qc = useQueryClient();
  const queryKey = userLibraryQueryKey(storageKey);
  const type = libraryTypeFromStorageKey(storageKey);
  return useMutation<UserLibraryResp, Error, string, { prev?: UserLibraryResp }>({
    mutationFn: async (roomId) => {
      const { data } = await http.delete<UserLibraryResp>(
        `/rooms/library/${type}/${encodeURIComponent(roomId)}`,
      );
      return { items: data.items ?? [] };
    },
    onMutate: async (roomId) => {
      await qc.cancelQueries({ queryKey });
      const prev = qc.getQueryData<UserLibraryResp>(queryKey);
      qc.setQueryData<UserLibraryResp>(queryKey, {
        items: (prev?.items ?? []).filter((item) => item.id !== roomId),
      });
      return { prev };
    },
    onError: (_err, _roomId, ctx) => {
      if (ctx?.prev) qc.setQueryData(queryKey, ctx.prev);
    },
    onSuccess: (data) => {
      qc.setQueryData<UserLibraryResp>(queryKey, data);
    },
  });
}

export function useClearUserLibrary(storageKey: string) {
  const qc = useQueryClient();
  const queryKey = userLibraryQueryKey(storageKey);
  const type = libraryTypeFromStorageKey(storageKey);
  return useMutation<UserLibraryResp, Error, void, { prev?: UserLibraryResp }>({
    mutationFn: async () => {
      const { data } = await http.delete<UserLibraryResp>(`/rooms/library/${type}`);
      return { items: data.items ?? [] };
    },
    onMutate: async () => {
      await qc.cancelQueries({ queryKey });
      const prev = qc.getQueryData<UserLibraryResp>(queryKey);
      qc.setQueryData<UserLibraryResp>(queryKey, { items: [] });
      return { prev };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(queryKey, ctx.prev);
    },
    onSuccess: (data) => {
      qc.setQueryData<UserLibraryResp>(queryKey, data);
    },
  });
}

export function useSyncUserLibrary(storageKey: string) {
  const qc = useQueryClient();
  const queryKey = userLibraryQueryKey(storageKey);
  const type = libraryTypeFromStorageKey(storageKey);
  return useMutation<UserLibraryResp, Error, LibraryStream[]>({
    mutationFn: async (items) => {
      const { data } = await http.post<UserLibraryResp>(`/rooms/library/${type}/sync`, {
        items: items
          .slice(0, MAX_LIBRARY_ITEMS)
          .map((item) => toServerLibraryItem(item, defaultLibraryStamp(storageKey))),
      });
      return { items: data.items ?? [] };
    },
    onSuccess: (data) => {
      qc.setQueryData<UserLibraryResp>(queryKey, data);
    },
  });
}

function toServerLibraryItem(
  stream: Pick<LibraryStream, 'id' | 'savedAt' | 'watchedAt'>,
  stamp: LibraryStamp,
  at?: string,
): ServerLibraryItem {
  const timestamp = at ?? (stamp === 'watchedAt' ? stream.watchedAt : stream.savedAt);
  return {
    roomId: stream.id,
    ...(stamp === 'watchedAt' ? { watchedAt: timestamp } : { savedAt: timestamp }),
  };
}

function upsertLibraryItem(
  items: LibraryStream[],
  stream: Stream,
  stamp: LibraryStamp,
  at = new Date().toISOString(),
): LibraryStream[] {
  return [{ ...stream, [stamp]: at }, ...items.filter((item) => item.id !== stream.id)].slice(
    0,
    MAX_LIBRARY_ITEMS,
  );
}
