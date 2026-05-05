import type { FormEvent } from 'react';
import { useCallback, useEffect, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AxiosError } from 'axios';
import { BadgeCheck, KeyRound, Mail, PlayCircle, RefreshCw, Ticket, UserRound } from 'lucide-react';
import {
  fetchCaptcha,
  useGoogleLinkExistingMutation,
  useGoogleLoginMutation,
  useGoogleRegisterMutation,
  useLoginMutation,
  useRegisterMutation,
  useResetPasswordMutation,
} from '@/api/auth';
import { GoLiveLogo } from '@/components/Logo';
import { cn } from '@/lib/cn';
import {
  GoogleIdentityButton,
  decodeGoogleCredentialEmail,
  isGoogleConfigured,
} from '@/lib/googleIdentity';
import { GoogleIcon } from '@/components/GoogleIcon';
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog';
import type { LoginResp } from '@/types/user';

type AuthMode = 'signin' | 'signup' | 'reset';

const PASSWORD_RULE_TEXT = '至少 8 位，包含英文和数字';
const PASSWORD_PATTERN = '(?=.*[A-Za-z])(?=.*[0-9]).{8,}';
const PASSWORD_RULE_RE = /^(?=.*[A-Za-z])(?=.*\d).{8,}$/;

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
    const reason = (err.response?.data as { reason?: string } | undefined)?.reason;
    if (reason === 'invalid_captcha') {
      return t('auth.errors.invalidCaptcha', { defaultValue: 'Captcha is incorrect or expired.' });
    }
    if (reason === 'email_taken') {
      return t('auth.errors.emailTaken', {
        defaultValue: 'This email is already bound to another account.',
      });
    }
    if (reason === 'invalid_email') {
      return t('auth.errors.invalidEmail', { defaultValue: 'Enter a valid email address.' });
    }
    if (reason === 'invalid_invite') {
      return t('auth.errors.invalidInvite', { defaultValue: 'Invite code is invalid.' });
    }
    if (reason === 'invite_used') {
      return t('auth.errors.inviteUsed', { defaultValue: 'Invite code has already been used.' });
    }
    if (reason === 'email_not_found') {
      return t('auth.errors.emailNotFound', { defaultValue: 'No account is bound to this email.' });
    }
    if (reason === 'invalid_password') {
      return t('auth.errors.invalidPassword', { defaultValue: PASSWORD_RULE_TEXT });
    }
    if (reason === 'google_not_configured') {
      return t('auth.errors.googleNotConfigured', {
        defaultValue: 'Google sign-in is not configured yet.',
      });
    }
    if (reason === 'invalid_google_credential') {
      return t('auth.errors.invalidGoogleCredential', {
        defaultValue: 'Google sign-in could not be verified.',
      });
    }
    if (reason === 'google_account_not_found') {
      return t('auth.errors.googleAccountNotFound', {
        defaultValue: 'Use Register with an invite code before signing in with Google.',
      });
    }
    if (reason === 'google_already_linked') {
      return t('auth.errors.googleAlreadyLinked', {
        defaultValue: 'This Google account is already linked.',
      });
    }
    if (reason === 'google_email_exists') {
      return t('auth.errors.googleEmailExists', {
        defaultValue: 'This Google email already belongs to an existing GoLive account.',
      });
    }
    if (err.response?.status === 401) return t('auth.errors.invalidCredentials');
    if (err.response?.status === 409) return t('auth.errors.usernameTaken');
    if (err.response?.status === 400) return t('auth.errors.badFields');
  }
  if (mode === 'reset') {
    return t('auth.errors.resetFailed', {
      defaultValue: 'Password reset failed, please try again.',
    });
  }
  return mode === 'signin' ? t('auth.errors.signInFailed') : t('auth.errors.signUpFailed');
}

