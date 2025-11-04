const withOpacityValue = (variable, defaultValue = '1') => {
  return ({ opacityValue } = {}) => {
    if (opacityValue !== undefined) {
      return `rgb(var(${variable}) / ${opacityValue})`;
    }
    return `rgb(var(${variable}) / ${defaultValue})`;
  };
};

/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./index.html', './src/**/*.{svelte,ts,js}'],
  theme: {
    extend: {
      fontFamily: {
        sans: [
          'Inter',
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          '"Segoe UI"',
          '"Noto Sans"',
          '"Helvetica Neue"',
          'Arial',
          'sans-serif'
        ],
        mono: ['"JetBrains Mono"', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace']
      },
      colors: {
        accent: {
          DEFAULT: withOpacityValue('--accent-rgb'),
          emphasis: withOpacityValue('--accent-emphasis-rgb'),
          subtle: withOpacityValue('--accent-rgb', 'var(--accent-subtle-opacity)')
        },
        surface: {
          0: withOpacityValue('--surface-0-rgb'),
          1: withOpacityValue('--surface-1-rgb'),
          2: withOpacityValue('--surface-2-rgb'),
          strong: withOpacityValue('--surface-strong-rgb')
        },
        text: {
          primary: withOpacityValue('--text-strong-rgb'),
          muted: withOpacityValue('--text-muted-rgb'),
          inverse: withOpacityValue('--text-inverse-rgb')
        },
        state: {
          ok: withOpacityValue('--state-ok-rgb'),
          notice: withOpacityValue('--state-notice-rgb'),
          warn: withOpacityValue('--state-warn-rgb'),
          degraded: withOpacityValue('--state-degraded-rgb'),
          critical: withOpacityValue('--state-critical-rgb')
        },
        border: {
          DEFAULT: withOpacityValue('--border-rgb', 'var(--border-opacity)'),
          subtle: withOpacityValue('--border-rgb', 'var(--border-subtle-opacity)')
        },
        focus: withOpacityValue('--focus-ring-rgb')
      },
      spacing: {
        1.5: '0.375rem',
        4.5: '1.125rem'
      },
      borderRadius: {
        lg: '12px',
        xl: '16px',
        '2xl': '24px'
      },
      boxShadow: {
        focus: '0 0 0 3px var(--focus-ring-shadow)'
      },
      transitionTimingFunction: {
        standard: 'cubic-bezier(0.2, 0, 0, 1)'
      },
      transitionDuration: {
        default: '180ms'
      }
    }
  },
  plugins: [],
};
