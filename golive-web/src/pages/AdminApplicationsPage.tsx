import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Navigate, useNavigate } from 'react-router-dom';
import { Ban, Check, RefreshCw, Shield, X } from 'lucide-react';
import { toast } from 'sonner';
import {
  useAdminCreatorApplications,
  useAdminLiveCreators,
  useReviewCreatorApplication,
  useUpdateLivePermission,
  type CreatorApplication,
  type LiveCreator,
} from '@/api/creator';
import { Avatar } from '@/components/Avatar';
import { useAuthStore } from '@/stores/useAuthStore';

type AdminTab = 'permissions' | 'applications';

function statusText(status: CreatorApplication['status'], t: ReturnType<typeof useTranslation>['t']) {
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

export function AdminIndexPage() {
  return <Navigate to="/admin/applications" replace />;
}

export default function AdminApplicationsPage() {
  const { t } = useTranslation('pages');
  const navigate = useNavigate();
  const user = useAuthStore((s) => s.user);
  const isAdmin = user?.role === 'admin';
  const [tab, setTab] = useState<AdminTab>('permissions');
  const apps = useAdminCreatorApplications(isAdmin);
  const creators = useAdminLiveCreators(isAdmin);
  const approve = useReviewCreatorApplication('approve');
  const reject = useReviewCreatorApplication('reject');
  const updatePermission = useUpdateLivePermission();

  if (!isAdmin) {
    return (
      <div className="gl-page gl-admin-page">
        <section className="gl-admin-denied">
          <Shield size={34} />
          <h1>{t('admin.denied.title', { defaultValue: 'No admin access' })}</h1>
          <p>{t('admin.denied.body', { defaultValue: 'This area is only available to administrators.' })}</p>
          <button type="button" className="gl-retry-btn" onClick={() => navigate('/')}>
            {t('admin.denied.action', { defaultValue: 'Back to home' })}
          </button>
        </section>
      </div>
    );
  }

  const refresh = () => {
    void apps.refetch();
    void creators.refetch();
  };

  return (
    <div className="gl-page gl-admin-page">
      <div className="gl-admin-head">
        <div>
          <div className="gl-admin-kicker">
            <Shield size={16} />
            {t('admin.kicker', { defaultValue: 'Admin' })}
          </div>
          <h1>{t('admin.title', { defaultValue: 'Creator permissions' })}</h1>
        </div>
        <button type="button" className="gl-secondary-btn" onClick={refresh}>
          <RefreshCw size={16} />
          {t('admin.refresh', { defaultValue: 'Refresh' })}
        </button>
      </div>

      <div className="gl-admin-tabs" role="tablist" aria-label={t('admin.tabs.label', { defaultValue: 'Admin creator sections' })}>
        <button
          type="button"
          className={tab === 'permissions' ? 'is-active' : ''}
          onClick={() => setTab('permissions')}
        >
          {t('admin.tabs.permissions', { defaultValue: 'Permission users' })}
          <span>{creators.data?.items.length ?? 0}</span>
        </button>
        <button
          type="button"
          className={tab === 'applications' ? 'is-active' : ''}
          onClick={() => setTab('applications')}
        >
          {t('admin.tabs.applications', { defaultValue: 'Applications' })}
          <span>{apps.data?.items.filter((item) => item.status === 'pending').length ?? 0}</span>
        </button>
      </div>

      {tab === 'permissions' ? (
        <PermissionPanel
          items={creators.data?.items ?? []}
          loading={creators.isLoading}
          error={creators.isError}
          busy={updatePermission.isPending}
          onDisable={(item) => {
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
                onSuccess: () => toast.success(t('admin.permissions.disabled', { defaultValue: 'Live permission disabled.' })),
                onError: (err) => toast.error(err.message || t('admin.permissions.disableFailed', { defaultValue: 'Could not update permission.' })),
              },
            );
          }}
        />
      ) : (
        <ApplicationsPanel
          items={apps.data?.items ?? []}
          loading={apps.isLoading}
          error={apps.isError}
          approveBusy={approve.isPending}
          rejectBusy={reject.isPending}
          onApprove={(id) => {
            approve.mutate(
              { id },
              {
                onSuccess: () => toast.success(t('admin.applications.approved', { defaultValue: 'Application approved.' })),
                onError: (err) => toast.error(err.message || t('admin.applications.reviewFailed', { defaultValue: 'Review failed.' })),
              },
            );
          }}
          onReject={(id, reason) => {
            reject.mutate(
              { id, reason },
              {
                onSuccess: () => toast.success(t('admin.applications.rejected', { defaultValue: 'Application rejected.' })),
                onError: (err) => toast.error(err.message || t('admin.applications.reviewFailed', { defaultValue: 'Review failed.' })),
              },
            );
          }}
        />
      )}
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
        <div className="gl-yt-shelf-empty">{t('admin.permissions.loading', { defaultValue: 'Loading creators...' })}</div>
      ) : error ? (
        <div className="gl-yt-shelf-empty">{t('admin.permissions.error', { defaultValue: 'Could not load creators.' })}</div>
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
              <span className="gl-admin-status is-approved">{t('admin.status.approved', { defaultValue: 'Approved' })}</span>
              <button type="button" className="gl-admin-danger-btn" disabled={busy} onClick={() => onDisable(item)}>
                <Ban size={16} />
                {t('admin.permissions.disable', { defaultValue: 'Disable live' })}
              </button>
            </div>
          ))}
        </div>
      ) : (
        <div className="gl-yt-shelf-empty">{t('admin.permissions.empty', { defaultValue: 'No creators currently have live permission.' })}</div>
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
          <span>{t('admin.applications.eyebrow', { defaultValue: 'Application review' })}</span>
          <h2>{t('admin.applications.title', { defaultValue: 'Creator applications' })}</h2>
        </div>
      </div>
      {loading ? (
        <div className="gl-yt-shelf-empty">{t('admin.applications.loading', { defaultValue: 'Loading applications...' })}</div>
      ) : error ? (
        <div className="gl-yt-shelf-empty">{t('admin.applications.error', { defaultValue: 'Could not load applications.' })}</div>
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
                    name={app.displayName || app.username || t('admin.fallbackCreator', { defaultValue: 'Creator' })}
                    handle={`@${app.username || app.userId.slice(0, 8)}`}
                  />
                  <div className="gl-admin-review-meta">
                    <span className={`gl-admin-status is-${app.status}`}>{statusText(app.status, t)}</span>
                    <small>{formatDate(app.createdAt)}</small>
                  </div>
                </div>
                <div className="gl-admin-reason">
                  <span>{t('admin.applications.reason', { defaultValue: 'Live reason' })}</span>
                  <p>{app.reason || t('admin.applications.noReason', { defaultValue: 'No reason provided.' })}</p>
                </div>
                {app.rejectReason && (
                  <div className="gl-admin-reason is-reject">
                    <span>{t('admin.applications.rejectReason', { defaultValue: 'Reject reason' })}</span>
                    <p>{app.rejectReason}</p>
                  </div>
                )}
                {pending && (
                  <div className="gl-admin-review-actions">
                    <button type="button" className="gl-admin-action-text approve" disabled={busy} onClick={() => onApprove(app.id)}>
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
                      <span>{t('admin.applications.rejectReasonLabel', { defaultValue: 'Rejection reason' })}</span>
                      <textarea
                        rows={3}
                        value={rejectReason}
                        maxLength={500}
                        onChange={(event) => setRejectReason(event.target.value)}
                        placeholder={t('admin.applications.rejectReasonPlaceholder', { defaultValue: 'Tell the creator what needs to be improved.' })}
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
        <div className="gl-yt-shelf-empty">{t('admin.applications.empty', { defaultValue: 'No creator applications yet.' })}</div>
      )}
    </section>
  );
}

function AdminUser({
  avatar,
  name,
  handle,
}: {
  avatar?: string;
  name: string;
  handle: string;
}) {
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
