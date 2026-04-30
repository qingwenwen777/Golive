import type { Config } from 'tailwindcss';
import animate from 'tailwindcss-animate';

const config: Config = {
  darkMode: 'class',
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        // Project (legacy) tokens — keep for existing utilities like
        // bg-bg-2, bg-bg-hover, text-secondary, etc.
        bg: {
          DEFAULT: 'var(--gl-bg)',
          primary: 'var(--gl-bg)',
          2: 'var(--gl-bg-2)',
          hover: 'var(--gl-bg-hover)',
          card: 'var(--gl-bg-card)',
        },
        text: {
          DEFAULT: 'var(--gl-text)',
          primary: 'var(--gl-text)',
          secondary: 'var(--gl-text-2)',
          tertiary: 'var(--gl-text-3)',
        },
        chip: {
          DEFAULT: 'var(--gl-chip-bg)',
          active: 'var(--gl-chip-bg-active)',
          'active-fg': 'var(--gl-chip-text-active)',
        },
        red: 'var(--gl-red)',
        blue: '#1c62b9',

        // shadcn / radix-style semantic tokens. Defined in CSS as HSL triples
        // so opacity modifiers like `bg-popover/80` keep working.
        border: 'hsl(var(--border))',
        input: 'hsl(var(--input))',
        ring: 'hsl(var(--ring))',
        background: 'hsl(var(--background))',
        foreground: 'hsl(var(--foreground))',
        primary: {
          DEFAULT: 'hsl(var(--primary))',
          foreground: 'hsl(var(--primary-foreground))',
          // legacy aliases
          fg: 'var(--gl-btn-primary-text)',
          hover: 'var(--gl-btn-primary-hover)',
        },
        secondary: {
          DEFAULT: 'hsl(var(--secondary))',
          foreground: 'hsl(var(--secondary-foreground))',
          fg: 'var(--gl-btn-secondary-text)',
        },
        destructive: {
          DEFAULT: 'hsl(var(--destructive))',
          foreground: 'hsl(var(--destructive-foreground))',
        },
        muted: {
          DEFAULT: 'hsl(var(--muted))',
          foreground: 'hsl(var(--muted-foreground))',
        },
        accent: {
          DEFAULT: 'hsl(var(--accent))',
          foreground: 'hsl(var(--accent-foreground))',
        },
        popover: {
          DEFAULT: 'hsl(var(--popover))',
          foreground: 'hsl(var(--popover-foreground))',
        },
        card: {
          DEFAULT: 'hsl(var(--card))',
          foreground: 'hsl(var(--card-foreground))',
        },
      },
      borderRadius: {
        pill: '9999px',
        card: '12px',
        lg: 'var(--radius)',
        md: 'calc(var(--radius) - 2px)',
        sm: 'calc(var(--radius) - 4px)',
      },
      fontFamily: {
        sans: ['Roboto', 'Arial', '"PingFang SC"', '"Microsoft YaHei"', 'sans-serif'],
      },
      boxShadow: {
        card: 'var(--gl-shadow)',
      },
    },
  },
  plugins: [animate],
};

export default config;
