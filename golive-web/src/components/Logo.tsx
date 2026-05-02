interface LogoProps {
  height?: number;
  variant?: 'mark' | 'wordmark';
}

const WORDMARK_ASPECT = 220 / 60;

export function GoLiveLogo({ height = 24, variant = 'wordmark' }: LogoProps) {
  if (variant === 'mark') {
    return (
      <img
        src="/golive-icon.svg"
        alt=""
        aria-hidden="true"
        draggable={false}
        style={{ width: height, height, display: 'block' }}
      />
    );
  }

  return (
    <span
      className="gl-brand-logo"
      aria-hidden="true"
      style={{ width: height * WORDMARK_ASPECT, height }}
    >
      <img className="gl-brand-logo-image is-light" src="/golive-logo.svg" alt="" draggable={false} />
      <img className="gl-brand-logo-image is-dark" src="/golive-logo-white.svg" alt="" draggable={false} />
    </span>
  );
}
