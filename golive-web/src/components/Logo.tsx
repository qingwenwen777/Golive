interface LogoProps {
  height?: number;
}

export function GoLiveLogo({ height = 20 }: LogoProps) {
  return (
    <svg viewBox="0 0 28 20" style={{ height, display: 'block' }} aria-hidden="true">
      <path
        d="M27.9 3.1a3.5 3.5 0 0 0-2.5-2.5C23.2 0 14 0 14 0S4.8 0 2.6.6A3.5 3.5 0 0 0 .1 3.1 36 36 0 0 0 0 10a36 36 0 0 0 .6 6.9 3.5 3.5 0 0 0 2.5 2.5C4.8 20 14 20 14 20s9.2 0 11.4-.6a3.5 3.5 0 0 0 2.5-2.5A36 36 0 0 0 28 10a36 36 0 0 0-.1-6.9Z"
        fill="#FF0033"
      />
      <polygon points="11.2,14.3 18.9,10 11.2,5.7" fill="#fff" />
    </svg>
  );
}
