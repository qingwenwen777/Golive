import type { CSSProperties } from 'react';

interface LogoProps {
  height?: number;
  variant?: 'mark' | 'wordmark';
}

const RABBIT_ICON_SRC = '/golive-rabbit-icon.png';
const WORDMARK_ASPECT = 220 / 60;

export function GoLiveLogo({ height = 24, variant = 'wordmark' }: LogoProps) {
  if (variant === 'mark') {
    return (
      <img
        src={RABBIT_ICON_SRC}
        alt=""
        aria-hidden="true"
        draggable={false}
        style={{ width: height, height, display: 'block', objectFit: 'contain' }}
      />
    );
  }

  const wordmarkStyle = {
    width: height * WORDMARK_ASPECT,
    height,
    '--gl-logo-font-size': `${height * 0.6}px`,
    '--gl-logo-gap': `${Math.max(4, height * 0.14)}px`,
  } as CSSProperties;

  return (
    <span
      className="gl-brand-logo"
      aria-hidden="true"
      style={wordmarkStyle}
    >
      <img className="gl-brand-logo-mark" src={RABBIT_ICON_SRC} alt="" draggable={false} />
      <span className="gl-brand-logo-word" aria-hidden="true">
        <span className="gl-brand-logo-go">Go</span>
        <span className="gl-brand-logo-live">Live</span>
      </span>
    </span>
  );
}
