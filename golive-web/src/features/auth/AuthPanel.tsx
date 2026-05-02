import type { FormEvent } from 'react';
import { useEffect, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AxiosError } from 'axios';
import { BadgeCheck, KeyRound, PlayCircle, Radio, UserRound } from 'lucide-react';
import { useLoginMutation, useRegisterMutation } from '@/api/auth';
import { GoLiveLogo } from '@/components/Logo';
import { cn } from '@/lib/cn';
import type { LoginResp } from '@/types/user';

type AuthMode = 'signin' | 'signup';

export interface AuthPanelProps {
  className?: string;
  onAuthenticated?: (resp: LoginResp) => void;
}

function authErrorMessage(
  err: Error,
  mode: AuthMode,
  t: ReturnType<typeof useTranslation>['t'],
): string {
  if (err instanceof AxiosError) {
    if (err.response?.status === 401) return t('auth.errors.invalidCredentials');
    if (err.response?.status === 409) return t('auth.errors.usernameTaken');
    if (err.response?.status === 400) return t('auth.errors.badFields');
  }
  return mode === 'signin'
    ? t('auth.errors.signInFailed')
    : t('auth.errors.signUpFailed');
}

export function AuthPanel({ className, onAuthenticated }: AuthPanelProps) {
  const { t } = useTranslation('common');
  const id = useId();
  const [mode, setMode] = useState<AuthMode>('signin');
  const [username, setUsername] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState<string | null>(null);

  const loginMut = useLoginMutation();
  const registerMut = useRegisterMutation();
  const isSigningUp = mode === 'signup';
  const isPending = isSigningUp ? registerMut.isPending : loginMut.isPending;
  const displayNameId = `${id}-display-name`;
  const usernameId = `${id}-username`;
  const passwordId = `${id}-password`;
  const confirmPasswordId = `${id}-confirm-password`;

  useEffect(() => {
    setError(null);
    loginMut.reset();
    registerMut.reset();
    setConfirmPassword('');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode]);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    setError(null);

    const cleanUsername = username.trim();
    const cleanDisplayName = displayName.trim();

    if (isSigningUp && password !== confirmPassword) {
      setError(t('auth.errors.passwordMismatch'));
      return;
    }

    const callbacks = {
      onSuccess: (resp: LoginResp) => {
        onAuthenticated?.(resp);
      },
      onError: (err: Error) => {
        setError(authErrorMessage(err, mode, t));
      },
    };

    if (isSigningUp) {
      registerMut.mutate(
        { username: cleanUsername, displayName: cleanDisplayName, password },
        callbacks,
      );
    } else {
      loginMut.mutate({ username: cleanUsername, password }, callbacks);
    }
  };

  return (
    <section className={cn('gl-auth-panel', className)}>
      <div className="gl-auth-head">
        <div className="gl-auth-brand" aria-hidden="true">
          <GoLiveLogo height={26} />
          <span className="gl-auth-brand-region">JP</span>
        </div>
        <div className="gl-auth-live">
          <span className="gl-live-dot-red" />
          <span>{t('auth.liveAccount')}</span>
        </div>
      </div>

      <div className="gl-auth-copy">
        <h1>{isSigningUp ? t('auth.signUpTitle') : t('auth.signInTitle')}</h1>
        <p>
          {isSigningUp
            ? t('auth.signUpSub')
            : t('auth.signInSub')}
        </p>
      </div>

      <div className="gl-auth-segment" role="tablist" aria-label={t('auth.mode')}>
        <button
          type="button"
          role="tab"
          aria-selected={!isSigningUp}
          className={cn('gl-auth-segment-btn', !isSigningUp && 'is-active')}
          onClick={() => setMode('signin')}
        >
          {t('auth.signIn')}
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={isSigningUp}
          className={cn('gl-auth-segment-btn', isSigningUp && 'is-active')}
          onClick={() => setMode('signup')}
        >
          {t('auth.register')}
        </button>
      </div>

      <form onSubmit={onSubmit} className="gl-auth-form">
        {isSigningUp && (
          <label className="gl-auth-field" htmlFor={displayNameId}>
            <span className="gl-auth-label">{t('auth.displayName')}</span>
            <span className="gl-auth-input-wrap">
              <BadgeCheck size={18} />
              <input
                id={displayNameId}
                type="text"
                autoComplete="name"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                placeholder={t('auth.displayNamePlaceholder')}
                required
              />
            </span>
          </label>
        )}

        <label className="gl-auth-field" htmlFor={usernameId}>
          <span className="gl-auth-label">{t('auth.username')}</span>
          <span className="gl-auth-input-wrap">
            <UserRound size={18} />
            <input
              id={usernameId}
              type="text"
              autoComplete="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder={isSigningUp ? t('auth.usernamePlaceholder') : 'demo'}
              minLength={3}
              required
            />
          </span>
        </label>

        <label className="gl-auth-field" htmlFor={passwordId}>
          <span className="gl-auth-label">{t('auth.password')}</span>
          <span className="gl-auth-input-wrap">
            <KeyRound size={18} />
            <input
              id={passwordId}
              type="password"
              autoComplete={isSigningUp ? 'new-password' : 'current-password'}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={isSigningUp ? t('auth.passwordPlaceholder') : 'demo'}
              minLength={3}
              required
            />
          </span>
        </label>

        {isSigningUp && (
          <label className="gl-auth-field" htmlFor={confirmPasswordId}>
            <span className="gl-auth-label">{t('auth.confirmPassword')}</span>
            <span className="gl-auth-input-wrap">
              <KeyRound size={18} />
              <input
                id={confirmPasswordId}
                type="password"
                autoComplete="new-password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                placeholder={t('auth.confirmPasswordPlaceholder')}
                minLength={3}
                required
              />
            </span>
          </label>
        )}

        {error && (
          <div className="gl-auth-error" role="alert">
            {error}
          </div>
        )}

        <button type="submit" disabled={isPending} className="gl-auth-submit">
          <PlayCircle size={18} />
          <span>
            {isPending
              ? isSigningUp
                ? t('auth.creating')
                : t('auth.signingIn')
              : isSigningUp
                ? t('auth.createAccount')
                : t('auth.signIn')}
          </span>
        </button>

        <button
          type="button"
          disabled
          className="gl-auth-secondary"
          title={t('auth.socialUnavailable')}
        >
          <Radio size={18} />
          <span>{t('auth.continueGoogle')}</span>
        </button>
      </form>
    </section>
  );
}
