import { useMemo, useState, type ChangeEvent, type ComponentType } from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { Link, Navigate, NavLink, useLocation, useNavigate } from 'react-router-dom';
import {
  Ban,
  Check,
  ChevronDown,
  Clipboard,
  Coins,
  Eye,
  FileCheck2,
  FileText,
  Gauge,
  History,
  ListFilter,
  LogOut,
  Pencil,
  Plus,
  RefreshCw,
  Save,
  Search,
  Settings,
  Shield,
  SlidersHorizontal,
  Ticket,
  Trash2,
  Upload,
  Users,
  Video,
  X,
  type LucideProps,
} from 'lucide-react';
import { toast } from 'sonner';
import {
  useAdminCreatorApplications,
  useAdminInviteCodes,
  useAdminLiveCreators,
  useCreateInviteCode,
  useDeleteInviteCode,
  useReviewCreatorApplication,
  useUpdateLivePermission,
  type AdminInviteCode,
  type CreatorApplication,
  type LiveCreator,
} from '@/api/creator';
import {
  useAdminBlockedWords,
  useAdminAuditLogs,
  useAdminReportDetail,
  useAdminReports,
  useCreateBlockedWord,
  useDeleteBlockedWord,
  useImportBlockedWords,
  useUpdateAdminReport,
  useUpdateBlockedWord,
  type AdminAuditCategory,
  type BlockedWord,
  type ReportAction,
} from '@/api/contentModeration';
import { Avatar } from '@/components/Avatar';
import { GoLiveLogo } from '@/components/Logo';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useAuthStore } from '@/stores/useAuthStore';

type AdminModule = 'dashboard' | 'users' | 'creators' | 'content' | 'economy' | 'system' | 'logs';
type AdminIcon = ComponentType<LucideProps>;
type Translate = ReturnType<typeof useTranslation>['t'];

interface AdminModuleDef {
  key: AdminModule;
  path: string;
  icon: AdminIcon;
}

interface AdminMetrics {
  pendingApplications: number;
  approvedCreators: number;
  availableInvites: number;
  totalApplications: number;
  totalInvites: number;
}

const ADMIN_MODULES: AdminModuleDef[] = [
  { key: 'dashboard', path: '/admin/dashboard', icon: Gauge },
  { key: 'users', path: '/admin/users', icon: Users },
  { key: 'creators', path: '/admin/creators', icon: Video },
  { key: 'content', path: '/admin/content', icon: FileCheck2 },
  { key: 'economy', path: '/admin/economy', icon: Coins },
  { key: 'system', path: '/admin/system', icon: SlidersHorizontal },
  { key: 'logs', path: '/admin/logs', icon: History },
];

const MODULE_DEFAULTS: Record<AdminModule, { label: string; subtitle: string }> = {
  dashboard: { label: 'Dashboard', subtitle: 'Core status' },
  users: { label: 'Users', subtitle: 'Accounts and invite codes' },
  creators: { label: 'Creators', subtitle: 'Applications and live permissions' },
  content: { label: 'Content review', subtitle: 'Live rooms and posts' },
  economy: { label: 'Economy', subtitle: 'Coins and revenue' },
  system: { label: 'System config', subtitle: 'Policies and switches' },
  logs: { label: 'Operation logs', subtitle: 'Audit trail' },
};

function isAdminModule(value: string): value is AdminModule {
  return ADMIN_MODULES.some((module) => module.key === value);
}

function getModuleFromPath(pathname: string): AdminModule | undefined {
  const section = pathname.replace(/^\/admin\/?/, '').split('/')[0] || 'dashboard';
  if (section === 'applications') return 'creators';
  if (isAdminModule(section)) return section;
  return undefined;
}

function moduleText(t: Translate, module: AdminModule) {
  const defaults = MODULE_DEFAULTS[module];
  return {
    label: t(`admin.modules.${module}.label`, { defaultValue: defaults.label }),
    subtitle: t(`admin.modules.${module}.subtitle`, { defaultValue: defaults.subtitle }),
  };
}

function statusText(status: CreatorApplication['status'], t: Translate) {
  if (status === 'pending') return t('admin.status.pending', { defaultValue: 'Pending' });
  if (status === 'approved') return t('admin.status.approved', { defaultValue: 'Approved' });
  if (status === 'rejected') return t('admin.status.rejected', { defaultValue: 'Rejected' });
  return t('admin.status.none', { defaultValue: 'None' });
}

function formatDate(value?: string) {
  if (!value) return '-';
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value));
}

export default function AdminPage() {
  const { t } = useTranslation('pages');
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const user = useAuthStore((s) => s.user);
  const isAdmin = user?.role === 'admin';
  const currentModule = getModuleFromPath(location.pathname);
  const currentText = moduleText(t, currentModule ?? 'dashboard');

  const apps = useAdminCreatorApplications(isAdmin);
  const creators = useAdminLiveCreators(isAdmin);
  const invites = useAdminInviteCodes(isAdmin);
  const approve = useReviewCreatorApplication('approve');
  const reject = useReviewCreatorApplication('reject');
  const updatePermission = useUpdateLivePermission();
  const createInvite = useCreateInviteCode();
  const deleteInvite = useDeleteInviteCode();

  const appItems = apps.data?.items ?? [];
  const creatorItems = creators.data?.items ?? [];
  const inviteItems = invites.data?.items ?? [];
  const metrics = useMemo<AdminMetrics>(
    () => ({
      pendingApplications: appItems.filter((item) => item.status === 'pending').length,
      approvedCreators: creatorItems.length,
      availableInvites: inviteItems.filter((item) => !item.used).length,
      totalApplications: appItems.length,
      totalInvites: inviteItems.length,
    }),
    [appItems, creatorItems.length, inviteItems],
  );

  if (!currentModule) {
    return <Navigate to="/admin/dashboard" replace />;
  }

  if (!isAdmin) {
    return (
      <div className="gl-page gl-admin-page">
        <section className="gl-admin-denied">
          <Shield size={34} />
          <h1>{t('admin.denied.title', { defaultValue: 'No admin access' })}</h1>
          <p>
            {t('admin.denied.body', {
              defaultValue: 'This area is only available to administrators.',
            })}
          </p>
          <button type="button" className="gl-retry-btn" onClick={() => navigate('/')}>
            {t('admin.denied.action', { defaultValue: 'Back to home' })}
          </button>
        </section>
      </div>
    );
  }

  const refresh = () => {
    void queryClient.invalidateQueries();
    void apps.refetch();
    void creators.refetch();
    void invites.refetch();
    toast.success(t('admin.refreshDone', { defaultValue: 'Admin data refreshed.' }));
  };

  return (
    <div className="gl-page gl-admin-page">
      <div className="gl-admin-workspace">
        <AdminNav currentModule={currentModule} metrics={metrics} />
        <main className="gl-admin-stage">
          <header className="gl-admin-topbar">
            <div>
              <div className="gl-admin-kicker">
                <Shield size={16} />
                {t('admin.kicker', { defaultValue: 'Admin' })}
              </div>
              <h1>{currentText.label}</h1>
              <p>{currentText.subtitle}</p>
            </div>
            <div className="gl-admin-topbar-actions">
              <button type="button" className="gl-secondary-btn gl-admin-refresh" onClick={refresh}>
                <RefreshCw size={16} />
                {t('admin.refresh', { defaultValue: 'Refresh' })}
              </button>
              <button
                type="button"
                className="gl-secondary-btn gl-admin-exit"
                onClick={() => navigate('/')}
              >
                <LogOut size={16} />
                {t('admin.exit', { defaultValue: 'Exit admin' })}
              </button>
            </div>
          </header>

          {currentModule === 'dashboard' && (
            <DashboardPage
              metrics={metrics}
              loading={apps.isLoading || creators.isLoading || invites.isLoading}
            />
          )}
          {currentModule === 'users' && (
            <UsersPage
              inviteItems={inviteItems}
              invitesLoading={invites.isLoading}
              invitesError={invites.isError}
              createBusy={createInvite.isPending}
              deleteBusy={deleteInvite.isPending}
              onCreateInvite={() => {
                createInvite.mutate(undefined, {
                  onSuccess: ({ inviteCode }) => {
                    toast.success(
                      t('admin.invites.created', {
                        code: inviteCode.code,
                        defaultValue: 'Invite code {{code}} created.',
                      }),
                    );
                  },
                  onError: (err) =>
                    toast.error(
                      err.message ||
                        t('admin.invites.createFailed', {
                          defaultValue: 'Could not create invite code.',
                        }),
                    ),
                });
              }}
              onDeleteInvite={(item) => {
                const ok = window.confirm(
                  t('admin.invites.confirmDelete', {
                    code: item.code,
                    defaultValue: 'Delete unused invite code {{code}}?',
                  }),
                );
                if (!ok) return;
                deleteInvite.mutate(
                  { id: item.id },
                  {
                    onSuccess: () =>
                      toast.success(
                        t('admin.invites.deleted', {
                          defaultValue: 'Invite code deleted.',
                        }),
                      ),
                    onError: (err) =>
                      toast.error(
                        err.message ||
                          t('admin.invites.deleteFailed', {
                            defaultValue: 'Could not delete invite code.',
                          }),
                      ),
                  },
                );
              }}
            />
          )}
          {currentModule === 'creators' && (
            <CreatorsPage
              applications={appItems}
              creators={creatorItems}
              applicationsLoading={apps.isLoading}
              creatorsLoading={creators.isLoading}
              applicationsError={apps.isError}
              creatorsError={creators.isError}
              approveBusy={approve.isPending}
              rejectBusy={reject.isPending}
              permissionBusy={updatePermission.isPending}
              onApprove={(id) => {
                approve.mutate(
                  { id },
                  {
                    onSuccess: () =>
                      toast.success(
                        t('admin.applications.approved', {
                          defaultValue: 'Application approved.',
                        }),
                      ),
                    onError: (err) =>
                      toast.error(
                        err.message ||
                          t('admin.applications.reviewFailed', {
                            defaultValue: 'Review failed.',
                          }),
                      ),
                  },
                );
              }}
              onReject={(id, reason) => {
                reject.mutate(
                  { id, reason },
                  {
                    onSuccess: () =>
                      toast.success(
                        t('admin.applications.rejected', {
                          defaultValue: 'Application rejected.',
                        }),
                      ),
                    onError: (err) =>
                      toast.error(
                        err.message ||
                          t('admin.applications.reviewFailed', {
                            defaultValue: 'Review failed.',
                          }),
                      ),
                  },
                );
              }}
              onDisableCreator={(item) => {
                const ok = window.confirm(
                  t('admin.permissions.confirmDisable', {
                    name: item.displayName || item.username,
                    defaultValue: 'Disable live permission for {{name}}?',
                  }),
                );
                if (!ok) return;
                updatePermission.mutate(
                  { id: item.id, status: 'rejected' },
                  {
                    onSuccess: () =>
                      toast.success(
                        t('admin.permissions.disabled', {
                          defaultValue: 'Live permission disabled.',
                        }),
                      ),
                    onError: (err) =>
                      toast.error(
                        err.message ||
                          t('admin.permissions.disableFailed', {
                            defaultValue: 'Could not update permission.',
                          }),
                      ),
                  },
                );
              }}
            />
          )}
          {currentModule === 'content' && <ContentPage />}
          {currentModule === 'economy' && <ScaffoldModulePage module="economy" />}
          {currentModule === 'system' && <ScaffoldModulePage module="system" />}
          {currentModule === 'logs' && <LogsPage />}
        </main>
      </div>
    </div>
  );
}

