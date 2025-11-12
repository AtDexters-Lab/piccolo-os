import { http, type ApiError } from './http';

export type InitStatus = {
  initialized: boolean;
  locked?: boolean;
};

export type SessionInfo = {
  authenticated: boolean;
  user?: string;
  expiresAt?: string;
  volumesLocked?: boolean;
};

export type CryptoStatus = {
  initialized: boolean;
  locked: boolean;
};

type InitStatusDTO = {
  initialized?: boolean;
};

type SessionDTO = {
  authenticated?: boolean;
  user?: string;
  expires_at?: string;
  volumes_locked?: boolean;
};

type CryptoStatusDTO = {
  initialized?: boolean;
  locked?: boolean;
};

export async function fetchInitializationStatus(signal?: AbortSignal): Promise<InitStatus> {
  try {
    const data = await http<InitStatusDTO>('/auth/initialized', { signal, skipCsrf: true });
    return { initialized: Boolean(data?.initialized) };
  } catch (error) {
    const apiError = error as ApiError | undefined;
    if (apiError?.code === 423) {
      return { initialized: false, locked: true };
    }
    throw error;
  }
}

export async function fetchSessionInfo(signal?: AbortSignal): Promise<SessionInfo> {
  const data = await http<SessionDTO>('/auth/session', { signal, skipCsrf: true });
  return {
    authenticated: Boolean(data?.authenticated),
    user: typeof data?.user === 'string' ? data.user : undefined,
    expiresAt: typeof data?.expires_at === 'string' ? data.expires_at : undefined,
    volumesLocked: typeof data?.volumes_locked === 'boolean' ? data.volumes_locked : undefined
  };
}

export async function fetchCryptoStatus(signal?: AbortSignal): Promise<CryptoStatus> {
  const data = await http<CryptoStatusDTO>('/crypto/status', { signal, skipCsrf: true });
  return {
    initialized: Boolean(data?.initialized),
    locked: Boolean(data?.locked)
  };
}

export async function createAdmin(password: string, signal?: AbortSignal): Promise<void> {
  await http('/auth/setup', { method: 'POST', json: { password }, signal });
}

export async function initCrypto(password: string, signal?: AbortSignal): Promise<void> {
  await http('/crypto/setup', { method: 'POST', json: { password }, signal });
}

export async function unlockCrypto(params: { password?: string; recoveryKey?: string }, signal?: AbortSignal): Promise<void> {
  const payload: { password?: string | null; recovery_key?: string | null } = {};
  if (typeof params.password === 'string') payload.password = params.password;
  if (typeof params.recoveryKey === 'string') payload.recovery_key = params.recoveryKey;
  await http('/crypto/unlock', { method: 'POST', json: payload, signal });
}
