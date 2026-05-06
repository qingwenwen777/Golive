import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { http } from '@/lib/axios';
import type { Gift } from '@/types/gift';
import type { User } from '@/types/user';
import type { AdminAuditLog } from '@/api/contentModeration';

export type AdminUserRole = 'user' | 'admin' | 'moderator';
export type AdminUserStatus =
  | 'all'
  | 'active'
  | 'banned'
  | 'appeal_pending'
  | 'frozen'
  | 'live_approved';
export type CoinAdjustAction = 'add' | 'deduct' | 'freeze' | 'unfreeze';
export type AdminUnbanAppealStatus = 'pending' | 'reviewing' | 'approved' | 'rejected';

export interface AdminHealthItem {
  key: string;
  label: string;
  status: 'ok' | 'down' | 'unknown' | string;
  detail?: string;
  checked: boolean;
}

export interface AdminOverview {
  onlineRooms: number;
  onlineViewers: number;
  todayNewUsers: number;
  todayRevenueCoins: number;
  health: AdminHealthItem[];
}

export interface AdminSystemRuntimeItem {
  key: string;
  label: string;
  value: string;
  description?: string;
}

export interface AdminSystemSettings {
  registrationPolicy: 'invite_only' | string;
  liveReviewEnabled: boolean;
  contentPolicyLevel: 'standard' | 'strict' | 'relaxed' | string;
  reportReviewTimeoutMinutes: number;
  siteMuteDurations: number[];
  defaultSiteMuteMinutes: number;
  runtime: AdminSystemRuntimeItem[];
  updatedBy?: string;
  updatedAt?: string;
}

export interface UpdateAdminSystemSettingsPayload {
  reportReviewTimeoutMinutes?: number;
  defaultSiteMuteMinutes?: number;
  note?: string;
}

export interface AdminUserListItem extends User {
  email?: string;
  frozenCoins: number;
  banned?: boolean;
  banReason?: string;
  pendingAppeals?: number;
  createdAt: string;
  updatedAt: string;
}

export interface AdminUserStats {
  total: number;
  active: number;
  banned: number;
  admins: number;
  moderators: number;
  pendingAppeals: number;
}

export interface AdminUserListResp {
  items: AdminUserListItem[];
  total: number;
  page: number;
  size: number;
  stats: AdminUserStats;
}

export interface CoinTransaction {
  id: string;
  userId: string;
  type: string;
  amount: number;
  balanceAfter: number;
  title: string;
  description?: string;
  createdAt: string;
}

export interface AdminLiveRecord {
  id: string;
  title: string;
  status: string;
  viewers: number;
  peakViewers: number;
  startedAt: string;
  endedAt?: string;
}

export interface AdminReportRecord {
  id: string;
  targetType: string;
  targetId: string;
  reason: string;
  status: string;
  reporterId: string;
  reporterName: string;
  targetTitle?: string;
  targetText?: string;
  resolutionAction?: string;
  createdAt: string;
  resolvedAt?: string;
}