function AdminNav({
  currentModule,
  metrics,
}: {
  currentModule: AdminModule;
  metrics: AdminMetrics;
}) {
  const { t } = useTranslation('pages');
  const badgeFor = (module: AdminModule) => {
    if (module === 'creators' && metrics.pendingApplications > 0)
      return metrics.pendingApplications;
    if (module === 'users' && metrics.availableInvites > 0) return metrics.availableInvites;
    return undefined;
  };

  return (
    <aside
      className="gl-admin-nav"
      aria-label={t('admin.nav.aria', { defaultValue: 'Admin modules' })}
    >
      <div className="gl-admin-nav-brand">
        <span className="gl-admin-mark" aria-hidden="true">
          <GoLiveLogo variant="mark" height={34} />
        </span>
        <div>
          <strong>GoLive Admin</strong>
          <span>{t('admin.brand.controlCenter', { defaultValue: 'Control Center' })}</span>
        </div>
      </div>
      <nav className="gl-admin-nav-list">
        {ADMIN_MODULES.map((module) => {
          const Icon = module.icon;
          const badge = badgeFor(module.key);
          const copy = moduleText(t, module.key);
          return (
            <NavLink
              key={module.key}
              to={module.path}
              className={currentModule === module.key ? 'is-active' : undefined}
            >
              <Icon size={20} />
              <span>
                <strong>{copy.label}</strong>
                <small>{copy.subtitle}</small>
              </span>
              {badge ? <em>{badge}</em> : null}
            </NavLink>
          );
        })}
      </nav>
    </aside>
  );
}

function DashboardPage({ metrics, loading }: { metrics: AdminMetrics; loading: boolean }) {
  const { t } = useTranslation('pages');
  return (
    <div className="gl-admin-section-stack">
      <section
        className="gl-admin-kpi-grid"
        aria-label={t('admin.dashboard.aria', { defaultValue: 'Admin overview' })}
      >
        <AdminKpi
          icon={FileCheck2}
          label={t('admin.dashboard.kpis.pendingApplications', {
            defaultValue: 'Pending live applications',
          })}
          value={loading ? '-' : metrics.pendingApplications}
          tone="red"
        />
        <AdminKpi
          icon={Video}
          label={t('admin.dashboard.kpis.approvedCreators', { defaultValue: 'Approved creators' })}
          value={loading ? '-' : metrics.approvedCreators}
        />
        <AdminKpi
          icon={Ticket}
          label={t('admin.dashboard.kpis.availableInvites', {
            defaultValue: 'Available invite codes',
          })}
          value={loading ? '-' : metrics.availableInvites}
        />
        <AdminKpi
          icon={ListFilter}
          label={t('admin.dashboard.kpis.pendingContent', { defaultValue: 'Pending content' })}
          value="0"
        />
      </section>

      <section className="gl-admin-panel">
        <div className="gl-admin-panel-head">
          <div>
            <span>{t('admin.dashboard.modules.eyebrow', { defaultValue: 'Modules' })}</span>
            <h2>{t('admin.dashboard.modules.title', { defaultValue: 'Admin modules' })}</h2>
          </div>
        </div>
        <div className="gl-admin-module-grid">
          {ADMIN_MODULES.filter((module) => module.key !== 'dashboard').map((module) => {
            const Icon = module.icon;
            const copy = moduleText(t, module.key);
            return (
              <Link className="gl-admin-module-card" to={module.path} key={module.key}>
                <Icon size={22} />
                <strong>{copy.label}</strong>
                <span>{copy.subtitle}</span>
              </Link>
            );
          })}
        </div>
      </section>

      <section className="gl-admin-panel">
        <div className="gl-admin-panel-head">
          <div>
            <span>{t('admin.dashboard.focus.eyebrow', { defaultValue: 'Operations' })}</span>
            <h2>{t('admin.dashboard.focus.title', { defaultValue: 'Current focus' })}</h2>
          </div>
        </div>
        <div className="gl-admin-detail-list">
          <AdminDetailRow
            label={t('admin.dashboard.focus.applicationReview', {
              defaultValue: 'Live application review',
            })}
            value={t('admin.dashboard.focus.applicationReviewValue', {
              pending: metrics.pendingApplications,
              total: metrics.totalApplications,
              defaultValue: '{{pending}} pending / {{total}} total applications',
            })}
          />
          <AdminDetailRow
            label={t('admin.dashboard.focus.permissionManagement', {
              defaultValue: 'Live permission management',
            })}
            value={t('admin.dashboard.focus.permissionManagementValue', {
              count: metrics.approvedCreators,
              defaultValue: '{{count}} creators can go live',
            })}
          />
          <AdminDetailRow
            label={t('admin.dashboard.focus.inviteManagement', {
              defaultValue: 'Invite code management',
            })}
            value={t('admin.dashboard.focus.inviteManagementValue', {
              available: metrics.availableInvites,
              total: metrics.totalInvites,
              defaultValue: '{{available}} available / {{total}} total',
            })}
          />
        </div>
      </section>
    </div>
  );
}

function UsersPage({
  inviteItems,
  invitesLoading,
  invitesError,
  createBusy,
  deleteBusy,
  onCreateInvite,
  onDeleteInvite,
}: {
  inviteItems: AdminInviteCode[];
  invitesLoading: boolean;
  invitesError: boolean;
  createBusy: boolean;
  deleteBusy: boolean;
  onCreateInvite: () => void;
  onDeleteInvite: (item: AdminInviteCode) => void;
}) {
  const { t } = useTranslation('pages');
  return (
    <div className="gl-admin-section-stack">
      <section
        className="gl-admin-kpi-grid"
        aria-label={t('admin.users.aria', { defaultValue: 'User management summary' })}
      >
        <AdminKpi
          icon={Ticket}
          label={t('admin.users.kpis.availableInvites', { defaultValue: 'Available invite codes' })}
          value={inviteItems.filter((item) => !item.used).length}
        />
        <AdminKpi
          icon={Clipboard}
          label={t('admin.users.kpis.usedInvites', { defaultValue: 'Used invite codes' })}
          value={inviteItems.filter((item) => item.used).length}
        />
        <AdminKpi
          icon={Users}
          label={t('admin.users.kpis.accountDirectory', { defaultValue: 'Account directory' })}
          value="-"
        />
        <AdminKpi
          icon={Shield}
          label={t('admin.users.kpis.admins', { defaultValue: 'Admins' })}
          value="-"
        />
      </section>
      <InvitesPanel
        items={inviteItems}
        loading={invitesLoading}
        error={invitesError}
        busy={createBusy}
        deleteBusy={deleteBusy}
        onCreate={onCreateInvite}
        onDelete={onDeleteInvite}
      />
      <StaticOperationsPanel
        eyebrow={t('admin.users.framework.eyebrow', { defaultValue: 'Users' })}
        title={t('admin.users.framework.title', { defaultValue: 'User management framework' })}
        rows={[
          [
            t('admin.users.framework.accounts', { defaultValue: 'Account list' }),
            t('admin.users.framework.accountsSub', { defaultValue: 'Profiles, roles, and status' }),
          ],
          [
            t('admin.users.framework.groups', { defaultValue: 'Permission groups' }),
            t('admin.users.framework.groupsSub', { defaultValue: 'Regular users and admins' }),
          ],
          [
            t('admin.users.framework.risk', { defaultValue: 'Risk status' }),
            t('admin.users.framework.riskSub', {
              defaultValue: 'Registration, login, and invite redemption',
            }),
          ],
        ]}
      />
    </div>
  );
}

