import { useNavigate } from 'react-router-dom';
import { AuthPanel } from '@/features/auth/AuthPanel';

export default function LoginPage() {
  const navigate = useNavigate();

  return (
    <div className="gl-auth-page">
      <div className="gl-auth-page-inner">
        <AuthPanel
          onAuthenticated={(resp) => navigate(resp.user.role === 'admin' ? '/admin' : '/')}
        />
        <aside className="gl-auth-preview" aria-label="GoLive preview">
          <img src="https://picsum.photos/seed/golive-auth/720/405" alt="" />
          <div className="gl-auth-preview-shade" />
          <div className="gl-auth-preview-top">
            <span className="gl-live-pill">LIVE</span>
            <span>38,214 watching</span>
          </div>
          <div className="gl-auth-preview-bottom">
            <div className="gl-auth-preview-title">Creator spotlight: first look stream</div>
            <div className="gl-auth-preview-meta">FUWAMOCO Ch. hololive-EN</div>
          </div>
        </aside>
      </div>
    </div>
  );
}
