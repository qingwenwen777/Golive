import { Navigate, useNavigate } from 'react-router-dom';
import { Check, Shield, X } from 'lucide-react';
import { toast } from 'sonner';
import {
  useAdminCreatorApplications,
  useReviewCreatorApplication,
  type CreatorApplication,
} from '@/api/creator';
import { Avatar } from '@/components/Avatar';
import { useAuthStore } from '@/stores/useAuthStore';

function statusText(status: CreatorApplication['status']) {
  if (status === 'pending') return 'Pending';
  if (status === 'approved') return 'Approved';
  if (status === 'rejected') return 'Rejected';
  return 'None';
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
  const navigate = useNavigate();
  const user = useAuthStore((s) => s.user);
  const isAdmin = user?.role === 'admin';
  const apps = useAdminCreatorApplications(isAdmin);
  const approve = useReviewCreatorApplication('approve');
  const reject = useReviewCreatorApplication('reject');

  if (!isAdmin) {
    return (
      <div className="gl-page gl-admin-page">
        <section className="gl-admin-denied">
          <Shield size={34} />
          <h1>No admin access</h1>
          <p>This area is only available to administrators.</p>
          <button type="button" className="gl-retry-btn" onClick={() => navigate('/')}>
            Back to home
          </button>
        </section>
      </div>
    );
  }

  const handleReview = (id: string, action: 'approve' | 'reject') => {
    const mut = action === 'approve' ? approve : reject;
    mut.mutate(id, {
      onSuccess: () =>
        toast.success(action === 'approve' ? 'Application approved.' : 'Application rejected.'),
      onError: (err) => toast.error(err.message || 'Review failed.'),
    });
  };

  return (
    <div className="gl-page gl-admin-page">
      <div className="gl-admin-head">
        <div>
          <div className="gl-admin-kicker">
            <Shield size={16} />
            Admin
          </div>
          <h1>Creator applications</h1>
        </div>
        <button type="button" className="gl-secondary-btn" onClick={() => apps.refetch()}>
          Refresh
        </button>
      </div>

      <section className="gl-admin-panel">
        {apps.isLoading ? (
          <div className="gl-yt-shelf-empty">Loading applications...</div>
        ) : apps.isError ? (
          <div className="gl-yt-shelf-empty">Could not load applications.</div>
        ) : apps.data?.items.length ? (
          <div className="gl-admin-table" role="table" aria-label="Creator applications">
            <div className="gl-admin-row gl-admin-row-head" role="row">
              <span>Creator</span>
              <span>Status</span>
              <span>Submitted</span>
              <span>Action</span>
            </div>
            {apps.data.items.map((app) => {
              const pending = app.status === 'pending';
              const busy = approve.isPending || reject.isPending;
              return (
                <div className="gl-admin-row" role="row" key={app.id}>
                  <div className="gl-admin-user">
                    <Avatar
                      name={app.displayName || app.username || app.userId}
                      src={app.avatar}
                      size={36}
                    />
                    <div>
                      <strong>{app.displayName || app.username || 'Creator'}</strong>
                      <span>@{app.username || app.userId.slice(0, 8)}</span>
                    </div>
                  </div>
                  <span className={`gl-admin-status is-${app.status}`}>
                    {statusText(app.status)}
                  </span>
                  <span className="gl-admin-muted">{formatDate(app.createdAt)}</span>
                  <div className="gl-admin-actions">
                    <button
                      type="button"
                      className="gl-admin-action approve"
                      disabled={!pending || busy}
                      onClick={() => handleReview(app.id, 'approve')}
                      aria-label="Approve application"
                    >
                      <Check size={16} />
                    </button>
                    <button
                      type="button"
                      className="gl-admin-action reject"
                      disabled={!pending || busy}
                      onClick={() => handleReview(app.id, 'reject')}
                      aria-label="Reject application"
                    >
                      <X size={16} />
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          <div className="gl-yt-shelf-empty">No creator applications yet.</div>
        )}
      </section>
    </div>
  );
}