export function AuthPanel({ className, onAuthenticated }: AuthPanelProps) {
  const { t } = useTranslation('common');
  const id = useId();
  const [mode, setMode] = useState<AuthMode>('signin');
  const [username, setUsername] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [email, setEmail] = useState('');
  const [inviteCode, setInviteCode] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [resetEmail, setResetEmail] = useState('');
  const [resetPassword, setResetPassword] = useState('');
  const [resetConfirmPassword, setResetConfirmPassword] = useState('');
  const [captchaId, setCaptchaId] = useState('');
  const [captchaImage, setCaptchaImage] = useState('');
  const [captchaCode, setCaptchaCode] = useState('');
  const [googleLinkCredential, setGoogleLinkCredential] = useState('');
  const [googleLinkEmail, setGoogleLinkEmail] = useState('');
  const [googleLinkPassword, setGoogleLinkPassword] = useState('');
  const [googleRegisterOpen, setGoogleRegisterOpen] = useState(false);
  const [googleRegisterUsername, setGoogleRegisterUsername] = useState('');
  const [googleRegisterInviteCode, setGoogleRegisterInviteCode] = useState('');
  const [googleRegisterError, setGoogleRegisterError] = useState<string | null>(null);
  const [captchaLoading, setCaptchaLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const loginMut = useLoginMutation();
  const registerMut = useRegisterMutation();
  const resetPasswordMut = useResetPasswordMutation();
  const googleLoginMut = useGoogleLoginMutation();
  const googleRegisterMut = useGoogleRegisterMutation();
  const googleLinkExistingMut = useGoogleLinkExistingMutation();
  const isSigningUp = mode === 'signup';
  const isResetting = mode === 'reset';
  const isPending = isResetting
    ? resetPasswordMut.isPending
    : isSigningUp
      ? registerMut.isPending || googleRegisterMut.isPending || googleLinkExistingMut.isPending
      : loginMut.isPending || googleLoginMut.isPending || googleLinkExistingMut.isPending;
  const displayNameId = `${id}-display-name`;
  const usernameId = `${id}-username`;
  const emailId = `${id}-email`;
  const inviteCodeId = `${id}-invite-code`;
  const passwordId = `${id}-password`;
  const confirmPasswordId = `${id}-confirm-password`;
  const resetEmailId = `${id}-reset-email`;
  const resetPasswordId = `${id}-reset-password`;
  const resetConfirmPasswordId = `${id}-reset-confirm-password`;
  const captchaIdAttr = `${id}-captcha`;
  const googleRegisterUsernameId = `${id}-google-register-username`;
  const googleRegisterInviteCodeId = `${id}-google-register-invite`;

  const refreshCaptcha = useCallback(async () => {
    if (mode === 'reset') return;
    setCaptchaLoading(true);
    try {
      const challenge = await fetchCaptcha();
      setCaptchaId(challenge.id);
      setCaptchaImage(challenge.image);
      setCaptchaCode('');
    } catch {
      setCaptchaId('');
      setCaptchaImage('');
      setError(t('auth.errors.captchaLoadFailed', { defaultValue: 'Could not load captcha.' }));
    } finally {
      setCaptchaLoading(false);
    }
  }, [mode, t]);

  useEffect(() => {
    setError(null);
    loginMut.reset();
    registerMut.reset();
    resetPasswordMut.reset();
    googleLoginMut.reset();
    googleRegisterMut.reset();
    googleLinkExistingMut.reset();
    setConfirmPassword('');
    setResetConfirmPassword('');
    setGoogleLinkCredential('');
    setGoogleLinkEmail('');
    setGoogleLinkPassword('');
    setGoogleRegisterOpen(false);
    setGoogleRegisterError(null);
    if (mode === 'reset') {
      setCaptchaId('');
      setCaptchaImage('');
      setCaptchaCode('');
      return;
    }
    void refreshCaptcha();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode]);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    setSuccess(null);

    if (isResetting) {
      if (!PASSWORD_RULE_RE.test(resetPassword)) {
        setError(t('auth.errors.invalidPassword', { defaultValue: PASSWORD_RULE_TEXT }));
        return;
      }
      if (resetPassword !== resetConfirmPassword) {
        setError(t('auth.errors.passwordMismatch'));
        return;
      }
      resetPasswordMut.mutate(
        { email: resetEmail.trim(), newPassword: resetPassword },
        {
          onSuccess: () => {
            setSuccess(
              t('auth.resetSuccess', {
                defaultValue: 'Password has been reset. You can sign in now.',
              }),
            );
            setMode('signin');
            setPassword('');
            setResetPassword('');
            setResetConfirmPassword('');
          },
          onError: (err: Error) => {
            setError(authErrorMessage(err, mode, t));
          },
        },
      );
      return;
    }

    const cleanUsername = username.trim();
    const cleanDisplayName = displayName.trim();
    const cleanEmail = email.trim();
    const cleanInviteCode = inviteCode.trim();
    const cleanCaptcha = captchaCode.trim();

    if (!captchaId || !cleanCaptcha) {
      setError(
        t('auth.errors.captchaRequired', { defaultValue: 'Enter the captcha to continue.' }),
      );
      return;
    }

    if (isSigningUp && password !== confirmPassword) {
      setError(t('auth.errors.passwordMismatch'));
      return;
    }
    if (isSigningUp && !PASSWORD_RULE_RE.test(password)) {
      setError(t('auth.errors.invalidPassword', { defaultValue: PASSWORD_RULE_TEXT }));
      return;
    }

    const callbacks = {
      onSuccess: (resp: LoginResp) => {
        onAuthenticated?.(resp);
      },
      onError: (err: Error) => {
        setError(authErrorMessage(err, mode, t));
        void refreshCaptcha();
      },
    };

    if (isSigningUp) {
      registerMut.mutate(
        {
          username: cleanUsername,
          displayName: cleanDisplayName,
          email: cleanEmail,
          inviteCode: cleanInviteCode,
          password,
          captchaId,
          captchaCode: cleanCaptcha,
        },
        callbacks,
      );
    } else {
      loginMut.mutate(
        { username: cleanUsername, password, captchaId, captchaCode: cleanCaptcha },
        callbacks,
      );
    }
  };

  const showGoogleLinkPrompt = useCallback((credential: string, err: Error): boolean => {
    if (!(err instanceof AxiosError)) return false;
    const data = err.response?.data as { reason?: string; email?: string } | undefined;
    if (data?.reason !== 'google_email_exists') return false;
    setGoogleLinkCredential(credential);
    setGoogleLinkEmail(data.email || decodeGoogleCredentialEmail(credential));
    setGoogleLinkPassword('');
    setError(null);
    setSuccess(null);
    return true;
  }, []);

  const handleGoogleCredential = useCallback(
    (credential: string) => {
      if (isResetting || isSigningUp) return;
      setError(null);
      setSuccess(null);
      setGoogleLinkCredential('');
      setGoogleLinkPassword('');

      const callbacks = {
        onSuccess: (resp: LoginResp) => {
          onAuthenticated?.(resp);
        },
        onError: (err: Error) => {
          if (showGoogleLinkPrompt(credential, err)) return;
          setError(authErrorMessage(err, mode, t));
        },
      };

      googleLoginMut.mutate({ credential }, callbacks);
    },
    [googleLoginMut, isResetting, isSigningUp, mode, onAuthenticated, showGoogleLinkPrompt, t],
  );

  const openGoogleRegisterDialog = () => {
    setError(null);
    setSuccess(null);
    setGoogleRegisterError(null);
    setGoogleRegisterUsername(username.trim());
    setGoogleRegisterInviteCode(inviteCode.trim().toUpperCase());
    setGoogleRegisterOpen(true);
  };

  const handleGoogleRegisterCredential = useCallback(
    (credential: string) => {
      const cleanUsername = googleRegisterUsername.trim();
      const cleanInviteCode = googleRegisterInviteCode.trim().toUpperCase();
      setGoogleRegisterError(null);
      if (!cleanInviteCode) {
        setGoogleRegisterError(
          t('auth.errors.inviteRequired', {
            defaultValue: 'Enter an invite code before continuing with Google.',
          }),
        );
        return;
      }
      if (!/^[A-Za-z0-9][A-Za-z0-9_.-]{2,31}$/.test(cleanUsername)) {
        setGoogleRegisterError(
          t('auth.errors.invalidUsername', {
            defaultValue: 'Choose a valid username before continuing with Google.',
          }),
        );
        return;
      }
      googleRegisterMut.mutate(
        {
          credential,
          username: cleanUsername,
          displayName: '',
          inviteCode: cleanInviteCode,
        },
        {
          onSuccess: (resp) => {
            setGoogleRegisterOpen(false);
            onAuthenticated?.(resp);
          },
          onError: (err: Error) => {
            if (showGoogleLinkPrompt(credential, err)) {
              setGoogleRegisterOpen(false);
              return;
            }
            setGoogleRegisterError(authErrorMessage(err, 'signup', t));
          },
        },
      );
    },
    [
      googleRegisterInviteCode,
      googleRegisterMut,
      googleRegisterUsername,
      onAuthenticated,
      showGoogleLinkPrompt,
      t,
    ],
  );

  const handleGoogleLinkExisting = () => {
    if (!googleLinkCredential) return;
    if (!googleLinkPassword) {
      setError(
        t('auth.errors.passwordRequired', {
          defaultValue: 'Enter the existing account password to link Google.',
        }),
      );
      return;
    }
    setError(null);
    googleLinkExistingMut.mutate(
      { credential: googleLinkCredential, password: googleLinkPassword },
      {
        onSuccess: (resp) => {
          setGoogleLinkCredential('');
          setGoogleLinkPassword('');
          onAuthenticated?.(resp);
        },
        onError: (err: Error) => setError(authErrorMessage(err, mode, t)),
      },
    );
  };

  const handleGoogleUnavailable = useCallback(
    () =>
      setError(
        t('auth.errors.googleLoadFailed', { defaultValue: 'Could not load Google sign-in.' }),
      ),
    [t],
  );

  const googleRegisterReady =
    /^[A-Za-z0-9][A-Za-z0-9_.-]{2,31}$/.test(googleRegisterUsername.trim()) &&
    googleRegisterInviteCode.trim().length > 0 &&
    !googleRegisterMut.isPending;

  const title = isResetting
    ? t('auth.resetTitle', { defaultValue: 'Reset password' })
    : isSigningUp
      ? t('auth.signUpTitle')
      : t('auth.signInTitle');
  const sub = isResetting
    ? t('auth.resetSub', {
        defaultValue: 'Use the email bound to your account to set a new password.',
      })
    : isSigningUp
      ? t('auth.signUpSub')
      : t('auth.signInSub');

  return (
    <section
      className={cn(
        'gl-auth-panel',
        isSigningUp && 'is-signup',
        isResetting && 'is-reset',
        className,
      )}
    >
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
        <h1>{title}</h1>
        <p>{sub}</p>
      </div>

      {!isResetting && (
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
      )}

      <form onSubmit={onSubmit} className="gl-auth-form">
        {isResetting ? (
          <>
            <label className="gl-auth-field" htmlFor={resetEmailId}>
              <span className="gl-auth-label">{t('auth.email', { defaultValue: 'Email' })}</span>
              <span className="gl-auth-input-wrap">
                <Mail size={18} />
                <input
                  id={resetEmailId}
                  type="email"
                  autoComplete="email"
                  value={resetEmail}
                  onChange={(e) => setResetEmail(e.target.value)}
                  placeholder={t('auth.emailPlaceholder', { defaultValue: 'you@example.com' })}
                  required
                />
              </span>
            </label>
            <label className="gl-auth-field" htmlFor={resetPasswordId}>
              <span className="gl-auth-label">
                {t('auth.newPassword', { defaultValue: 'New password' })}
              </span>
              <span className="gl-auth-input-wrap">
                <KeyRound size={18} />
                <input
                  id={resetPasswordId}
                  type="password"
                  autoComplete="new-password"
                  value={resetPassword}
                  onChange={(e) => setResetPassword(e.target.value)}
                  placeholder={PASSWORD_RULE_TEXT}
                  minLength={8}
                  pattern={PASSWORD_PATTERN}
                  title={PASSWORD_RULE_TEXT}
                  required
                />
              </span>
            </label>
            <label className="gl-auth-field" htmlFor={resetConfirmPasswordId}>
              <span className="gl-auth-label">{t('auth.confirmPassword')}</span>
              <span className="gl-auth-input-wrap">
                <KeyRound size={18} />
                <input
                  id={resetConfirmPasswordId}
                  type="password"
                  autoComplete="new-password"
                  value={resetConfirmPassword}
                  onChange={(e) => setResetConfirmPassword(e.target.value)}
                  placeholder={t('auth.confirmPasswordPlaceholder')}
                  minLength={8}
                  pattern={PASSWORD_PATTERN}
                  title={PASSWORD_RULE_TEXT}
                  required
                />
              </span>
            </label>
          </>
        ) : (
          <>
            {isSigningUp && (
              <>
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
                <label className="gl-auth-field" htmlFor={emailId}>
                  <span className="gl-auth-label">
                    {t('auth.email', { defaultValue: 'Email' })}
                  </span>
                  <span className="gl-auth-input-wrap">
                    <Mail size={18} />
                    <input
                      id={emailId}
                      type="email"
                      autoComplete="email"
                      value={email}
                      onChange={(e) => setEmail(e.target.value)}
                      placeholder={t('auth.emailPlaceholder', { defaultValue: 'you@example.com' })}
                      required
                    />
                  </span>
                </label>
                <label className="gl-auth-field" htmlFor={inviteCodeId}>
                  <span className="gl-auth-label">
                    {t('auth.inviteCode', { defaultValue: 'Invite code' })}
                  </span>
                  <span className="gl-auth-input-wrap">
                    <Ticket size={18} />
                    <input
                      id={inviteCodeId}
                      type="text"
                      autoComplete="one-time-code"
                      value={inviteCode}
                      onChange={(e) => setInviteCode(e.target.value.toUpperCase())}
                      placeholder={t('auth.inviteCodePlaceholder', {
                        defaultValue: 'Enter invite code',
                      })}
                      required
                    />
                  </span>
                </label>
              </>
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
                  placeholder={isSigningUp ? PASSWORD_RULE_TEXT : 'demo'}
                  minLength={isSigningUp ? 8 : 3}
                  pattern={isSigningUp ? PASSWORD_PATTERN : undefined}
                  title={isSigningUp ? PASSWORD_RULE_TEXT : undefined}
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
                    minLength={8}
                    pattern={PASSWORD_PATTERN}
                    title={PASSWORD_RULE_TEXT}
                    required
                  />
                </span>
              </label>
            )}

            <label className="gl-auth-field" htmlFor={captchaIdAttr}>
              <span className="gl-auth-label">
                {t('auth.captcha', { defaultValue: 'Captcha' })}
              </span>
              <span className="gl-auth-captcha-row">
                <span className="gl-auth-input-wrap">
                  <input
                    id={captchaIdAttr}
                    type="text"
                    value={captchaCode}
                    onChange={(e) => setCaptchaCode(e.target.value.toUpperCase())}
                    placeholder={t('auth.captchaPlaceholder', { defaultValue: 'Code' })}
                    autoComplete="off"
                    required
                  />
                </span>
                <button
                  type="button"
                  className="gl-auth-captcha-image"
                  onClick={() => void refreshCaptcha()}
                  disabled={captchaLoading}
                  title={t('auth.refreshCaptcha', { defaultValue: 'Refresh captcha' })}
                >
                  {captchaImage ? (
                    <img src={captchaImage} alt={t('auth.captcha', { defaultValue: 'Captcha' })} />
                  ) : (
                    <RefreshCw size={18} />
                  )}
                </button>
              </span>
            </label>
          </>
        )}

        {error && (
          <div className="gl-auth-error" role="alert">
            {error}
          </div>
        )}
        {success && (
          <div className="gl-auth-success" role="status">
            {success}
          </div>
        )}

        {googleLinkCredential && (
          <div className="gl-auth-google-link" role="status">
            <strong>
              {t('auth.googleLinkExistingTitle', { defaultValue: 'Google email already exists' })}
            </strong>
            <p>
              {t('auth.googleLinkExistingSub', {
                email: googleLinkEmail,
                defaultValue:
                  '{{email}} is already used by a GoLive account. Enter that account password to link Google, or cancel.',
              })}
            </p>
            <label className="gl-auth-field">
              <span className="gl-auth-label">
                {t('auth.existingPassword', { defaultValue: 'Existing account password' })}
              </span>
              <span className="gl-auth-input-wrap">
                <KeyRound size={18} />
                <input
                  type="password"
                  autoComplete="current-password"
                  value={googleLinkPassword}
                  onChange={(event) => setGoogleLinkPassword(event.target.value)}
                  placeholder={t('auth.password')}
                />
              </span>
            </label>
            <div className="gl-auth-google-link-actions">
              <button
                type="button"
                className="gl-auth-submit"
                disabled={googleLinkExistingMut.isPending}
                onClick={handleGoogleLinkExisting}
              >
                {googleLinkExistingMut.isPending
                  ? t('auth.linkingGoogle', { defaultValue: 'Linking...' })
                  : t('auth.linkGoogle', { defaultValue: 'Link Google' })}
              </button>
              <button
                type="button"
                className="gl-auth-link-btn"
                onClick={() => {
                  setGoogleLinkCredential('');
                  setGoogleLinkPassword('');
                }}
              >
                {t('auth.cancelGoogleLink', { defaultValue: 'Cancel' })}
              </button>
            </div>
          </div>
        )}

        <button type="submit" disabled={isPending} className="gl-auth-submit">
          <PlayCircle size={18} />
          <span>
            {isPending
              ? isResetting
                ? t('auth.resetting', { defaultValue: 'Resetting...' })
                : isSigningUp
                  ? t('auth.creating')
                  : t('auth.signingIn')
              : isResetting
                ? t('auth.resetPassword', { defaultValue: 'Reset password' })
                : isSigningUp
                  ? t('auth.createAccount')
                  : t('auth.signIn')}
          </span>
        </button>

        {!isResetting && (
          <button
            type="button"
            className="gl-auth-link-btn"
            onClick={() => {
              setResetEmail(email);
              setMode('reset');
            }}
          >
            {t('auth.forgotPassword', { defaultValue: 'Forgot password?' })}
          </button>
        )}

        {isResetting ? (
          <button type="button" className="gl-auth-secondary" onClick={() => setMode('signin')}>
            <UserRound size={18} />
            <span>{t('auth.backToSignIn', { defaultValue: 'Back to sign in' })}</span>
          </button>
        ) : isSigningUp ? (
          <button
            type="button"
            className="gl-auth-secondary gl-auth-google-register-trigger"
            disabled={isPending}
            onClick={openGoogleRegisterDialog}
          >
            <GoogleIcon className="gl-google-letter-mark" />
            <span>{t('auth.googleRegisterCta', { defaultValue: 'Sign up with Google' })}</span>
          </button>
        ) : (
          <GoogleIdentityButton
            text="continue_with"
            disabled={isPending}
            fallbackLabel={
              isGoogleConfigured() ? t('auth.continueGoogle') : t('auth.socialUnavailable')
            }
            onCredential={handleGoogleCredential}
            onUnavailable={handleGoogleUnavailable}
          />
        )}
      </form>

      <Dialog
        open={googleRegisterOpen}
        onOpenChange={(open) => {
          setGoogleRegisterOpen(open);
          if (!open) setGoogleRegisterError(null);
        }}
      >
        <DialogContent className="gl-google-register-dialog p-0 sm:max-w-[380px]">
          <div className="gl-google-register-body">
            <GoogleIcon size="large" />
            <DialogTitle>
              {t('auth.googleRegisterDialogTitle', {
                defaultValue: 'Register with Google',
              })}
            </DialogTitle>
            <DialogDescription>
              {t('auth.googleRegisterDialogSub', {
                defaultValue: 'Enter a username and invite code, then continue with Google.',
              })}
            </DialogDescription>
            <label className="gl-auth-field" htmlFor={googleRegisterUsernameId}>
              <span className="gl-auth-label">{t('auth.username')}</span>
              <span className="gl-auth-input-wrap">
                <UserRound size={18} />
                <input
                  id={googleRegisterUsernameId}
                  type="text"
                  autoComplete="username"
                  value={googleRegisterUsername}
                  onChange={(event) => setGoogleRegisterUsername(event.target.value)}
                  placeholder={t('auth.usernamePlaceholder')}
                  minLength={3}
                />
              </span>
            </label>
            <label className="gl-auth-field" htmlFor={googleRegisterInviteCodeId}>
              <span className="gl-auth-label">
                {t('auth.inviteCode', { defaultValue: 'Invite code' })}
              </span>
              <span className="gl-auth-input-wrap">
                <Ticket size={18} />
                <input
                  id={googleRegisterInviteCodeId}
                  type="text"
                  autoComplete="one-time-code"
                  value={googleRegisterInviteCode}
                  onChange={(event) =>
                    setGoogleRegisterInviteCode(event.target.value.toUpperCase())
                  }
                  placeholder={t('auth.inviteCodePlaceholder', {
                    defaultValue: 'Enter invite code',
                  })}
                />
              </span>
            </label>
            {googleRegisterError && (
              <div className="gl-auth-error" role="alert">
                {googleRegisterError}
              </div>
            )}
            <GoogleIdentityButton
              className="gl-google-register-button"
              text="signup_with"
              disabled={!googleRegisterReady}
              fallbackLabel={
                !isGoogleConfigured()
                  ? t('auth.socialUnavailable')
                  : googleRegisterReady
                    ? t('auth.continueGoogle')
                    : t('auth.googleRegisterFillFirst', {
                        defaultValue: 'Fill username and invite code first',
                      })
              }
              onCredential={handleGoogleRegisterCredential}
              onUnavailable={(message) =>
                setGoogleRegisterError(
                  message ||
                    t('auth.errors.googleLoadFailed', {
                      defaultValue: 'Could not load Google sign-in.',
                    }),
                )
              }
            />
          </div>
        </DialogContent>
      </Dialog>
    </section>
  );
}
