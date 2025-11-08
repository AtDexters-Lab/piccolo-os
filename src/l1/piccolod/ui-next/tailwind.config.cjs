const config = {
  content: ['./src/**/*.{html,js,svelte,ts}'],
  theme: {
    extend: {
      colors: {
        surface: 'rgb(var(--color-surface) / <alpha-value>)',
        'surface-strong': 'rgb(var(--color-surface-strong) / <alpha-value>)',
        text: 'rgb(var(--color-text) / <alpha-value>)',
        muted: 'rgb(var(--color-text-muted) / <alpha-value>)',
        accent: 'rgb(var(--color-accent) / <alpha-value>)',
        warning: 'rgb(var(--color-warning) / <alpha-value>)',
        success: 'rgb(var(--color-success) / <alpha-value>)',
        info: 'rgb(var(--color-info) / <alpha-value>)',
        critical: 'rgb(var(--color-critical) / <alpha-value>)'
      },
      borderRadius: {
        '2xl': '1.5rem'
      },
      fontFamily: {
        sans: ['"Inter"', '"SF Pro Text"', 'system-ui', 'sans-serif']
      }
    }
  },
  plugins: []
};

module.exports = config;
