import type { FormEvent } from 'react';
import { useEffect, useId, useState } from 'react';
import { AxiosError } from 'axios';
import { BadgeCheck, KeyRound, PlayCircle, Radio, UserRound } from 'lucide-react';
import { useLoginMutation, useRegisterMutation } from '@/api/auth';
import { GoLiveLogo } from '@/components/Logo';
import { cn } from '@/lib/cn';

type AuthMode = 'signin' | 'signup';

export interface AuthPanelProps {
  className?: string;
  onAuthenticated?: () => void;
}

function authErrorMessage(err: Error, mode: AuthMode): string {
  if (err instanceof AxiosError) {
    if (err.response?.status === 401) return 'Invalid username or password.';
    if (err.response?.status === 409) return 'That username is already taken.';
    if (err.response?.status === 400) return 'Check the fields and try again.';
  }
  return mode === 'signin'
    ? 'Sign-in failed. Please try again.'
    : 'Account creation failed. Please try again.';
}

export function AuthPanel({ className, onAuthenticated }: AuthPanelProps) {
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
      setError('Passwords do not match.');
      return;
    }

    const callbacks = {
      onSuccess: () => {
        onAuthenticated?.();
      },
      onError: (err: Error) => {
        setError(authErrorMessage(err, mode));
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
          <GoLiveLogo height={22} />
          <span>GoLive</span>
          <span className="gl-auth-brand-region">JP</span>
        </div>
        <div className="gl-auth-live">
          <span className="gl-live-dot-red" />
          <span>Live account</span>
        </div>
      </div>

      <div className="gl-auth-copy">
        <h1>{isSigningUp ? 'Create your GoLive account' : 'Sign in to GoLive'}</h1>
        <p>
          {isSigningUp
            ? 'Pick a channel name and jump into live chat, gifts, and follows.'
            : 'Use demo / demo for the preview build, or create a local preview account.'}
        </p>
      </div>

      <div className="gl-auth-segment" role="tablist" aria-label="Authentication mode">
        <button
          type="button"
          role="tab"
          aria-selected={!isSigningUp}
          className={cn('gl-auth-segment-btn', !isSigningUp && 'is-active')}
          onClick={() => setMode('signin')}
        >
          Sign in
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={isSigningUp}
          className={cn('gl-auth-segment-btn', isSigningUp && 'is-active')}
          onClick={() => setMode('signup')}
        >
          Register
        </button>
      </div>

      <form onSubmit={onSubmit} className="gl-auth-form">
        {isSigningUp && (
          <label className="gl-auth-field" htmlFor={displayNameId}>
            <span className="gl-auth-label">Display name</span>
            <span className="gl-auth-input-wrap">
              <BadgeCheck size={18} />
              <input
                id={displayNameId}
                type="text"
                autoComplete="name"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                placeholder="Your channel name"
                required
              />
            </span>
          </label>
        )}

        <label className="gl-auth-field" htmlFor={usernameId}>
          <span className="gl-auth-label">Username</span>
          <span className="gl-auth-input-wrap">
            <UserRound size={18} />
            <input
              id={usernameId}
              type="text"
              autoComplete="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder={isSigningUp ? 'Choose a username' : 'demo'}
              minLength={3}
              required
            />
          </span>
        </label>

        <label className="gl-auth-field" htmlFor={passwordId}>
          <span className="gl-auth-label">Password</span>
          <span className="gl-auth-input-wrap">
            <KeyRound size={18} />
            <input
              id={passwordId}
              type="password"
              autoComplete={isSigningUp ? 'new-password' : 'current-password'}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={isSigningUp ? 'At least 3 characters' : 'demo'}
              minLength={3}
              required
            />
          </span>
        </label>

        {isSigningUp && (
          <label className="gl-auth-field" htmlFor={confirmPasswordId}>
            <span className="gl-auth-label">Confirm password</span>
            <span className="gl-auth-input-wrap">
              <KeyRound size={18} />
              <input
                id={confirmPasswordId}
                type="password"
                autoComplete="new-password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                placeholder="Repeat password"
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
                ? 'Creating...'
                : 'Signing in...'
              : isSigningUp
                ? 'Create account'
                : 'Sign in'}
          </span>
        </button>

        <button
          type="button"
          disabled
          className="gl-auth-secondary"
          title="Social sign-in is not available in this preview"
        >
          <Radio size={18} />
          <span>Continue with Google</span>
        </button>
      </form>
    </section>
  );
}