export interface AdminUnbanAppealRecord {
  id: string;
  userId: string;
  reason: string;
  status: AdminUnbanAppealStatus;
  reviewerId?: string;
  reviewer?: string;
  reviewNote?: string;
  reviewedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AdminUserDetail {
  user: AdminUserListItem;
  coinTransactions: CoinTransaction[];
  liveRecords: AdminLiveRecord[];
  reportRecords: AdminReportRecord[];
  appealRecords: AdminUnbanAppealRecord[];
}

export interface AdminEconomySummary {
  giftCount: number;
  enabledGiftCount: number;
  todayGiftRevenue: number;
  todaySuperChatRevenue: number;
  todayTopupCoins: number;
  todaySpendCoins: number;
  todayBetTurnover: number;
  totalCoinBalance: number;
  totalFrozenCoins: number;
  openBetRounds: number;
  unsettledBetRounds: number;
  pendingSettlementCoins: number;
}

export interface AdminGiftStats {
  total: number;
  enabled: number;
  disabled: number;
  catalogValue: number;
}

export interface AdminGiftListResp {
  items: Gift[];
  stats: AdminGiftStats;
}

export type AdminOrderType = 'all' | 'gift' | 'super_chat' | 'bet';

export interface AdminEconomyOrderRecord {
  type: 'gift' | 'super_chat' | 'bet';
  orderId: string;
  userId: string;
  userName: string;
  roomId?: string;
  roomTitle?: string;
  itemId: string;
  itemName: string;
  count: number;
  amount: number;
  status: string;
  failReason?: string;
  option?: string;
  result?: string;
  createdAt: string;
}

export interface AdminEconomyOrderResp {
  items: AdminEconomyOrderRecord[];
  total: number;
  page: number;
  size: number;
}

export interface AdminCoinStats {
  totalTopupCoins: number;
  totalSpendCoins: number;
  totalBalanceCoins: number;
  totalFrozenCoins: number;
  todayTopupCoins: number;
  todaySpendCoins: number;
  userCount: number;
}

export interface AdminCoinRecord {
  id: string;
  userId: string;
  userName: string;
  type: string;
  amount: number;
  balanceAfter: number;
  title: string;
  description?: string;
  sourceType?: string;
  sourceId?: string;
  roomId?: string;
  counterpartyId?: string;
  createdAt: string;
}

export interface AdminCoinLedgerResp {
  items: AdminCoinRecord[];
  total: number;
  page: number;
  size: number;
  stats: AdminCoinStats;
}

export interface AdminBetStats {
  total: number;
  open: number;
  closed: number;
  settled: number;
  cancelled: number;
  lockedCoins: number;
}

export interface AdminBetRoundRecord {
  id: string;
  roomId: string;
  roomTitle?: string;
  ownerId: string;
  ownerName: string;
  question: string;
  amount: number;
  status: string;
  winningOption?: string;
  closeAt: string;
  settledAt?: string;
  createdAt: string;
  updatedAt: string;
  wagerCount: number;
  totalPool: number;
  winCount: number;
  loseCount: number;
  winPool: number;
  losePool: number;
}

export interface AdminBetListResp {
  items: AdminBetRoundRecord[];
  total: number;
  page: number;
  size: number;
  stats: AdminBetStats;
}

export type AdminReportPeriod = 'day' | 'week' | 'month';

export interface AdminRevenueReportRow {
  periodStart: string;
  periodEnd: string;
  topupCoins: number;
  giftCoins: number;
  superChatCoins: number;
  revenueCoins: number;
  betWagerCoins: number;
  betPayoutCoins: number;
  betRefundCoins: number;
  netBetCoins: number;
}

export interface AdminRevenueReportResp {
  period: AdminReportPeriod;
  items: AdminRevenueReportRow[];
}

export function useAdminOverview(enabled = true) {
  return useQuery<AdminOverview, Error>({
    queryKey: ['admin-overview'],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<AdminOverview>('/rooms/admin/dashboard', { signal });
      return data;
    },
    enabled,
    staleTime: 8_000,
    retry: 1,
  });
}

export function useAdminSystemSettings(enabled = true) {
  return useQuery<AdminSystemSettings, Error>({
    queryKey: ['admin-system-settings'],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<AdminSystemSettings>('/rooms/admin/system-settings', {
        signal,
      });
      return data;
    },
    enabled,
    staleTime: 10_000,
    retry: 1,
  });
}

