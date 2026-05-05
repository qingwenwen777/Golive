import { useEffect, useRef, useState } from 'react';

const GOOGLE_IDENTITY_SRC = 'https://accounts.google.com/gsi/client';

type GoogleButtonText = 'signin_with' | 'signup_with' | 'continue_with';

interface GoogleCredentialResponse {
  credential?: string;
}

interface GoogleAccountsID {
  initialize: (config: {
    client_id: string;
    callback: (response: GoogleCredentialResponse) => void;
    auto_select?: boolean;
    cancel_on_tap_outside?: boolean;
  }) => void;
  renderButton: (
    element: HTMLElement,
    config: {
      type?: 'standard' | 'icon';
      theme?: 'outline' | 'filled_blue' | 'filled_black';
      size?: 'large' | 'medium' | 'small';
      shape?: 'rectangular' | 'pill' | 'circle' | 'square';
      text?: GoogleButtonText;
      logo_alignment?: 'left' | 'center';
      width?: number;
    },
  ) => void;
}

declare global {
  interface Window {
    google?: {
      accounts?: {
        id?: GoogleAccountsID;
      };
    };
  }
}

let scriptPromise: Promise<void> | null = null;

export function googleClientId(): string {
  return (import.meta.env.VITE_GOOGLE_CLIENT_ID || '').trim();
}

export function isGoogleConfigured(): boolean {
  return googleClientId().length > 0;
}

export function decodeGoogleCredentialEmail(credential: string): string {
  try {
    const [, payload] = credential.split('.');
    if (!payload) return '';
    const normalized = payload.replace(/-/g, '+').replace(/_/g, '/');
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=');
    const json = JSON.parse(window.atob(padded)) as {
      email?: string;
    };
    return typeof json.email === 'string' ? json.email : '';
  } catch {
    return '';
  }
}

function loadGoogleIdentity(): Promise<void> {
  if (window.google?.accounts?.id) return Promise.resolve();
  if (scriptPromise) return scriptPromise;
  scriptPromise = new Promise((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>(
      `script[src="${GOOGLE_IDENTITY_SRC}"]`,
    );
    if (existing) {
      existing.addEventListener('load', () => resolve(), { once: true });
      existing.addEventListener(
        'error',
        () => reject(new Error('Google Identity failed to load')),
        {
          once: true,
        },
      );
      return;
    }
    const script = document.createElement('script');
    script.src = GOOGLE_IDENTITY_SRC;
    script.async = true;
    script.defer = true;
    script.onload = () => resolve();
    script.onerror = () => reject(new Error('Google Identity failed to load'));
    document.head.appendChild(script);
  });
  return scriptPromise;
}

export function GoogleIdentityButton({
  text = 'continue_with',
  disabled,
  fallbackLabel,
  onCredential,
  onUnavailable,
}: {
  text?: GoogleButtonText;
  disabled?: boolean;
  fallbackLabel: string;
  onCredential: (credential: string) => void;
  onUnavailable?: (message: string) => void;
}) {
  const ref = useRef<HTMLDivElement | null>(null);
  const latestCredentialHandler = useRef(onCredential);
  const [ready, setReady] = useState(false);
  const clientId = googleClientId();

  useEffect(() => {
    latestCredentialHandler.current = onCredential;
  }, [onCredential]);

  useEffect(() => {
    const node = ref.current;
    if (!node || disabled || !clientId) return;
    let cancelled = false;
    node.innerHTML = '';

    loadGoogleIdentity()
      .then(() => {
        if (cancelled || !node || !window.google?.accounts?.id) return;
        window.google.accounts.id.initialize({
          client_id: clientId,
          auto_select: false,
          cancel_on_tap_outside: true,
          callback: (response) => {
            if (response.credential) latestCredentialHandler.current(response.credential);
          },
        });
        window.google.accounts.id.renderButton(node, {
          type: 'standard',
          theme: 'outline',
          size: 'large',
          shape: 'pill',
          text,
          logo_alignment: 'left',
          width: Math.max(220, Math.floor(node.getBoundingClientRect().width || 320)),
        });
        setReady(true);
      })
      .catch((err: Error) => {
        if (!cancelled) onUnavailable?.(err.message);
      });

    return () => {
      cancelled = true;
      if (node) node.innerHTML = '';
      setReady(false);
    };
  }, [clientId, disabled, onUnavailable, text]);

  if (!clientId || disabled) {
    return (
      <button type="button" className="gl-google-fallback" disabled>
        {fallbackLabel}
      </button>
    );
  }

  return (
    <div className="gl-google-button-shell">
      <div ref={ref} className="gl-google-button" aria-label={fallbackLabel} />
      {!ready && (
        <button type="button" className="gl-google-fallback" disabled>
          {fallbackLabel}
        </button>
      )}
    </div>
  );
}
