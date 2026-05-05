export function GoogleIcon({ size, className }: { size?: 'small' | 'large'; className?: string }) {
  const classes = [
    'gl-google-provider-icon',
    size === 'small' && 'is-small',
    size === 'large' && 'is-large',
    className,
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <span className={classes} aria-hidden="true">
      <svg viewBox="0 0 18 18" focusable="false">
        <path
          fill="#4285f4"
          d="M17.64 9.2c0-.64-.06-1.25-.17-1.84H9v3.48h4.84a4.14 4.14 0 0 1-1.8 2.72v2.26h2.91c1.7-1.57 2.69-3.88 2.69-6.62z"
        />
        <path
          fill="#34a853"
          d="M9 18c2.43 0 4.47-.8 5.96-2.18l-2.91-2.26c-.81.54-1.84.86-3.05.86-2.35 0-4.33-1.58-5.04-3.71H.96v2.33A9 9 0 0 0 9 18z"
        />
        <path
          fill="#fbbc05"
          d="M3.96 10.71A5.41 5.41 0 0 1 3.68 9c0-.59.1-1.17.28-1.71V4.96H.96A9 9 0 0 0 0 9c0 1.45.35 2.82.96 4.04l3-2.33z"
        />
        <path
          fill="#ea4335"
          d="M9 3.58c1.32 0 2.5.45 3.43 1.35l2.58-2.58C13.46.9 11.42 0 9 0A9 9 0 0 0 .96 4.96l3 2.33C4.67 5.16 6.65 3.58 9 3.58z"
        />
      </svg>
    </span>
  );
}