export function useUpdateAdminSystemSettings() {
  const qc = useQueryClient();
  return useMutation<AdminSystemSettings, Error, UpdateAdminSystemSettingsPayload>({
    mutationFn: async (payload) => {
      const { data } = await http.patch<AdminSystemSettings>(
        '/rooms/admin/system-settings',
        payload,
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-system-settings'] });
      void qc.invalidateQueries({ queryKey: ['admin-audit-logs'] });
    },
  });
}

export function useAdminUsers(
  params: {
    q?: string;
    role?: string;
    status?: string;
    page?: number;
    size?: number;
  },
  enabled = true,
) {
  const page = params.page ?? 1;
  const size = params.size ?? 20;
  return useQuery<AdminUserListResp, Error>({
    queryKey: ['admin-users', { ...params, page, size }],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<AdminUserListResp>('/admin/users', {
        params: { ...params, page, size },
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

export function useAdminUserDetail(id: string, enabled = true) {
  return useQuery<AdminUserDetail, Error>({
    queryKey: ['admin-user-detail', id],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<AdminUserDetail>(`/admin/users/${encodeURIComponent(id)}`, {
        signal,
      });
      return data;
    },
    enabled: enabled && !!id,
    staleTime: 8_000,
    retry: 1,
  });
}

function useAdminUserMutation<TPayload extends object>(
  fn: (payload: TPayload) => Promise<unknown>,
) {
  const qc = useQueryClient();
  return useMutation<unknown, Error, TPayload>({
    mutationFn: fn,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-users'] });
      void qc.invalidateQueries({ queryKey: ['admin-user-detail'] });
      void qc.invalidateQueries({ queryKey: ['admin-overview'] });
    },
  });
}

export function useAdminUpdateUserProfile(id: string) {
  return useAdminUserMutation<{ username?: string; displayName?: string }>(async (payload) => {
    const { data } = await http.patch(`/admin/users/${encodeURIComponent(id)}/profile`, payload);
    return data;
  });
}

export function useAdminUpdateUserEmail(id: string) {
  return useAdminUserMutation<{ email: string }>(async (payload) => {
    const { data } = await http.patch(`/admin/users/${encodeURIComponent(id)}/email`, payload);
    return data;
  });
}

export function useAdminUpdateUserPassword(id: string) {
  return useAdminUserMutation<{ password: string }>(async (payload) => {
    const { data } = await http.patch(`/admin/users/${encodeURIComponent(id)}/password`, payload);
    return data;
  });
}

export function useAdminUpdateUserRole(id: string) {
  return useAdminUserMutation<{ role: AdminUserRole }>(async (payload) => {
    const { data } = await http.patch(`/admin/users/${encodeURIComponent(id)}/role`, payload);
    return data;
  });
}

export function useAdminSetUserBan(id: string) {
  return useAdminUserMutation<{ banned: boolean; reason?: string }>(async (payload) => {
    const { data } = await http.patch(`/admin/users/${encodeURIComponent(id)}/ban`, payload);
    return data;
  });
}

export function useAdminReviewUnbanAppeal(id: string, appealId: string) {
  return useAdminUserMutation<{ status: 'reviewing' | 'approved' | 'rejected'; note?: string }>(
    async (payload) => {
      const { data } = await http.patch(
        `/admin/users/${encodeURIComponent(id)}/unban-appeals/${encodeURIComponent(appealId)}`,
        payload,
      );
      return data;
    },
  );
}

export function useAdminAdjustUserCoins(id: string) {
  return useAdminUserMutation<{
    action: CoinAdjustAction;
    amount: number;
    note?: string;
  }>(async (payload) => {
    const { data } = await http.post(`/admin/users/${encodeURIComponent(id)}/coins`, payload);
    return data;
  });
}

export function useAdminEconomySummary(enabled = true) {
  return useQuery<AdminEconomySummary, Error>({
    queryKey: ['admin-economy-summary'],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<AdminEconomySummary>('/admin/economy/summary', { signal });
      return data;
    },
    enabled,
    staleTime: 8_000,
    retry: 1,
  });
}

export function useAdminEconomyGifts(enabled = true) {
  return useQuery<AdminGiftListResp, Error>({
    queryKey: ['admin-economy-gifts'],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<AdminGiftListResp>('/admin/economy/gifts', { signal });
      return data;
    },
    enabled,
    staleTime: 10_000,
    retry: 1,
  });
}

export function useAdminUpdateGift() {
  const qc = useQueryClient();
  return useMutation<
    Gift,
    Error,
    {
      id: string;
      priceCoin?: number;
      enabled?: boolean;
    }
  >({
    mutationFn: async ({ id, ...payload }) => {
      const { data } = await http.patch<Gift>(
        `/admin/economy/gifts/${encodeURIComponent(id)}`,
        payload,
      );
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-economy-summary'] });
      void qc.invalidateQueries({ queryKey: ['admin-economy-gifts'] });
      void qc.invalidateQueries({ queryKey: ['gifts'] });
    },
  });
}

export function useAdminEconomyOrders(
  params: {
    type?: AdminOrderType;
    status?: string;
    q?: string;
    page?: number;
    size?: number;
  },
  enabled = true,
) {
  const page = params.page ?? 1;
  const size = params.size ?? 20;
  return useQuery<AdminEconomyOrderResp, Error>({
    queryKey: ['admin-economy-orders', { ...params, page, size }],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<AdminEconomyOrderResp>('/admin/economy/orders', {
        params: { ...params, page, size },
        signal,
      });
      return data;
    },
    enabled,
    staleTime: 8_000,
    placeholderData: keepPreviousData,
    retry: 1,
  });
}

export function useAdminCoinLedger(
  params: {
    type?: string;
    q?: string;
    page?: number;
    size?: number;
  },
  enabled = true,
) {
  const page = params.page ?? 1;
  const size = params.size ?? 20;
  return useQuery<AdminCoinLedgerResp, Error>({
    queryKey: ['admin-economy-coins', { ...params, page, size }],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<AdminCoinLedgerResp>('/admin/economy/coins', {
        params: { ...params, page, size },
        signal,
      });
      return data;
    },
    enabled,
    staleTime: 8_000,
    placeholderData: keepPreviousData,
    retry: 1,
  });
}