function CreatorsPage({
  applications,
  creators,
  applicationsLoading,
  creatorsLoading,
  applicationsError,
  creatorsError,
  approveBusy,
  rejectBusy,
  permissionBusy,
  onApprove,
  onReject,
  onDisableCreator,
}: {
  applications: CreatorApplication[];
  creators: LiveCreator[];
  applicationsLoading: boolean;
  creatorsLoading: boolean;
  applicationsError: boolean;
  creatorsError: boolean;
  approveBusy: boolean;
  rejectBusy: boolean;
  permissionBusy: boolean;
  onApprove: (id: string) => void;
  onReject: (id: string, reason: string) => void;
  onDisableCreator: (item: LiveCreator) => void;
}) {
  const { t } = useTranslation('pages');
  return (
    <div className="gl-admin-section-stack">
      <section
        className="gl-admin-kpi-grid"
        aria-label={t('admin.creators.aria', { defaultValue: 'Creator management summary' })}
      >
        <AdminKpi
          icon={FileCheck2}
          label={t('admin.creators.kpis.pendingApplications', {
            defaultValue: 'Pending applications',
          })}
          value={applications.filter((item) => item.status === 'pending').length}
          tone="red"
        />
        <AdminKpi
          icon={Video}
          label={t('admin.creators.kpis.liveCreators', {
            defaultValue: 'Creators who can go live',
          })}
          value={creators.length}
        />
        <AdminKpi
          icon={Check}
          label={t('admin.creators.kpis.approvedApplications', {
            defaultValue: 'Approved applications',
          })}
          value={applications.filter((item) => item.status === 'approved').length}
        />
        <AdminKpi
          icon={X}
          label={t('admin.creators.kpis.rejectedApplications', {
            defaultValue: 'Rejected applications',
          })}
          value={applications.filter((item) => item.status === 'rejected').length}
        />
      </section>
      <div className="gl-admin-split-grid">
        <ApplicationsPanel
          items={applications}
          loading={applicationsLoading}
          error={applicationsError}
          approveBusy={approveBusy}
          rejectBusy={rejectBusy}
          onApprove={onApprove}
          onReject={onReject}
        />
        <PermissionPanel
          items={creators}
          loading={creatorsLoading}
          error={creatorsError}
          busy={permissionBusy}
          onDisable={onDisableCreator}
        />
      </div>
    </div>
  );
}

