import { writable } from 'svelte/store';
import { api } from '@api/client';

type Session = { authenticated: boolean; user?: string; volumes_locked?: boolean };

export const sessionStore = writable<Session>({ authenticated: false });

export async function bootstrapSession() {
  try {
    const data = await api<Session>('/auth/session');
    sessionStore.set({ authenticated: !!data.authenticated, user: data.user, volumes_locked: data.volumes_locked });
  } catch {
    sessionStore.set({ authenticated: false });
  }
}