export function useAdminBetRounds(
  params: {
    status?: string;
    q?: string;
    page?: number;
    size?: number;
  },
  enabled = true,
) {
  const page = params.page ?? 1;
  const size = params.size ?? 20;
  return useQuery<AdminBetListResp, Error>({
    queryKey: ['admin-economy-bets', { ...params, page, size }],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<AdminBetListResp>('/admin/economy/bets', {
        params: { ...params, page, size },
        signal,
      });
      return data;
    },
    enabled,
    staleTime: 8_000,
    placeholderData: keepPreviousData,
    retry: 1,
  });
}

function useAdminBetMutation<TPayload extends object>(
  fn: (payload: TPayload) => Promise<AdminBetRoundRecord>,
) {
  const qc = useQueryClient();
  return useMutation<AdminBetRoundRecord, Error, TPayload>({
    mutationFn: fn,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin-economy-summary'] });
      void qc.invalidateQueries({ queryKey: ['admin-economy-bets'] });
      void qc.invalidateQueries({ queryKey: ['admin-economy-orders'] });
      void qc.invalidateQueries({ queryKey: ['admin-economy-coins'] });
      void qc.invalidateQueries({ queryKey: ['admin-economy-reports'] });
    },
  });
}

export function useAdminSettleBet() {
  return useAdminBetMutation<{ id: string; option: 'win' | 'lose' }>(async ({ id, option }) => {
    const { data } = await http.post<AdminBetRoundRecord>(
      `/admin/economy/bets/${encodeURIComponent(id)}/settle`,
      { option },
    );
    return data;
  });
}

export function useAdminCancelBet() {
  return useAdminBetMutation<{ id: string }>(async ({ id }) => {
    const { data } = await http.post<AdminBetRoundRecord>(
      `/admin/economy/bets/${encodeURIComponent(id)}/cancel`,
    );
    return data;
  });
}

export function useAdminRevenueReports(
  params: {
    period?: AdminReportPeriod;
    limit?: number;
  },
  enabled = true,
) {
  const period = params.period ?? 'day';
  const limit = params.limit ?? (period === 'day' ? 14 : period === 'week' ? 8 : 6);
  return useQuery<AdminRevenueReportResp, Error>({
    queryKey: ['admin-economy-reports', { period, limit }],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<AdminRevenueReportResp>('/admin/economy/reports', {
        params: { period, limit },
        signal,
      });
      return data;
    },
    enabled,
    staleTime: 15_000,
    retry: 1,
  });
}

export type { AdminAuditLog };
