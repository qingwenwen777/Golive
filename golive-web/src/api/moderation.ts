import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { http } from '@/lib/axios';

export interface ModerationUser {
  id: string;
  username?: string;
  displayName?: string;
  name: string;
  avatar?: string;
  verified: boolean;
  moderator: boolean;
  createdAt?: string;
}

export interface ModerationUserListResp {
  items: ModerationUser[];
  total: number;
  page: number;
  size: number;
}

export interface ModerationLog {
  id: string;
  ownerId: string;
  roomId?: string;
  actorId: string;
  actorName: string;
  actorAvatar?: string;
  targetUserId: string;
  targetName: string;
  targetAvatar?: string;
  action: 'add_moderator' | 'remove_moderator' | 'mute' | string;
  durationMinutes?: number;
  createdAt: string;
}

export interface ModerationLogListResp {
  items: ModerationLog[];
  total: number;
  page: number;
  size: number;
}

export interface RoomModerationState {
  roomId: string;
  ownerId: string;
  role: 'owner' | 'moderator' | 'viewer' | string;
  canModerate: boolean;
  muted?: boolean;
  muteExpiresAt?: string;
  muteRemainingSeconds?: number;
}

export interface MuteUserPayload {
  targetUserId: string;
  targetName?: string;
  targetAvatar?: string;
  durationMinutes: 5 | 10 | 30 | 60;
}

export interface MuteUserResp {
  roomId: string;
  targetUserId: string;
  targetName: string;
  durationMinutes: number;
  expiresAt: string;
}

export interface RoomMuteState {
  roomId: string;
  targetUserId: string;
  targetName?: string;
  targetAvatar?: string;
  muted: boolean;
  muteExpiresAt?: string;
  muteRemainingSeconds?: number;
}

export function useModeratorFollowers(query: string, page = 1, size = 10, enabled = true) {
  return useQuery<ModerationUserListResp, Error>({
    queryKey: ['moderation-followers', query, page, size],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<ModerationUserListResp>('/rooms/moderation/followers', {
        params: { q: query || undefined, page, size },
        signal,
      });
      return data;
    },
    enabled,
    staleTime: 15_000,
    placeholderData: keepPreviousData,
    retry: 1,
  });
}

export function useRoomModerators(page = 1, size = 20, enabled = true) {
  return useQuery<ModerationUserListResp, Error>({
    queryKey: ['room-moderators', page, size],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<ModerationUserListResp>('/rooms/moderation/moderators', {
        params: { page, size },
        signal,
      });
      return data;
    },
    enabled,
    staleTime: 15_000,
    placeholderData: keepPreviousData,
    retry: 1,
  });
}

export function useModeratorLogs(page = 1, size = 10, enabled = true) {
  return useQuery<ModerationLogListResp, Error>({
    queryKey: ['moderation-logs', page, size],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<ModerationLogListResp>('/rooms/moderation/logs', {
        params: { page, size },
        signal,
      });
      return data;
    },
    enabled,
    staleTime: 10_000,
    placeholderData: keepPreviousData,
    retry: 1,
  });
}

export function useAddModerator() {
  const qc = useQueryClient();
  return useMutation<ModerationUser, Error, string>({
    mutationFn: async (userId) => {
      const { data } = await http.post<ModerationUser>(
        `/rooms/moderation/moderators/${encodeURIComponent(userId)}`,
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['moderation-followers'] });
      void qc.invalidateQueries({ queryKey: ['room-moderators'] });
      void qc.invalidateQueries({ queryKey: ['moderation-logs'] });
      void qc.invalidateQueries({ queryKey: ['room-moderation-state'] });
    },
  });
}

export function useRemoveModerator() {
  const qc = useQueryClient();
  return useMutation<{ ok: boolean }, Error, string>({
    mutationFn: async (userId) => {
      const { data } = await http.delete<{ ok: boolean }>(
        `/rooms/moderation/moderators/${encodeURIComponent(userId)}`,
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['moderation-followers'] });
      void qc.invalidateQueries({ queryKey: ['room-moderators'] });
      void qc.invalidateQueries({ queryKey: ['moderation-logs'] });
      void qc.invalidateQueries({ queryKey: ['room-moderation-state'] });
    },
  });
}

export function useRoomModerationState(roomId: string, enabled = true) {
  return useQuery<RoomModerationState, Error>({
    queryKey: ['room-moderation-state', roomId],
    queryFn: ({ signal }) => getRoomModerationState(roomId, signal),
    enabled: enabled && !!roomId,
    staleTime: 10_000,
    retry: 1,
  });
}

export async function getRoomModerationState(roomId: string, signal?: AbortSignal) {
  const { data } = await http.get<RoomModerationState>(
    `/rooms/${encodeURIComponent(roomId)}/moderation/state`,
    { signal },
  );
  return data;
}

export function useRoomMuteState(roomId: string, targetUserId: string, enabled = true) {
  return useQuery<RoomMuteState, Error>({
    queryKey: ['room-mute-state', roomId, targetUserId],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<RoomMuteState>(
        `/rooms/${encodeURIComponent(roomId)}/moderation/mutes/${encodeURIComponent(targetUserId)}`,
        { signal },
      );
      return data;
    },
    enabled: enabled && !!roomId && !!targetUserId,
    staleTime: 2_000,
    retry: 1,
  });
}

export function useMuteRoomUser(roomId: string) {
  const qc = useQueryClient();
  return useMutation<MuteUserResp, Error, MuteUserPayload>({
    mutationFn: async (payload) => {
      const { data } = await http.post<MuteUserResp>(
        `/rooms/${encodeURIComponent(roomId)}/moderation/mutes`,
        payload,
      );
      return data;
    },
    onSuccess: (_data, payload) => {
      void qc.invalidateQueries({ queryKey: ['moderation-logs'] });
      void qc.invalidateQueries({ queryKey: ['room-mute-state', roomId, payload.targetUserId] });
      void qc.invalidateQueries({ queryKey: ['room-moderation-state', roomId] });
    },
  });
}

export function useUnmuteRoomUser(roomId: string) {
  const qc = useQueryClient();
  return useMutation<RoomMuteState, Error, string>({
    mutationFn: async (targetUserId) => {
      const { data } = await http.delete<RoomMuteState>(
        `/rooms/${encodeURIComponent(roomId)}/moderation/mutes/${encodeURIComponent(targetUserId)}`,
      );
      return data;
    },
    onSuccess: (_data, targetUserId) => {
      void qc.invalidateQueries({ queryKey: ['moderation-logs'] });
      void qc.invalidateQueries({ queryKey: ['room-mute-state', roomId, targetUserId] });
      void qc.invalidateQueries({ queryKey: ['room-moderation-state', roomId] });
    },
  });
}
