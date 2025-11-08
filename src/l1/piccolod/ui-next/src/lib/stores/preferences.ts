import { writable } from 'svelte/store';

export type ThemeMode = 'light' | 'dark';
export type Preferences = {
  theme: ThemeMode;
  background: 'aurora' | 'midnight' | 'plain';
  accent: string;
};

const defaultPreferences: Preferences = {
  theme: 'light',
  background: 'aurora',
  accent: '#6266ff'
};

export const preferencesStore = writable<Preferences>(defaultPreferences);
