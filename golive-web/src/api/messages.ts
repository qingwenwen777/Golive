import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { http } from '@/lib/axios';

export interface MessageUser {
  id: string;
  username?: string;
  displayName?: string;
  name: string;
  avatar?: string;
  verified: boolean;
  livePermissionStatus?: string;
  fanBadge?: MessageFanBadge;
}

export interface MessageFanBadge {
  creatorId: string;
  level: number;
}

export interface DirectThread {
  id: string;
  viewerId: string;
  creatorId: string;
  channelId: string;
  peer: MessageUser;
  lastMessagePreview?: string;
  lastSenderId?: string;
  lastMessageAt?: string;
  unread: number;
  pinned: boolean;
  muted: boolean;
  pushDisabled: boolean;
  canSend: boolean;
  awaitingReply: boolean;
  blocked: boolean;
}

export interface DirectThreadListResp {
  items: DirectThread[];
  total: number;
  page: number;
  size: number;
}

export interface DirectMessage {
  id: string;
  threadId: string;
  senderId: string;
  receiverId: string;
  sender?: MessageUser;
  body: string;
  createdAt: string;
}

export interface DirectMessageListResp {
  items: DirectMessage[];
  total: number;
  page: number;
  size: number;
}

export interface MessagePreference {
  messageReminderEnabled: boolean;
  replyReminderScope: 'all' | 'following' | 'none' | string;
  mentionReminderScope: 'all' | 'following' | 'none' | string;
  likeReminderEnabled: boolean;
  foldUnfollowedMessages: boolean;
}

export interface BlockedUser {
  user: MessageUser;
  role: string;
  reason?: string;
  createdAt: string;
}

export interface BlockedUserListResp {
  items: BlockedUser[];
  total: number;
  page: number;
  size: number;
}

export interface FanGroupMember {
  user: MessageUser;
  fanBadge?: MessageFanBadge;
  role: 'owner' | 'admin' | 'member' | string;
  muted: boolean;
  mutedUntil?: string;
  kicked?: boolean;
  kickedAt?: string;
  kickReason?: string;
  rejoinRequestedAt?: string;
  rejoinRejectedAt?: string;
  pendingRejoin?: boolean;
  createdAt: string;
}

export interface FanGroup {
  id: string;
  creatorId: string;
  groupNo: number;
  name: string;
  memberCount: number;
  members: FanGroupMember[];
  createdAt: string;
  updatedAt: string;
}

export interface FanGroupListResp {
  items: FanGroup[];
  total: number;
}

export interface FanGroupMessage {
  id: string;
  groupId: string;
  sender: MessageUser;
  role?: 'owner' | 'admin' | 'member' | string;
  fanBadge?: MessageFanBadge;
  body: string;
  createdAt: string;
}

export interface FanGroupMessageListResp {
  items: FanGroupMessage[];
  total: number;
  page: number;
  size: number;
}

export function useDirectThreads(enabled = true, page = 1, size = 30) {
  return useQuery<DirectThreadListResp, Error>({
    queryKey: ['direct-threads', page, size],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<DirectThreadListResp>('/messages/direct', {
        params: { page, size },
        signal,
      });
      return data;
    },
    enabled,
    staleTime: 10_000,
    refetchInterval: enabled ? 20_000 : false,
    placeholderData: keepPreviousData,
    retry: 1,
  });
}

export function useDirectDraft(creatorId: string, enabled = true) {
  return useQuery<DirectThread, Error>({
    queryKey: ['direct-draft', creatorId],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<DirectThread>(
        `/messages/direct/creators/${encodeURIComponent(creatorId)}`,
        { signal },
      );
      return data;
    },
    enabled: enabled && !!creatorId,
    staleTime: 10_000,
    retry: 0,
  });
}

export function useDirectMessages(threadId: string, enabled = true, page = 1, size = 100) {
  return useQuery<DirectMessageListResp, Error>({
    queryKey: ['direct-messages', threadId, page, size],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<DirectMessageListResp>(
        `/messages/direct/${encodeURIComponent(threadId)}`,
        { params: { page, size }, signal },
      );
      return data;
    },
    enabled: enabled && !!threadId,
    staleTime: 5_000,
    refetchInterval: enabled && threadId ? 10_000 : false,
    retry: 1,
  });
}

export function useSendDirect() {
  const qc = useQueryClient();
  return useMutation<DirectThread, Error, { creatorId?: string; channelId?: string; content: string }>({
    mutationFn: async (payload) => {
      const { data } = await http.post<DirectThread>('/messages/direct', payload);
      return data;
    },
    onSuccess: (thread) => {
      void qc.invalidateQueries({ queryKey: ['direct-threads'] });
      void qc.invalidateQueries({ queryKey: ['direct-draft', thread.creatorId] });
      void qc.invalidateQueries({ queryKey: ['direct-messages', thread.id] });
    },
  });
}

export function useSendThreadMessage(threadId: string) {
  const qc = useQueryClient();
  return useMutation<DirectThread, Error, string>({
    mutationFn: async (content) => {
      const { data } = await http.post<DirectThread>(
        `/messages/direct/${encodeURIComponent(threadId)}`,
        { content },
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['direct-threads'] });
      void qc.invalidateQueries({ queryKey: ['direct-messages', threadId] });
    },
  });
}

export function useUpdateThreadOptions(threadId: string) {
  const qc = useQueryClient();
  return useMutation<DirectThread, Error, Partial<Pick<DirectThread, 'pinned' | 'muted' | 'pushDisabled'>>>({
    mutationFn: async (payload) => {
      const { data } = await http.patch<DirectThread>(
        `/messages/direct/${encodeURIComponent(threadId)}`,
        payload,
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['direct-threads'] });
    },
  });
}

