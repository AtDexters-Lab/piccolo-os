const config = {
  content: ['./src/**/*.{html,js,svelte,ts}'],
  theme: {
    extend: {
      colors: {
        surface: 'rgb(var(--sys-surface) / <alpha-value>)',
        'surface-variant': 'rgb(var(--sys-surface-variant) / <alpha-value>)',
        'surface-muted': 'rgb(var(--sys-surface-muted) / <alpha-value>)',
        ink: 'rgb(var(--sys-ink) / <alpha-value>)',
        muted: 'rgb(var(--sys-ink-muted) / <alpha-value>)',
        accent: 'rgb(var(--sys-accent-rgb) / <alpha-value>)',
        'accent-hero': 'rgb(var(--sys-accent-hero-rgb) / <alpha-value>)',
        success: 'rgb(var(--sys-success) / <alpha-value>)',
        info: 'rgb(var(--sys-info) / <alpha-value>)',
        warning: 'rgb(var(--sys-warning) / <alpha-value>)',
        critical: 'rgb(var(--sys-critical) / <alpha-value>)'
      },
      borderRadius: {
        xs: 'var(--radius-xs)',
        sm: 'var(--radius-sm)',
        md: 'var(--radius-md)',
        lg: 'var(--radius-lg)',
        xl: 'var(--radius-xl)',
        pill: 'var(--radius-pill)'
      },
      boxShadow: {
        soft: 'var(--shadow-soft)',
        strong: 'var(--shadow-strong)'
      },
      fontFamily: {
        sans: ['Inter', 'SF Pro Text', 'system-ui', 'sans-serif'],
        display: ['Comfortaa', 'Inter', 'system-ui', 'sans-serif'],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Consolas', 'Liberation Mono', 'monospace']
      }
    }
  },
  plugins: []
};

module.exports = config;
