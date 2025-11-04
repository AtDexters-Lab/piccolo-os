import { writable } from 'svelte/store';

export type ThemeMode = 'system' | 'light' | 'dark' | 'high-contrast';

export type BackgroundOption = {
  id: string;
  name: string;
  description: string;
  tone: 'light' | 'dark' | 'color';
  preview: string;
  value: string;
};

export type Preferences = {
  theme: ThemeMode;
  background: string;
};

const STORAGE_KEY = 'piccolo_preferences';

const backgroundOptions: BackgroundOption[] = [
  {
    id: 'aurora',
    name: 'Aurora',
    description: 'Calm violet gradients with soft glow.',
    tone: 'color',
    preview: 'linear-gradient(135deg, rgba(138,92,246,0.75) 0%, rgba(56,189,248,0.65) 100%)',
    value: 'linear-gradient(135deg, rgba(var(--accent-rgb) / 0.6), rgba(var(--state-notice-rgb) / 0.55))'
  },
  {
    id: 'dawn',
    name: 'Dawn',
    description: 'Warm sunrise hues over a subtle blur.',
    tone: 'color',
    preview: 'linear-gradient(160deg, rgba(251,146,60,0.7) 0%, rgba(244,114,182,0.65) 100%)',
    value: 'linear-gradient(160deg, rgba(var(--state-degraded-rgb) / 0.7), rgba(244 114 182 / 0.6))'
  },
  {
    id: 'midnight',
    name: 'Midnight',
    description: 'Deep blues with a faint nebula.',
    tone: 'dark',
    preview: 'radial-gradient(circle at 20% 20%, rgba(2,132,199,0.45), transparent 45%), #0b1120',
    value: 'radial-gradient(circle at 20% 20%, rgba(14 165 233 / 0.35), transparent 50%), #0b1120'
  },
  {
    id: 'slate',
    name: 'Slate',
    description: 'Minimal graphite for focus.',
    tone: 'dark',
    preview: 'linear-gradient(180deg, rgba(30,41,59,1) 0%, rgba(15,23,42,1) 100%)',
    value: 'linear-gradient(180deg, rgba(30 41 59 / 1), rgba(15 23 42 / 1))'
  },
  {
    id: 'paper',
    name: 'Paper',
    description: 'Clean, neutral surface for bright rooms.',
    tone: 'light',
    preview: 'linear-gradient(180deg, rgba(255,255,255,1) 0%, rgba(226,232,240,1) 100%)',
    value: 'linear-gradient(180deg, rgba(255 255 255 / 1), rgba(226 232 240 / 1))'
  },
  {
    id: 'grid',
    name: 'Circuit Grid',
    description: 'Technical grid with subtle accent nodes.',
    tone: 'dark',
    preview: 'radial-gradient(circle at 10% 10%, rgba(139,92,246,0.25), transparent 30%), repeating-linear-gradient(0deg, rgba(255,255,255,0.05), rgba(255,255,255,0.05) 1px, transparent 1px, transparent 32px)',
    value: 'radial-gradient(circle at 10% 10%, rgba(var(--accent-rgb) / 0.25), transparent 30%), repeating-linear-gradient(0deg, rgba(255 255 255 / 0.04), rgba(255 255 255 / 0.04) 1px, transparent 1px, transparent 32px)'
  }
];

const defaultPreferences: Preferences = {
  theme: 'system',
  background: backgroundOptions[0].id
};

function loadPreferences(): Preferences {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return defaultPreferences;
    const parsed = JSON.parse(raw);
    return {
      theme: parsed.theme ?? defaultPreferences.theme,
      background: parsed.background ?? defaultPreferences.background
    };
  } catch {
    return defaultPreferences;
  }
}

function persistPreferences(prefs: Preferences) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs));
  } catch {
    /* no-op */
  }
}

const preferencesStore = writable<Preferences>(defaultPreferences, (set) => {
  if (typeof window !== 'undefined') {
    set(loadPreferences());
  }
  return () => undefined;
});

preferencesStore.subscribe((value) => {
  if (typeof window !== 'undefined') {
    persistPreferences(value);
  }
});

export function setTheme(theme: ThemeMode) {
  preferencesStore.update((prefs) => ({ ...prefs, theme }));
}

export function setBackground(background: string) {
  preferencesStore.update((prefs) => ({ ...prefs, background }));
}

export function applyTheme(theme: ThemeMode) {
  const root = document.documentElement;
  if (!root) return;

  if (theme === 'system') {
    const mediaQuery = typeof window !== 'undefined' ? window.matchMedia?.('(prefers-color-scheme: dark)') : undefined;
    const prefersDark = mediaQuery?.matches ?? false;
    root.dataset.theme = prefersDark ? 'dark' : 'light';
  } else if (theme === 'high-contrast') {
    root.dataset.theme = 'high-contrast';
  } else {
    root.dataset.theme = theme;
  }
}

export function applyBackground(backgroundId: string) {
  const option = backgroundOptions.find((bg) => bg.id === backgroundId) ?? backgroundOptions[0];
  const root = document.documentElement;
  if (root.dataset.theme === 'high-contrast') {
    root.style.setProperty('--app-background', 'linear-gradient(180deg, rgba(255 255 255 / 1), rgba(210 210 210 / 1))');
    root.dataset.backgroundTone = 'light';
    return;
  }
  root.style.setProperty('--app-background', option.value);
  root.dataset.backgroundTone = option.tone;
}

export { preferencesStore, backgroundOptions };
