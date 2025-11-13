import { writable } from 'svelte/store';
import type { ApiError } from '$lib/api/http';
import {
  createAdmin,
  initCrypto,
  unlockCrypto,
  fetchInitializationStatus,
  fetchSessionInfo,
  fetchCryptoStatus,
  type SessionInfo,
  type CryptoStatus
} from '$lib/api/setup';

export type SetupPhase = 'loading' | 'first-run' | 'unlock' | 'ready' | 'submitting' | 'error';

export type SubmissionMode = 'first-run' | 'unlock';

export type SetupState =
  | { phase: 'loading' }
  | { phase: 'first-run'; crypto: CryptoStatus; session?: SessionInfo }
  | { phase: 'unlock'; crypto: CryptoStatus; session?: SessionInfo }
  | { phase: 'submitting'; flow: SubmissionMode; step: 'crypto-setup' | 'crypto-unlock' | 'auth-setup' }
  | { phase: 'ready'; locked: boolean; session?: SessionInfo }
  | { phase: 'error'; message: string; code?: number; retryAfter?: number };

const initialState: SetupState = { phase: 'loading' };

function toErrorState(error: unknown): SetupState {
  const fallback = 'Setup failed. Please retry.';
  if (typeof error === 'string') return { phase: 'error', message: error };
  const apiError = error as ApiError | undefined;
  return {
    phase: 'error',
    message: apiError?.message ?? fallback,
    code: apiError?.code,
    retryAfter: apiError?.retryAfter
  };
}

export function createSetupController() {
  const store = writable<SetupState>(initialState);

  async function refresh(signal?: AbortSignal) {
    store.set({ phase: 'loading' });
    try {
      const [crypto, session] = await Promise.all([
        fetchCryptoStatus(signal),
        fetchSessionInfo(signal).catch(() => undefined)
      ]);

      if (!crypto.initialized) {
        store.set({ phase: 'first-run', crypto, session });
        return;
      }

      if (crypto.locked) {
        store.set({ phase: 'unlock', crypto, session });
        return;
      }

      store.set({ phase: 'ready', locked: false, session });
    } catch (error) {
      store.set(toErrorState(error));
    }
  }

  async function ensureAdmin(password?: string) {
    const status = await fetchInitializationStatus();
    if (status.initialized) return;
    if (!password) {
      throw new Error('Admin password required to finish setup.');
    }
    await createAdmin(password);
  }

  async function submitCredentials({ password, mode }: { password?: string; mode: SubmissionMode }, signal?: AbortSignal) {
    try {
      const flow = mode;
      if (mode === 'first-run') {
        if (!password) {
          throw new Error('Password is required to initialize Piccolo.');
        }
        store.set({ phase: 'submitting', flow, step: 'crypto-setup' });
        await initCrypto(password, signal);
      }

      store.set({ phase: 'submitting', flow, step: 'crypto-unlock' });
      if (!password) {
        throw new Error('Enter the admin password to unlock Piccolo.');
      }
      await unlockCrypto(password, signal);

      store.set({ phase: 'submitting', flow, step: 'auth-setup' });
      await ensureAdmin(password);

      await refresh(signal);
    } catch (error) {
      store.set(toErrorState(error));
    }
  }

  function retry() {
    void refresh();
  }

  return {
    subscribe: store.subscribe,
    refresh,
    submitCredentials,
    retry
  };
}