export function useMessagePreference(enabled = true) {
  return useQuery<MessagePreference, Error>({
    queryKey: ['message-preference'],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<MessagePreference>('/messages/settings', { signal });
      return data;
    },
    enabled,
    staleTime: 30_000,
    retry: 1,
  });
}

export function useUpdateMessagePreference() {
  const qc = useQueryClient();
  return useMutation<MessagePreference, Error, MessagePreference>({
    mutationFn: async (payload) => {
      const { data } = await http.patch<MessagePreference>('/messages/settings', payload);
      return data;
    },
    onSuccess: (pref) => {
      qc.setQueryData(['message-preference'], pref);
    },
  });
}

export function useBlockedUsers(enabled = true, page = 1, size = 50) {
  return useQuery<BlockedUserListResp, Error>({
    queryKey: ['message-blocks', page, size],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<BlockedUserListResp>('/messages/blocks', {
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

export function useBlockUser() {
  const qc = useQueryClient();
  return useMutation<{ ok: boolean }, Error, { userId: string; reason?: string }>({
    mutationFn: async ({ userId, reason }) => {
      const { data } = await http.post<{ ok: boolean }>(
        `/messages/blocks/${encodeURIComponent(userId)}`,
        { reason },
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['message-blocks'] });
      void qc.invalidateQueries({ queryKey: ['direct-threads'] });
      void qc.invalidateQueries({ queryKey: ['rooms'] });
      void qc.invalidateQueries({ queryKey: ['channel-posts'] });
    },
  });
}

export function useUnblockUser() {
  const qc = useQueryClient();
  return useMutation<{ ok: boolean }, Error, string>({
    mutationFn: async (userId) => {
      const { data } = await http.delete<{ ok: boolean }>(
        `/messages/blocks/${encodeURIComponent(userId)}`,
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['message-blocks'] });
      void qc.invalidateQueries({ queryKey: ['direct-threads'] });
      void qc.invalidateQueries({ queryKey: ['rooms'] });
      void qc.invalidateQueries({ queryKey: ['channel-posts'] });
    },
  });
}

export function useFanGroups(enabled = true) {
  return useQuery<FanGroupListResp, Error>({
    queryKey: ['fan-groups'],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<FanGroupListResp>('/messages/fan-groups', { signal });
      return data;
    },
    enabled,
    staleTime: 15_000,
    retry: 1,
  });
}

export function useJoinedFanGroups(enabled = true) {
  return useQuery<FanGroupListResp, Error>({
    queryKey: ['joined-fan-groups'],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<FanGroupListResp>('/messages/fan-groups/joined', { signal });
      return data;
    },
    enabled,
    staleTime: 15_000,
    retry: 1,
  });
}

export function useFanGroupMessages(groupId: string, enabled = true, page = 1, size = 100) {
  return useQuery<FanGroupMessageListResp, Error>({
    queryKey: ['fan-group-messages', groupId, page, size],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<FanGroupMessageListResp>(
        `/messages/fan-groups/${encodeURIComponent(groupId)}/messages`,
        { params: { page, size }, signal },
      );
      return data;
    },
    enabled: enabled && !!groupId,
    staleTime: 5_000,
    refetchInterval: enabled && groupId ? 10_000 : false,
    retry: 1,
  });
}

export function useSendFanGroupMessage(groupId: string) {
  const qc = useQueryClient();
  return useMutation<FanGroupMessage, Error, string>({
    mutationFn: async (content) => {
      const { data } = await http.post<FanGroupMessage>(
        `/messages/fan-groups/${encodeURIComponent(groupId)}/messages`,
        { content },
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['fan-group-messages', groupId] });
      void qc.invalidateQueries({ queryKey: ['joined-fan-groups'] });
    },
  });
}

export function useRequestFanGroupRejoin() {
  const qc = useQueryClient();
  return useMutation<FanGroupListResp, Error, string>({
    mutationFn: async (groupId) => {
      const { data } = await http.post<FanGroupListResp>(
        `/messages/fan-groups/${encodeURIComponent(groupId)}/rejoin-requests`,
      );
      return data;
    },
    onSuccess: (data) => {
      qc.setQueryData(['joined-fan-groups'], data);
      void qc.invalidateQueries({ queryKey: ['fan-groups'] });
    },
  });
}

export function useSyncFanGroups() {
  const qc = useQueryClient();
  return useMutation<FanGroupListResp, Error, void>({
    mutationFn: async () => {
      const { data } = await http.post<FanGroupListResp>('/messages/fan-groups/sync');
      return data;
    },
    onSuccess: (data) => {
      qc.setQueryData(['fan-groups'], data);
    },
  });
}

export function useUpdateFanGroupMember() {
  const qc = useQueryClient();
  return useMutation<
    FanGroupListResp,
    Error,
    {
      groupId: string;
      userId: string;
      role?: string;
      muteMinutes?: number;
      kick?: boolean;
      approveRejoin?: boolean;
      rejectRejoin?: boolean;
    }
  >({
    mutationFn: async ({ groupId, userId, ...payload }) => {
      const { data } = await http.patch<FanGroupListResp>(
        `/messages/fan-groups/${encodeURIComponent(groupId)}/members/${encodeURIComponent(userId)}`,
        payload,
      );
      return data;
    },
    onSuccess: (data) => {
      qc.setQueryData(['fan-groups'], data);
      void qc.invalidateQueries({ queryKey: ['joined-fan-groups'] });
      void qc.invalidateQueries({ queryKey: ['fan-group-messages'] });
    },
  });
}