function ContentPage() {
  const { t } = useTranslation('pages');
  const [status, setStatus] = useState('all');
  const [targetType, setTargetType] = useState('all');
  const [reason, setReason] = useState('all');
  const [query, setQuery] = useState('');
  const [selectedId, setSelectedId] = useState('');
  const [resolutionNote, setResolutionNote] = useState('');
  const [detailOpen, setDetailOpen] = useState(false);
  const [selectedActions, setSelectedActions] = useState<ReportAction[]>([]);
  const [muteDuration, setMuteDuration] = useState(1440);
  const [word, setWord] = useState('');
  const [wordNote, setWordNote] = useState('');
  const [editingWord, setEditingWord] = useState<BlockedWord | null>(null);

  const reports = useAdminReports({
    status,
    targetType,
    reason,
    q: query.trim() || undefined,
    page: 1,
    size: 20,
  });
  const updateReport = useUpdateAdminReport();
  const blockedWords = useAdminBlockedWords(1, 50);
  const createWord = useCreateBlockedWord();
  const updateWord = useUpdateBlockedWord();
  const deleteWord = useDeleteBlockedWord();
  const importWords = useImportBlockedWords();

  const items = reports.data?.items ?? [];
  const selected = selectedId ? (items.find((item) => item.id === selectedId) ?? null) : null;
  const reportDetail = useAdminReportDetail(selectedId, detailOpen && Boolean(selectedId));
  const detail = reportDetail.data ?? selected;
  const stats = reports.data?.stats ?? { pending: 0, reviewing: 0, today: 0, total: 0 };
  const detailClosed = detail ? isReportClosed(detail.status) : false;
  const detailActions = detail
    ? reportActionsForTarget(detail.targetType).filter((action) => !isExclusiveReportAction(action))
    : [];

  const toggleReportAction = (action: ReportAction) => {
    setSelectedActions((current) =>
      current.includes(action) ? current.filter((item) => item !== action) : [...current, action],
    );
  };

  const submitReportActions = (nextActions = selectedActions) => {
    if (!detail) return;
    if (isReportClosed(detail.status)) {
      toast.info(
        t('admin.content.reports.alreadyHandled', {
          defaultValue: 'This report has already been handled.',
        }),
      );
      return;
    }
    const actions = uniqueReportActions(nextActions);
    if (actions.length === 0) {
      toast.error(
        t('admin.content.reports.actionRequired', {
          defaultValue: 'Select at least one action before confirming.',
        }),
      );
      return;
    }
    if (actions.length > 1 && actions.some((action) => isExclusiveReportAction(action))) {
      toast.error(
        t('admin.content.reports.exclusiveAction', {
          defaultValue: 'Dismiss and review actions cannot be combined with penalties.',
        }),
      );
      return;
    }
    updateReport.mutate(
      {
        id: detail.id,
        actions,
        note: resolutionNote,
        durationMinutes: actions.includes('site_mute') ? muteDuration : undefined,
      },
      {
        onSuccess: (updated) => {
          setSelectedId(updated.id);
          setResolutionNote(updated.resolutionNote ?? '');
          setSelectedActions([]);
          setDetailOpen(false);
          toast.success(
            t('admin.content.reports.updated', { defaultValue: 'Report status updated.' }),
          );
        },
        onError: (err) => toast.error(reportMutationErrorMessage(err, t)),
      },
    );
  };

  const handleBlockedWordImport = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    try {
      const text = await file.text();
      const itemsToImport = parseBlockedWordImport(text);
      if (itemsToImport.length === 0) {
        toast.error(
          t('admin.content.words.importEmpty', {
            defaultValue: 'No valid blocked words found in the file.',
          }),
        );
        return;
      }
      importWords.mutate(
        { items: itemsToImport },
        {
          onSuccess: (result) => {
            toast.success(
              t('admin.content.words.imported', {
                created: result.created,
                skipped: result.skipped,
                defaultValue: 'Imported {{created}} words, skipped {{skipped}}.',
              }),
            );
          },
          onError: (err) =>
            toast.error(
              err.message ||
                t('admin.content.words.importFailed', {
                  defaultValue: 'Could not import blocked words.',
                }),
            ),
        },
      );
    } catch {
      toast.error(
        t('admin.content.words.importFailed', {
          defaultValue: 'Could not import blocked words.',
        }),
      );
    }
  };

  const submitWord = () => {
    const trimmed = word.trim();
    if (!trimmed) {
      toast.error(
        t('admin.content.words.wordRequired', { defaultValue: 'Blocked word is required.' }),
      );
      return;
    }
    createWord.mutate(
      { word: trimmed, note: wordNote.trim() },
      {
        onSuccess: () => {
          setWord('');
          setWordNote('');
          toast.success(t('admin.content.words.created', { defaultValue: 'Blocked word added.' }));
        },
        onError: (err) =>
          toast.error(
            err.message ||
              t('admin.content.words.createFailed', {
                defaultValue: 'Could not add blocked word.',
              }),
          ),
      },
    );
  };

  const saveEditingWord = () => {
    if (!editingWord) return;
    updateWord.mutate(
      {
        id: editingWord.id,
        word: editingWord.word,
        note: editingWord.note ?? '',
        enabled: editingWord.enabled,
      },
      {
        onSuccess: () => {
          setEditingWord(null);
          toast.success(
            t('admin.content.words.updated', { defaultValue: 'Blocked word updated.' }),
          );
        },
        onError: (err) =>
          toast.error(
            err.message ||
              t('admin.content.words.updateFailed', {
                defaultValue: 'Could not update blocked word.',
              }),
          ),
      },
    );
  };

  return (
    <div className="gl-admin-section-stack">
      <section
        className="gl-admin-kpi-grid"
        aria-label={t('admin.content.aria', { defaultValue: 'Content moderation summary' })}
      >
        <AdminKpi
          icon={FileCheck2}
          label={t('admin.content.kpis.pending', { defaultValue: 'Pending reports' })}
          value={stats.pending}
          tone="red"
        />
        <AdminKpi
          icon={ListFilter}
          label={t('admin.content.kpis.reviewing', { defaultValue: 'In review' })}
          value={stats.reviewing}
        />
        <AdminKpi
          icon={Plus}
          label={t('admin.content.kpis.today', { defaultValue: 'Today new' })}
          value={stats.today}
        />
        <AdminKpi
          icon={Shield}
          label={t('admin.content.kpis.total', { defaultValue: 'Total reports' })}
          value={stats.total}
        />
      </section>

      <section className="gl-admin-panel gl-admin-content-panel">
        <div className="gl-admin-panel-head">
          <div>
            <span>{t('admin.content.reports.eyebrow', { defaultValue: 'Reports' })}</span>
            <h2>{t('admin.content.reports.title', { defaultValue: 'Report management' })}</h2>
          </div>
        </div>
        <div className="gl-admin-content-filters">
          <AdminFilterSelect
            label={t('admin.content.filters.status', { defaultValue: 'Status' })}
            value={status}
            options={['all', 'pending', 'reviewing', 'resolved', 'dismissed'].map((value) => ({
              value,
              label: reportStatusLabel(value, t),
            }))}
            onChange={setStatus}
          />
          <AdminFilterSelect
            label={t('admin.content.filters.target', { defaultValue: 'Target' })}
            value={targetType}
            options={['all', 'room', 'channel', 'danmu', 'post', 'post_comment', 'super_chat'].map(
              (value) => ({
                value,
                label: reportTargetLabel(value, t),
              }),
            )}
            onChange={setTargetType}
          />
          <AdminFilterSelect
            label={t('admin.content.filters.reason', { defaultValue: 'Reason' })}
            value={reason}
            options={[
              'all',
              'spam',
              'harassment',
              'sexual',
              'violence',
              'hate',
              'scam',
              'illegal',
              'other',
            ].map((value) => ({
              value,
              label: reportReasonLabel(value, t),
            }))}
            onChange={setReason}
          />
          <label className="gl-admin-content-search">
            <span>{t('admin.content.filters.search', { defaultValue: 'Search' })}</span>
            <div>
              <Search size={15} />
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t('admin.content.filters.searchPlaceholder', {
                  defaultValue: 'Reporter, target, text',
                })}
              />
            </div>
          </label>
        </div>

        <div className="gl-admin-content-grid">
          <div className="gl-admin-report-list" aria-busy={reports.isFetching}>
            {reports.isLoading ? (
              <AdminEmptyState
                label={t('admin.content.reports.loading', { defaultValue: 'Loading reports...' })}
              />
            ) : items.length === 0 ? (
              <AdminEmptyState
                label={t('admin.content.reports.empty', {
                  defaultValue: 'No reports in this filter.',
                })}
              />
            ) : (
              items.map((item) => (
                <button
                  type="button"
                  key={item.id}
                  className={
                    item.id === selectedId
                      ? 'gl-admin-report-card is-active'
                      : 'gl-admin-report-card'
                  }
                  onClick={() => {
                    setSelectedId(item.id);
                    setResolutionNote(item.resolutionNote ?? '');
                    setSelectedActions([]);
                    setDetailOpen(true);
                  }}
                >
                  <div className="gl-admin-report-card-head">
                    <span className={`gl-admin-report-severity ${item.status}`}>
                      {reportStatusLabel(item.status, t)}
                    </span>
                    <time>{formatDate(item.createdAt)}</time>
                  </div>
                  <strong>{reportReasonLabel(item.reason, t)}</strong>
                  <p>
                    {item.targetTitle || item.targetText || item.targetOwnerName || item.targetId}
                  </p>
                  <div className="gl-admin-report-card-meta">
                    <span>{reportTargetLabel(item.targetType, t)}</span>
                    <span>
                      {t('admin.content.reports.reportCount', {
                        count: item.reportCount ?? 1,
                        defaultValue: '{{count}} reports',
                      })}
                    </span>
                    {Boolean(item.recentCount) && (
                      <span>
                        {t('admin.content.reports.recentCount', {
                          count: item.recentCount,
                          defaultValue: '{{count}} in 1h',
                        })}
                      </span>
                    )}
                    {item.targetUserName && <span>{item.targetUserName}</span>}
                    {item.resolutionAction && (
                      <span>{reportActionLabels(item.resolutionAction, t)}</span>
                    )}
                  </div>
                </button>
              ))
            )}
          </div>
        </div>
      </section>

      <Dialog open={detailOpen} onOpenChange={setDetailOpen}>
        <DialogContent className="gl-admin-report-dialog p-0 sm:max-w-[1080px]">
          {detail ? (
            <div className="gl-admin-report-modal">
              <div className="gl-admin-report-modal-head">
                <div>
                  <span>{reportTargetLabel(detail.targetType, t)}</span>
                  <DialogTitle>
                    {detail.targetTitle ||
                      detail.targetText ||
                      t('admin.content.reports.title', { defaultValue: 'Report management' })}
                  </DialogTitle>
                  <DialogDescription>
                    {t('admin.content.reports.reportCount', {
                      count: detail.reportCount ?? detail.reports?.length ?? 1,
                      defaultValue: '{{count}} reports',
                    })}
                    {detail.recentCount
                      ? ` · ${t('admin.content.reports.recentCount', {
                          count: detail.recentCount,
                          defaultValue: '{{count}} in 1h',
                        })}`
                      : ''}
                  </DialogDescription>
                </div>
                <span className={`gl-admin-report-severity ${detail.status}`}>
                  {reportStatusLabel(detail.status, t)}
                </span>
              </div>

              <div className="gl-admin-report-modal-grid">
                <div className="gl-admin-report-modal-main">
                  <div className="gl-admin-report-snapshot is-modal">
                    <span>
                      {t('admin.content.reports.snapshot', { defaultValue: 'Content snapshot' })}
                    </span>
                    <h3>
                      {detail.targetTitle ||
                        t('admin.content.reports.noTitle', { defaultValue: 'No title' })}
                    </h3>
                    <p>
                      {detail.targetText ||
                        t('admin.content.reports.noSnapshot', {
                          defaultValue: 'No text snapshot.',
                        })}
                    </p>
                    {detail.targetUrl && (
                      <a href={detail.targetUrl} target="_blank" rel="noreferrer">
                        <Eye size={15} />
                        {t('admin.content.reports.openTarget', { defaultValue: 'Open target' })}
                      </a>
                    )}
                  </div>

                  <div className="gl-admin-reporters">
                    <strong>
                      {t('admin.content.reports.reporters', { defaultValue: 'Report records' })}
                    </strong>
                    {(detail.reports ?? [detail]).map((entry) => (
                      <div className="gl-admin-reporter-row" key={entry.id}>
                        <Avatar name={entry.reporterName} src={entry.reporterAvatar} size={30} />
                        <div>
                          <b>{entry.reporterName}</b>
                          <span>
                            {reportReasonLabel(entry.reason, t)} · {formatDate(entry.createdAt)}
                          </span>
                          {entry.description && <p>{entry.description}</p>}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>

                <aside className="gl-admin-report-modal-side">
                  <AdminDetailRow
                    label={t('admin.content.reports.reportedUser', {
                      defaultValue: 'Reported user',
                    })}
                    value={detail.targetUserName || detail.targetOwnerName || '-'}
                  />
                  <AdminDetailRow
                    label={t('admin.content.reports.status', { defaultValue: 'Status' })}
                    value={reportStatusLabel(detail.status, t)}
                  />
                  <AdminDetailRow
                    label={t('admin.content.reports.result', { defaultValue: 'Result' })}
                    value={
                      detail.resolutionAction ? reportActionLabels(detail.resolutionAction, t) : '-'
                    }
                  />
                  {detailClosed && (
                    <div className="gl-admin-handled-note">
                      <Check size={16} />
                      <div>
                        <strong>
                          {t('admin.content.reports.handledTitle', {
                            defaultValue: 'Handled report',
                          })}
                        </strong>
                        <span>
                          {t('admin.content.reports.handledBody', {
                            defaultValue: 'Closed reports cannot be processed again.',
                          })}
                        </span>
                      </div>
                    </div>
                  )}
                  <label className="gl-admin-resolution-note">
                    <span>
                      {t('admin.content.reports.note', { defaultValue: 'Resolution note' })}
                    </span>
                    <textarea
                      value={resolutionNote}
                      onChange={(event) => setResolutionNote(event.target.value)}
                      disabled={detailClosed}
                      placeholder={t('admin.content.reports.notePlaceholder', {
                        defaultValue: 'Record action notes for audit.',
                      })}
                    />
                  </label>
                  {!detailClosed && (
                    <div className="gl-admin-action-select">
                      <div className="gl-admin-action-select-head">
                        <span>{t('admin.content.reports.action', { defaultValue: 'Action' })}</span>
                        <small>
                          {t('admin.content.reports.actionMultiHint', {
                            defaultValue:
                              'Multiple penalties can be selected. Dismiss is separate.',
                          })}
                        </small>
                      </div>
                      <div className="gl-admin-action-grid">
                        {detailActions.map((action) => {
                          const active = selectedActions.includes(action);
                          return (
                            <button
                              type="button"
                              key={action}
                              className={
                                active ? 'gl-admin-action-card is-active' : 'gl-admin-action-card'
                              }
                              aria-pressed={active}
                              onClick={() => toggleReportAction(action)}
                            >
                              <span className="gl-admin-action-card-title">
                                <span>{reportActionLabel(action, t)}</span>
                                <i>{active && <Check size={13} />}</i>
                              </span>
                              <small>{reportActionDescription(action, t)}</small>
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  )}
                  {!detailClosed && selectedActions.includes('site_mute') && (
                    <div className="gl-admin-action-select">
                      <span>
                        {t('admin.content.reports.muteDuration', {
                          defaultValue: 'Mute duration',
                        })}
                      </span>
                      <div className="gl-admin-duration-grid">
                        {[30, 120, 1440, 10080].map((value) => (
                          <button
                            type="button"
                            className={
                              muteDuration === value
                                ? 'gl-admin-duration-pill is-active'
                                : 'gl-admin-duration-pill'
                            }
                            key={value}
                            onClick={() => setMuteDuration(value)}
                            disabled={detailClosed}
                          >
                            {formatMuteDuration(value)}
                          </button>
                        ))}
                      </div>
                    </div>
                  )}
                  <div className="gl-admin-report-actions-bar is-modal">
                    <button
                      type="button"
                      onClick={() => submitReportActions()}
                      disabled={
                        updateReport.isPending || detailClosed || selectedActions.length === 0
                      }
                    >
                      <Check size={15} />
                      {t('admin.content.reports.confirmAction', {
                        defaultValue: 'Confirm action',
                      })}
                    </button>
                    <button
                      type="button"
                      onClick={() => submitReportActions(['dismiss'])}
                      disabled={updateReport.isPending || detailClosed}
                    >
                      <X size={15} />
                      {reportActionLabel('dismiss', t)}
                    </button>
                  </div>
                </aside>
              </div>
            </div>
          ) : (
            <AdminEmptyState
              label={t('admin.content.reports.loading', { defaultValue: 'Loading reports...' })}
            />
          )}
        </DialogContent>
      </Dialog>

      <section className="gl-admin-panel gl-admin-words-panel">
        <div className="gl-admin-panel-head">
          <div>
            <span>{t('admin.content.words.eyebrow', { defaultValue: 'Policy' })}</span>
            <h2>{t('admin.content.words.title', { defaultValue: 'Blocked word management' })}</h2>
          </div>
        </div>
        <div className="gl-admin-word-form">
          <input
            value={word}
            maxLength={60}
            onChange={(event) => setWord(event.target.value)}
            placeholder={t('admin.content.words.wordPlaceholder', { defaultValue: 'Blocked word' })}
          />
          <input
            value={wordNote}
            maxLength={120}
            onChange={(event) => setWordNote(event.target.value)}
            placeholder={t('admin.content.words.notePlaceholder', {
              defaultValue: 'Note, optional',
            })}
          />
          <button type="button" onClick={submitWord} disabled={createWord.isPending}>
            <Plus size={15} />
            {t('admin.content.words.add', { defaultValue: 'Add' })}
          </button>
        </div>
        <div className="gl-admin-word-import-row">
          <label className="gl-admin-word-import">
            <input
              type="file"
              accept=".txt,.csv,.tsv,text/plain,text/csv"
              onChange={handleBlockedWordImport}
              disabled={importWords.isPending}
            />
            <Upload size={15} />
            {t('admin.content.words.import', { defaultValue: 'Batch import' })}
          </label>
          <span>
            {t('admin.content.words.importHint', {
              defaultValue: 'TXT/CSV/TSV, one word per line. Use word,note for remarks.',
            })}
          </span>
        </div>
        <div className="gl-admin-word-list" aria-busy={blockedWords.isFetching}>
          {(blockedWords.data?.items ?? []).length === 0 ? (
            <AdminEmptyState
              label={t('admin.content.words.empty', { defaultValue: 'No blocked words yet.' })}
            />
          ) : (
            (blockedWords.data?.items ?? []).map((item) => {
              const editing = editingWord?.id === item.id;
              const row = editingWord ?? item;
              return (
                <div className="gl-admin-word-row" key={item.id}>
                  {editing ? (
                    <>
                      <input
                        value={row.word}
                        onChange={(event) => setEditingWord({ ...row, word: event.target.value })}
                      />
                      <input
                        value={row.note ?? ''}
                        onChange={(event) => setEditingWord({ ...row, note: event.target.value })}
                      />
                    </>
                  ) : (
                    <>
                      <div>
                        <strong>{item.word}</strong>
                        <span>
                          {item.note ||
                            t('admin.content.words.noNote', { defaultValue: 'No note' })}
                        </span>
                      </div>
                      <span
                        className={
                          item.enabled ? 'gl-admin-word-status is-on' : 'gl-admin-word-status'
                        }
                      >
                        {item.enabled
                          ? t('admin.content.words.enabled', { defaultValue: 'Enabled' })
                          : t('admin.content.words.disabled', { defaultValue: 'Disabled' })}
                      </span>
                    </>
                  )}
                  <div className="gl-admin-word-actions">
                    {editing ? (
                      <>
                        <button
                          type="button"
                          onClick={saveEditingWord}
                          disabled={updateWord.isPending}
                        >
                          <Save size={15} />
                        </button>
                        <button type="button" onClick={() => setEditingWord(null)}>
                          <X size={15} />
                        </button>
                      </>
                    ) : (
                      <>
                        <button
                          type="button"
                          onClick={() =>
                            updateWord.mutate({
                              id: item.id,
                              enabled: !item.enabled,
                            })
                          }
                          disabled={updateWord.isPending}
                        >
                          {item.enabled ? <Ban size={15} /> : <Check size={15} />}
                        </button>
                        <button type="button" onClick={() => setEditingWord(item)}>
                          <Pencil size={15} />
                        </button>
                        <button
                          type="button"
                          className="is-danger"
                          onClick={() => {
                            const ok = window.confirm(
                              t('admin.content.words.confirmDelete', {
                                word: item.word,
                                defaultValue: 'Delete blocked word {{word}}?',
                              }),
                            );
                            if (!ok) return;
                            deleteWord.mutate(item.id, {
                              onSuccess: () =>
                                toast.success(
                                  t('admin.content.words.deleted', {
                                    defaultValue: 'Blocked word deleted.',
                                  }),
                                ),
                              onError: (err) =>
                                toast.error(
                                  err.message ||
                                    t('admin.content.words.deleteFailed', {
                                      defaultValue: 'Could not delete blocked word.',
                                    }),
                                ),
                            });
                          }}
                          disabled={deleteWord.isPending}
                        >
                          <Trash2 size={15} />
                        </button>
                      </>
                    )}
                  </div>
                </div>
              );
            })
          )}
        </div>
      </section>
    </div>
  );
}

function AdminEmptyState({ label }: { label: string }) {
  return <div className="gl-admin-empty-state">{label}</div>;
}

function AdminFilterSelect({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
}) {
  const selected = options.find((option) => option.value === value) ?? options[0];
  return (
    <div className="gl-admin-filter-select">
      <span>{label}</span>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button type="button" className="gl-admin-filter-trigger">
            <span>{selected?.label ?? value}</span>
            <ChevronDown size={15} />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="gl-admin-filter-menu" sideOffset={6}>
          {options.map((option) => (
            <DropdownMenuItem
              key={option.value}
              className={
                option.value === value
                  ? 'gl-admin-filter-option is-active'
                  : 'gl-admin-filter-option'
              }
              onSelect={() => onChange(option.value)}
            >
              <span>{option.label}</span>
              {option.value === value && <Check size={14} />}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

function reportStatusLabel(value: string, t: Translate) {
  const map: Record<string, string> = {
    all: t('admin.content.status.all', { defaultValue: 'All statuses' }),
    pending: t('admin.content.status.pending', { defaultValue: 'Pending' }),
    reviewing: t('admin.content.status.reviewing', { defaultValue: 'Reviewing' }),
    resolved: t('admin.content.status.resolved', { defaultValue: 'Resolved' }),
    dismissed: t('admin.content.status.dismissed', { defaultValue: 'Dismissed' }),
  };
  return map[value] ?? value;
}

function reportTargetLabel(value: string, t: Translate) {
  const map: Record<string, string> = {
    all: t('admin.content.targets.all', { defaultValue: 'All targets' }),
    room: t('admin.content.targets.room', { defaultValue: 'Live room' }),
    channel: t('admin.content.targets.channel', { defaultValue: 'Channel' }),
    danmu: t('admin.content.targets.danmu', { defaultValue: 'Chat message' }),
    post: t('admin.content.targets.post', { defaultValue: 'Post' }),
    post_comment: t('admin.content.targets.postComment', { defaultValue: 'Post comment' }),
    super_chat: t('admin.content.targets.superChat', { defaultValue: 'SuperChat' }),
  };
  return map[value] ?? value;
}

function reportReasonLabel(value: string, t: Translate) {
  const map: Record<string, string> = {
    all: t('admin.content.reasons.all', { defaultValue: 'All reasons' }),
    spam: t('report.reasons.spam', { defaultValue: 'Spam or ads' }),
    harassment: t('report.reasons.harassment', { defaultValue: 'Harassment' }),
    sexual: t('report.reasons.sexual', { defaultValue: 'Sexual content' }),
    violence: t('report.reasons.violence', { defaultValue: 'Violence' }),
    hate: t('report.reasons.hate', { defaultValue: 'Hate speech' }),
    scam: t('report.reasons.scam', { defaultValue: 'Scam' }),
    illegal: t('report.reasons.illegal', { defaultValue: 'Illegal activity' }),
    other: t('report.reasons.other', { defaultValue: 'Other' }),
  };
  return map[value] ?? value;
}

function reportActionLabel(value: string, t: Translate) {
  const map: Record<string, string> = {
    review: t('admin.content.actions.review', { defaultValue: 'Mark reviewing' }),
    dismiss: t('admin.content.actions.dismiss', { defaultValue: 'Dismiss' }),
    delete_content: t('admin.content.actions.deleteContent', { defaultValue: 'Delete content' }),
    warn_user: t('admin.content.actions.warnUser', { defaultValue: 'Warn user' }),
    warn_room: t('admin.content.actions.warnRoom', { defaultValue: 'Warn live room' }),
    site_mute: t('admin.content.actions.siteMute', { defaultValue: 'Site mute' }),
    ban_user: t('admin.content.actions.banUser', { defaultValue: 'Ban user' }),
    force_end_live: t('admin.content.actions.forceEndLive', {
      defaultValue: 'Force end live',
    }),
  };
  return map[value] ?? value;
}

function reportActionLabels(value: string, t: Translate) {
  return uniqueReportActions(
    value
      .split(',')
      .map((action) => action.trim())
      .filter(Boolean) as ReportAction[],
  )
    .map((action) => reportActionLabel(action, t))
    .join(' / ');
}

function reportActionDescription(value: string, t: Translate) {
  const map: Record<string, string> = {
    dismiss: t('admin.content.actionDescriptions.dismiss', {
      defaultValue: 'Close the case with no penalty.',
    }),
    delete_content: t('admin.content.actionDescriptions.deleteContent', {
      defaultValue: 'Remove the reported content from public views.',
    }),
    warn_user: t('admin.content.actionDescriptions.warnUser', {
      defaultValue: 'Send a moderation warning to the user.',
    }),
    warn_room: t('admin.content.actionDescriptions.warnRoom', {
      defaultValue: 'Warn the live room without ending it.',
    }),
    site_mute: t('admin.content.actionDescriptions.siteMute', {
      defaultValue: 'Block interactive posting for a period.',
    }),
    ban_user: t('admin.content.actionDescriptions.banUser', {
      defaultValue: 'Ban the account from interactive features.',
    }),
    force_end_live: t('admin.content.actionDescriptions.forceEndLive', {
      defaultValue: 'Immediately stop the active live stream.',
    }),
  };
  return map[value] ?? '';
}

function reportActionsForTarget(targetType: string): ReportAction[] {
  if (targetType === 'room') {
    return ['warn_room', 'force_end_live', 'warn_user', 'site_mute', 'ban_user', 'dismiss'];
  }
  if (targetType === 'channel') {
    return ['warn_user', 'site_mute', 'ban_user', 'dismiss'];
  }
  return ['delete_content', 'warn_user', 'site_mute', 'ban_user', 'dismiss'];
}

function isExclusiveReportAction(action: ReportAction) {
  return action === 'dismiss' || action === 'review';
}

function uniqueReportActions(actions: ReportAction[]) {
  const seen = new Set<ReportAction>();
  const out: ReportAction[] = [];
  actions.forEach((action) => {
    if (!action || seen.has(action)) return;
    seen.add(action);
    out.push(action);
  });
  return out;
}

function isReportClosed(status: string) {
  return status === 'resolved' || status === 'dismissed';
}

function formatMuteDuration(minutes: number) {
  if (minutes >= 1440) return `${Math.round(minutes / 1440)}d`;
  if (minutes >= 60) return `${Math.round(minutes / 60)}h`;
  return `${minutes}m`;
}

function reportMutationErrorMessage(err: unknown, t: Translate) {
  const response = (err as { response?: { data?: { reason?: string; message?: string } } })
    ?.response;
  if (response?.data?.reason === 'report_already_handled') {
    return t('admin.content.reports.alreadyHandled', {
      defaultValue: 'This report has already been handled.',
    });
  }
  if (response?.data?.message) return response.data.message;
  if (err instanceof Error && err.message) return err.message;
  return t('admin.content.reports.updateFailed', {
    defaultValue: 'Could not update report.',
  });
}

function parseBlockedWordImport(text: string) {
  const seen = new Set<string>();
  const items: { word: string; note?: string }[] = [];
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith('#')) continue;
    const [rawWord, ...noteParts] = splitBlockedWordLine(line);
    const word = cleanImportCell(rawWord);
    if (!word || isBlockedWordImportHeader(word)) continue;
    const normalized = word.toLocaleLowerCase();
    if (seen.has(normalized)) continue;
    seen.add(normalized);
    const note = cleanImportCell(noteParts.join(' '));
    items.push({ word, note: note || undefined });
  }
  return items;
}

function splitBlockedWordLine(line: string) {
  if (line.includes('\t')) return line.split('\t');
  if (line.includes(',')) return line.split(',');
  if (line.includes('，')) return line.split('，');
  return [line];
}

function cleanImportCell(value: string) {
  return value
    .trim()
    .replace(/^["'“”]+|["'“”]+$/g, '')
    .trim();
}

function isBlockedWordImportHeader(value: string) {
  const normalized = value.toLocaleLowerCase();
  return ['word', 'blocked word', 'blocked_word', '屏蔽词', '关键词', '禁止词', 'ワード'].includes(
    normalized,
  );
}

function auditCategoryLabel(value: string, t: Translate) {
  const map: Record<string, string> = {
    review: t('admin.logs.tabs.review', { defaultValue: 'Review actions' }),
    permission: t('admin.logs.tabs.permission', { defaultValue: 'Permission actions' }),
    system: t('admin.logs.tabs.system', { defaultValue: 'System actions' }),
  };
  return map[value] ?? value;
}

function auditActionLabel(value: string, t: Translate) {
  const map: Record<string, string> = {
    delete_content: t('admin.logs.actions.deleteContent', { defaultValue: 'Delete content' }),
    warn_user: t('admin.logs.actions.warnUser', { defaultValue: 'Warn user' }),
    warn_room: t('admin.logs.actions.warnRoom', { defaultValue: 'Warn live room' }),
    site_mute: t('admin.logs.actions.siteMute', { defaultValue: 'Site mute' }),
    ban_user: t('admin.logs.actions.banUser', { defaultValue: 'Ban user' }),
    force_end_live: t('admin.logs.actions.forceEndLive', { defaultValue: 'Force end live' }),
    dismiss: t('admin.logs.actions.dismiss', { defaultValue: 'Dismiss report' }),
    review: t('admin.logs.actions.review', { defaultValue: 'Mark reviewing' }),
    blocked_word_create: t('admin.logs.actions.blockedWordCreate', {
      defaultValue: 'Add blocked word',
    }),
    blocked_word_update: t('admin.logs.actions.blockedWordUpdate', {
      defaultValue: 'Update blocked word',
    }),
    blocked_word_delete: t('admin.logs.actions.blockedWordDelete', {
      defaultValue: 'Delete blocked word',
    }),
    blocked_word_import: t('admin.logs.actions.blockedWordImport', {
      defaultValue: 'Import blocked words',
    }),
  };
  return map[value] ?? value;
}

function ScaffoldModulePage({
  module,
}: {
  module: Exclude<AdminModule, 'dashboard' | 'users' | 'creators' | 'logs'>;
}) {
  const { t } = useTranslation('pages');
  const config = {
    content: {
      eyebrow: t('admin.scaffold.content.eyebrow', { defaultValue: 'Content' }),
      title: t('admin.scaffold.content.title', { defaultValue: 'Content review framework' }),
      icon: FileCheck2,
      stats: [
        [t('admin.scaffold.content.stats.liveRooms', { defaultValue: 'Pending live rooms' }), '0'],
        [t('admin.scaffold.content.stats.posts', { defaultValue: 'Post review' }), '0'],
        [t('admin.scaffold.content.stats.reports', { defaultValue: 'Reports queue' }), '0'],
        [t('admin.scaffold.content.stats.keywords', { defaultValue: 'Keyword policies' }), '0'],
      ] as [string, string][],
      rows: [
        [
          t('admin.scaffold.content.rows.liveRooms', { defaultValue: 'Live room review' }),
          t('admin.scaffold.content.rows.liveRoomsSub', {
            defaultValue: 'Title, cover, category, and live state',
          }),
        ],
        [
          t('admin.scaffold.content.rows.posts', { defaultValue: 'Post content review' }),
          t('admin.scaffold.content.rows.postsSub', {
            defaultValue: 'Posts, images, and comments',
          }),
        ],
        [
          t('admin.scaffold.content.rows.reports', { defaultValue: 'Report handling' }),
          t('admin.scaffold.content.rows.reportsSub', {
            defaultValue: 'User reports and moderation results',
          }),
        ],
      ] as [string, string][],
    },
    economy: {
      eyebrow: t('admin.scaffold.economy.eyebrow', { defaultValue: 'Economy' }),
      title: t('admin.scaffold.economy.title', { defaultValue: 'Economy system framework' }),
      icon: Coins,
      stats: [
        [t('admin.scaffold.economy.stats.topup', { defaultValue: 'Today top-up' }), '-'],
        [t('admin.scaffold.economy.stats.gifts', { defaultValue: 'Gift ledger' }), '-'],
        [t('admin.scaffold.economy.stats.superChat', { defaultValue: 'SC ledger' }), '-'],
        [
          t('admin.scaffold.economy.stats.withdrawals', { defaultValue: 'Withdrawal reserve' }),
          '-',
        ],
      ] as [string, string][],
      rows: [
        [
          t('admin.scaffold.economy.rows.accounts', { defaultValue: 'Coins accounts' }),
          t('admin.scaffold.economy.rows.accountsSub', {
            defaultValue: 'Balance, top-up, and spending ledger',
          }),
        ],
        [
          t('admin.scaffold.economy.rows.gifts', { defaultValue: 'Gifts and SuperChat' }),
          t('admin.scaffold.economy.rows.giftsSub', {
            defaultValue: 'Price, level, and revenue collection',
          }),
        ],
        [
          t('admin.scaffold.economy.rows.revenue', { defaultValue: 'Creator revenue' }),
          t('admin.scaffold.economy.rows.revenueSub', {
            defaultValue: 'Split, withdrawal, and settlement state',
          }),
        ],
      ] as [string, string][],
    },
    system: {
      eyebrow: t('admin.scaffold.system.eyebrow', { defaultValue: 'System' }),
      title: t('admin.scaffold.system.title', { defaultValue: 'System configuration framework' }),
      icon: Settings,
      stats: [
        [
          t('admin.scaffold.system.stats.registration', { defaultValue: 'Registration policy' }),
          t('admin.scaffold.system.values.inviteOnly', { defaultValue: 'Invite only' }),
        ],
        [
          t('admin.scaffold.system.stats.liveReview', { defaultValue: 'Live review' }),
          t('admin.scaffold.system.values.enabled', { defaultValue: 'Enabled' }),
        ],
        [
          t('admin.scaffold.system.stats.contentPolicy', { defaultValue: 'Content policy' }),
          t('admin.scaffold.system.values.standard', { defaultValue: 'Standard' }),
        ],
        [t('admin.scaffold.system.stats.serviceStatus', { defaultValue: 'Service status' }), '-'],
      ] as [string, string][],
      rows: [
        [
          t('admin.scaffold.system.rows.site', { defaultValue: 'Site configuration' }),
          t('admin.scaffold.system.rows.siteSub', {
            defaultValue: 'Brand, domain, and base switches',
          }),
        ],
        [
          t('admin.scaffold.system.rows.review', { defaultValue: 'Review policies' }),
          t('admin.scaffold.system.rows.reviewSub', {
            defaultValue: 'Live applications, content review, and report rules',
          }),
        ],
        [
          t('admin.scaffold.system.rows.services', { defaultValue: 'Service configuration' }),
          t('admin.scaffold.system.rows.servicesSub', {
            defaultValue: 'RTMP, replays, and upload limits',
          }),
        ],
      ] as [string, string][],
    },
  }[module];
  const Icon = config.icon;

  return (
    <div className="gl-admin-section-stack">
      <section className="gl-admin-kpi-grid" aria-label={`${config.title} summary`}>
        {config.stats.map(([label, value]) => (
          <AdminKpi key={label} icon={Icon} label={label} value={value} />
        ))}
      </section>
      <StaticOperationsPanel eyebrow={config.eyebrow} title={config.title} rows={config.rows} />
    </div>
  );
}

function LogsPage() {
  const { t } = useTranslation('pages');
  const [category, setCategory] = useState<AdminAuditCategory>('review');
  const logs = useAdminAuditLogs(category, 1, 30);
  const stats = logs.data?.stats ?? { today: 0, review: 0, permission: 0, system: 0 };
  const logItems = logs.data?.items ?? [];
  const tabs: AdminAuditCategory[] = ['review', 'permission', 'system'];

  return (
    <div className="gl-admin-section-stack">
      <section
        className="gl-admin-kpi-grid"
        aria-label={t('admin.logs.aria', { defaultValue: 'Action logs summary' })}
      >
        <AdminKpi
          icon={History}
          label={t('admin.logs.kpis.today', { defaultValue: 'Today actions' })}
          value={stats.today}
        />
        <AdminKpi
          icon={FileText}
          label={t('admin.logs.kpis.reviews', { defaultValue: 'Review records' })}
          value={stats.review}
        />
        <AdminKpi
          icon={Shield}
          label={t('admin.logs.kpis.permissions', { defaultValue: 'Permission changes' })}
          value={stats.permission}
        />
        <AdminKpi
          icon={Settings}
          label={t('admin.logs.kpis.accounts', { defaultValue: 'Account actions' })}
          value={stats.system}
        />
      </section>
      <section className="gl-admin-panel">
        <div className="gl-admin-panel-head">
          <div>
            <span>{t('admin.logs.eyebrow', { defaultValue: 'Logs' })}</span>
            <h2>{t('admin.logs.title', { defaultValue: 'Operation logs' })}</h2>
          </div>
        </div>
        <div className="gl-admin-log-tabs" role="tablist">
          {tabs.map((item) => (
            <button
              type="button"
              role="tab"
              aria-selected={category === item}
              className={category === item ? 'is-active' : ''}
              key={item}
              onClick={() => setCategory(item)}
            >
              {auditCategoryLabel(item, t)}
            </button>
          ))}
        </div>
        <div className="gl-admin-log-table" aria-busy={logs.isFetching}>
          {logs.isLoading ? (
            <AdminEmptyState label={t('admin.logs.loading', { defaultValue: 'Loading logs...' })} />
          ) : logItems.length === 0 ? (
            <AdminEmptyState
              label={t('admin.logs.empty', { defaultValue: 'No operations in this category.' })}
            />
          ) : (
            logItems.map((item) => (
              <article className="gl-admin-log-row" key={item.id}>
                <div>
                  <span>{auditCategoryLabel(item.category, t)}</span>
                  <strong>{auditActionLabel(item.action, t)}</strong>
                  <p>
                    {item.targetTitle ||
                      item.targetUserName ||
                      item.targetId ||
                      t('admin.logs.noTarget', { defaultValue: 'No target recorded' })}
                  </p>
                </div>
                <div className="gl-admin-log-meta">
                  <span>
                    {t('admin.logs.actor', { defaultValue: 'Actor' })}: {item.actorName || '-'}
                  </span>
                  <span>
                    {t('admin.logs.note', { defaultValue: 'Note' })}:{' '}
                    {item.note || t('admin.logs.noNote', { defaultValue: 'No note' })}
                  </span>
                  <time>{formatDate(item.createdAt)}</time>
                </div>
              </article>
            ))
          )}
        </div>
      </section>
    </div>
  );
}

function AdminKpi({
  icon: Icon,
  label,
  value,
  tone,
}: {
  icon: AdminIcon;
  label: string;
  value: string | number;
  tone?: 'red';
}) {
  return (
    <article className={tone === 'red' ? 'gl-admin-kpi is-red' : 'gl-admin-kpi'}>
      <span className="gl-admin-kpi-icon">
        <Icon size={18} />
      </span>
      <div>
        <strong>{value}</strong>
        <span>{label}</span>
      </div>
    </article>
  );
}

function StaticOperationsPanel({
  eyebrow,
  title,
  rows,
}: {
  eyebrow: string;
  title: string;
  rows: [string, string][];
}) {
  return (
    <section className="gl-admin-panel">
      <div className="gl-admin-panel-head">
        <div>
          <span>{eyebrow}</span>
          <h2>{title}</h2>
        </div>
      </div>
      <div className="gl-admin-detail-list">
        {rows.map(([label, value]) => (
          <AdminDetailRow key={label} label={label} value={value} />
        ))}
      </div>
    </section>
  );
}

function AdminDetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="gl-admin-detail-row">
      <strong>{label}</strong>
      <span>{value}</span>
    </div>
  );
}

function PermissionPanel({
  items,
  loading,
  error,
  busy,
  onDisable,
}: {
  items: LiveCreator[];
  loading: boolean;
  error: boolean;
  busy: boolean;
  onDisable: (item: LiveCreator) => void;
}) {
  const { t } = useTranslation('pages');
  return (
    <section className="gl-admin-panel">
      <div className="gl-admin-panel-head">
        <div>
          <span>{t('admin.permissions.eyebrow', { defaultValue: 'Permission management' })}</span>
          <h2>{t('admin.permissions.title', { defaultValue: 'Creators who can go live' })}</h2>
        </div>
      </div>
      {loading ? (
        <div className="gl-yt-shelf-empty">
          {t('admin.permissions.loading', { defaultValue: 'Loading creators...' })}
        </div>
      ) : error ? (
        <div className="gl-yt-shelf-empty">
          {t('admin.permissions.error', { defaultValue: 'Could not load creators.' })}
        </div>
      ) : items.length ? (
        <div className="gl-admin-list">
          {items.map((item) => (
            <div className="gl-admin-permission-row" key={item.id}>
              <AdminUser
                avatar={item.avatar}
                name={item.displayName || item.username}
                handle={`@${item.username || item.id.slice(0, 8)}`}
              />
              <span className="gl-admin-muted">{formatDate(item.updatedAt)}</span>
              <span className="gl-admin-status is-approved">
                {t('admin.status.approved', { defaultValue: 'Approved' })}
              </span>
              <button
                type="button"
                className="gl-admin-danger-btn"
                aria-label={t('admin.permissions.disableFor', {
                  name: item.displayName || item.username,
                  defaultValue: 'Disable live permission for {{name}}',
                })}
                title={t('admin.permissions.disable', { defaultValue: 'Disable live' })}
                disabled={busy}
                onClick={() => onDisable(item)}
              >
                <Ban size={16} />
                <span>{t('admin.permissions.disable', { defaultValue: 'Disable live' })}</span>
              </button>
            </div>
          ))}
        </div>
      ) : (
        <div className="gl-yt-shelf-empty">
          {t('admin.permissions.empty', {
            defaultValue: 'No creators currently have live permission.',
          })}
        </div>
      )}
    </section>
  );
}

function ApplicationsPanel({
  items,
  loading,
  error,
  approveBusy,
  rejectBusy,
  onApprove,
  onReject,
}: {
  items: CreatorApplication[];
  loading: boolean;
  error: boolean;
  approveBusy: boolean;
  rejectBusy: boolean;
  onApprove: (id: string) => void;
  onReject: (id: string, reason: string) => void;
}) {
  const { t } = useTranslation('pages');
  const [rejectingId, setRejectingId] = useState('');
  const [rejectReason, setRejectReason] = useState('');

  return (
    <section className="gl-admin-panel">
      <div className="gl-admin-panel-head">
        <div>
          <span>{t('admin.applications.eyebrow', { defaultValue: 'Platform review' })}</span>
          <h2>{t('admin.applications.title', { defaultValue: 'Platform creator applications' })}</h2>
          <p>
            {t('admin.applications.platformHint', {
              defaultValue:
                'Approving certifies the creator, enables the orange badge, and applies the lower withdrawal fee.',
            })}
          </p>
        </div>
      </div>
      {loading ? (
        <div className="gl-yt-shelf-empty">
          {t('admin.applications.loading', { defaultValue: 'Loading applications...' })}
        </div>
      ) : error ? (
        <div className="gl-yt-shelf-empty">
          {t('admin.applications.error', { defaultValue: 'Could not load applications.' })}
        </div>
      ) : items.length ? (
        <div className="gl-admin-review-list">
          {items.map((app) => {
            const pending = app.status === 'pending';
            const busy = approveBusy || rejectBusy;
            const rejecting = rejectingId === app.id;
            return (
              <article className="gl-admin-review-card" key={app.id}>
                <div className="gl-admin-review-top">
                  <AdminUser
                    avatar={app.avatar}
                    name={
                      app.displayName ||
                      app.username ||
                      t('admin.fallbackCreator', { defaultValue: 'Creator' })
                    }
                    handle={`@${app.username || app.userId.slice(0, 8)}`}
                  />
                  <div className="gl-admin-review-meta">
                    <span className={`gl-admin-status is-${app.status}`}>
                      {statusText(app.status, t)}
                    </span>
                    <small>{formatDate(app.createdAt)}</small>
                  </div>
                </div>
                <div className="gl-admin-reason">
                  <span>{t('admin.applications.reason', { defaultValue: 'Application note' })}</span>
                  <p>
                    {app.reason ||
                      t('admin.applications.noReason', { defaultValue: 'No reason provided.' })}
                  </p>
                </div>
                {app.rejectReason && (
                  <div className="gl-admin-reason is-reject">
                    <span>
                      {t('admin.applications.rejectReason', { defaultValue: 'Reject reason' })}
                    </span>
                    <p>{app.rejectReason}</p>
                  </div>
                )}
                {pending && (
                  <div className="gl-admin-review-actions">
                    <button
                      type="button"
                      className="gl-admin-action-text approve"
                      disabled={busy}
                      onClick={() => onApprove(app.id)}
                    >
                      <Check size={16} />
                      {t('admin.applications.approve', { defaultValue: 'Approve' })}
                    </button>
                    <button
                      type="button"
                      className="gl-admin-action-text reject"
                      disabled={busy}
                      onClick={() => {
                        setRejectingId(rejecting ? '' : app.id);
                        setRejectReason('');
                      }}
                    >
                      <X size={16} />
                      {t('admin.applications.reject', { defaultValue: 'Reject' })}
                    </button>
                  </div>
                )}
                {rejecting && pending && (
                  <div className="gl-admin-reject-form">
                    <label>
                      <span>
                        {t('admin.applications.rejectReasonLabel', {
                          defaultValue: 'Rejection reason',
                        })}
                      </span>
                      <textarea
                        rows={3}
                        value={rejectReason}
                        maxLength={500}
                        onChange={(event) => setRejectReason(event.target.value)}
                        placeholder={t('admin.applications.rejectReasonPlaceholder', {
                          defaultValue: 'Tell the creator what needs to be improved.',
                        })}
                      />
                    </label>
                    <button
                      type="button"
                      className="gl-admin-danger-btn"
                      disabled={busy || !rejectReason.trim()}
                      onClick={() => {
                        onReject(app.id, rejectReason.trim());
                        setRejectingId('');
                        setRejectReason('');
                      }}
                    >
                      {t('admin.applications.confirmReject', { defaultValue: 'Confirm reject' })}
                    </button>
                  </div>
                )}
              </article>
            );
          })}
        </div>
      ) : (
        <div className="gl-yt-shelf-empty">
          {t('admin.applications.empty', { defaultValue: 'No creator applications yet.' })}
        </div>
      )}
    </section>
  );
}

function InvitesPanel({
  items,
  loading,
  error,
  busy,
  deleteBusy,
  onCreate,
  onDelete,
}: {
  items: AdminInviteCode[];
  loading: boolean;
  error: boolean;
  busy: boolean;
  deleteBusy: boolean;
  onCreate: () => void;
  onDelete: (item: AdminInviteCode) => void;
}) {
  const { t } = useTranslation('pages');

  const copyCode = async (code: string) => {
    try {
      await navigator.clipboard.writeText(code);
      toast.success(t('admin.invites.copied', { defaultValue: 'Invite code copied.' }));
    } catch {
      toast.error(t('admin.invites.copyFailed', { defaultValue: 'Could not copy invite code.' }));
    }
  };

  return (
    <section className="gl-admin-panel">
      <div className="gl-admin-panel-head">
        <div>
          <span>{t('admin.invites.eyebrow', { defaultValue: 'Registration access' })}</span>
          <h2>{t('admin.invites.title', { defaultValue: 'Invite codes' })}</h2>
        </div>
        <button
          type="button"
          className="gl-admin-action-text approve"
          disabled={busy}
          onClick={onCreate}
        >
          <Plus size={16} />
          {t('admin.invites.create', { defaultValue: 'Create invite' })}
        </button>
      </div>
      {loading ? (
        <div className="gl-yt-shelf-empty">
          {t('admin.invites.loading', { defaultValue: 'Loading invite codes...' })}
        </div>
      ) : error ? (
        <div className="gl-yt-shelf-empty">
          {t('admin.invites.error', { defaultValue: 'Could not load invite codes.' })}
        </div>
      ) : items.length ? (
        <div className="gl-admin-invite-list">
          {items.map((item) => (
            <article className="gl-admin-invite-row" key={item.id}>
              <div className="gl-admin-invite-code">
                <Ticket size={18} />
                <strong>{item.code}</strong>
              </div>
              <span className={`gl-admin-status is-${item.used ? 'approved' : 'pending'}`}>
                {item.used
                  ? t('admin.invites.used', { defaultValue: 'Used' })
                  : t('admin.invites.unused', { defaultValue: 'Unused' })}
              </span>
              <div className="gl-admin-invite-user">
                {item.used ? (
                  <>
                    <strong>{item.usedDisplayName || item.usedUsername || item.usedBy}</strong>
                    <span>
                      @{item.usedUsername || item.usedBy?.slice(0, 8)} / {item.usedEmail || '-'}
                    </span>
                  </>
                ) : (
                  <>
                    <strong>{t('admin.invites.noUser', { defaultValue: 'No account yet' })}</strong>
                    <span>
                      {t('admin.invites.available', {
                        defaultValue: 'Available for one registration',
                      })}
                    </span>
                  </>
                )}
              </div>
              <span className="gl-admin-muted">
                {item.usedAt
                  ? t('admin.invites.usedAt', {
                      time: formatDate(item.usedAt),
                      defaultValue: 'Used {{time}}',
                    })
                  : t('admin.invites.createdAt', {
                      time: formatDate(item.createdAt),
                      defaultValue: 'Created {{time}}',
                    })}
              </span>
              <button
                type="button"
                className="gl-admin-action-text"
                onClick={() => copyCode(item.code)}
              >
                <Clipboard size={16} />
                {t('admin.invites.copy', { defaultValue: 'Copy' })}
              </button>
              <button
                type="button"
                className="gl-admin-action-text reject"
                disabled={item.used || deleteBusy}
                title={
                  item.used
                    ? t('admin.invites.deleteUsedHint', {
                        defaultValue: 'Used invite codes cannot be deleted.',
                      })
                    : t('admin.invites.delete', { defaultValue: 'Delete' })
                }
                onClick={() => onDelete(item)}
              >
                <X size={16} />
                {t('admin.invites.delete', { defaultValue: 'Delete' })}
              </button>
            </article>
          ))}
        </div>
      ) : (
        <div className="gl-yt-shelf-empty">
          {t('admin.invites.empty', { defaultValue: 'No invite codes have been created.' })}
        </div>
      )}
    </section>
  );
}

function AdminUser({ avatar, name, handle }: { avatar?: string; name: string; handle: string }) {
  return (
    <div className="gl-admin-user">
      <Avatar name={name} src={avatar} size={38} />
      <div>
        <strong>{name}</strong>
        <span>{handle}</span>
      </div>
    </div>
  );
}
