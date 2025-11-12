import { readable, writable } from 'svelte/store';
import type { CryptoStatus, SessionInfo } from '$lib/api/setup';
import { fetchCryptoStatus, fetchSessionInfo } from '$lib/api/setup';

type PlatformState = {
  crypto: CryptoStatus | null;
  session: SessionInfo | null;
  initialized: boolean;
  locked: boolean;
};

const initialState: PlatformState = {
  crypto: null,
  session: null,
  initialized: false,
  locked: true
};

const platformStore = writable<PlatformState>(initialState);

async function refreshCrypto(signal?: AbortSignal) {
  const crypto = await fetchCryptoStatus(signal);
  platformStore.update((current) => ({
    ...current,
    crypto,
    initialized: crypto.initialized,
    locked: crypto.locked
  }));
  return crypto;
}

async function refreshSession(signal?: AbortSignal) {
  const session = await fetchSessionInfo(signal).catch(() => undefined);
  platformStore.update((current) => ({ ...current, session: session ?? null }));
  return session;
}

export const platformState = readable(initialState, (set) => platformStore.subscribe(set));

export function getPlatformState(): PlatformState {
  let snapshot = initialState;
  platformStore.subscribe((value) => {
    snapshot = value;
  })();
  return snapshot;
}

export const platformController = {
  refreshCrypto,
  refreshSession
};
