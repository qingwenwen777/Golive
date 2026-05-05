import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { LogOut, ShieldAlert, Send } from 'lucide-react';
import { toast } from 'sonner';
import { logout as doLogout, useMe } from '@/api/auth';
import { useCreateUnbanAppeal } from '@/api/contentModeration';
import { Avatar } from '@/components/Avatar';
import { useAuthStore, useIsAuthed } from '@/stores/useAuthStore';

export default function BannedAccountPage() {
  const { t } = useTranslation('pages');
  const navigate = useNavigate();
  const isAuthed = useIsAuthed();
  const storedUser = useAuthStore((s) => s.user);
  const me = useMe();
  const user = isAuthed ? (me.data ?? storedUser) : null;
  const createAppeal = useCreateUnbanAppeal();
  const [reason, setReason] = useState('');

  useEffect(() => {
    if (!isAuthed) {
      navigate('/login', { replace: true });
    } else if (user && !user.banned) {
      navigate('/', { replace: true });
    }
  }, [isAuthed, navigate, user]);

  const submitAppeal = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const cleanReason = reason.trim();
    if (cleanReason.length < 10) {
      toast.error(
        t('bannedAccount.appealReasonRequired', {
          defaultValue: 'Please enter at least 10 characters.',
        }),
      );
      return;
    }
    createAppeal.mutate(
      { reason: cleanReason },
      {
        onSuccess: () => {
          setReason('');
          toast.success(
            t('bannedAccount.appealSubmitted', {
              defaultValue: 'Appeal submitted. We will review it as soon as possible.',
            }),
          );
        },
        onError: (err) =>
          toast.error(
            err.message ||
              t('bannedAccount.appealFailed', {
                defaultValue: 'Could not submit appeal.',
              }),
          ),
      },
    );
  };

  return (
    <div className="gl-banned-page">
      <section className="gl-banned-panel">
        <div className="gl-banned-icon" aria-hidden="true">
          <ShieldAlert size={34} />
        </div>
        <div className="gl-banned-head">
          <h1>{t('bannedAccount.title', { defaultValue: 'Account restricted' })}</h1>
          <p>
            {t('bannedAccount.subtitle', {
              defaultValue:
                'You can still sign in to review the restriction and submit an appeal. Other GoLive features are unavailable while the account is banned.',
            })}
          </p>
        </div>

        {user && (
          <div className="gl-banned-user">
            <Avatar name={user.displayName || user.username} src={user.avatar} size={44} />
            <div>
              <strong>{user.displayName || user.username}</strong>
              <span>@{user.username}</span>
            </div>
          </div>
        )}

        <div className="gl-banned-reason">
          <span>{t('bannedAccount.reasonLabel', { defaultValue: 'Restriction reason' })}</span>
          <p>
            {user?.banReason ||
              t('bannedAccount.noReason', {
                defaultValue: 'No detailed reason was provided by the moderation team.',
              })}
          </p>
        </div>

        <form className="gl-banned-appeal" onSubmit={submitAppeal}>
          <label>
            <span>{t('bannedAccount.appealReason', { defaultValue: 'Appeal reason' })}</span>
            <textarea
              value={reason}
              onChange={(event) => setReason(event.target.value)}
              maxLength={1000}
              placeholder={t('bannedAccount.appealPlaceholder', {
                defaultValue:
                  'Explain what happened and why the restriction should be reviewed.',
              })}
            />
            <small>{reason.length}/1000</small>
          </label>
          <div className="gl-banned-actions">
            <button
              className="gl-settings-button is-primary"
              type="submit"
              disabled={createAppeal.isPending || reason.trim().length < 10}
            >
              <Send size={15} />
              {createAppeal.isPending
                ? t('bannedAccount.appealSending', { defaultValue: 'Submitting...' })
                : t('bannedAccount.appealSubmit', { defaultValue: 'Submit appeal' })}
            </button>
            <button
              className="gl-settings-button"
              type="button"
              onClick={() => {
                void doLogout().finally(() => navigate('/login', { replace: true }));
              }}
            >
              <LogOut size={15} />
              {t('bannedAccount.signOut', { defaultValue: 'Sign out' })}
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}
