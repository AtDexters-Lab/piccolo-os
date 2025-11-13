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
  passwordStale?: boolean;
  recoveryStale?: boolean;
};

export type CryptoStatus = {
  initialized: boolean;
  locked: boolean;
};

export type RecoveryKeyStatus = {
  present: boolean;
  stale?: boolean;
};

type InitStatusDTO = {
  initialized?: boolean;
};

type SessionDTO = {
  authenticated?: boolean;
  user?: string;
  expires_at?: string;
  volumes_locked?: boolean;
  password_stale?: boolean;
  recovery_stale?: boolean;
};

type CryptoStatusDTO = {
  initialized?: boolean;
  locked?: boolean;
};

type RecoveryKeyStatusDTO = {
  present?: boolean;
  stale?: boolean;
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
    volumesLocked: typeof data?.volumes_locked === 'boolean' ? data.volumes_locked : undefined,
    passwordStale: typeof data?.password_stale === 'boolean' ? data.password_stale : undefined,
    recoveryStale: typeof data?.recovery_stale === 'boolean' ? data.recovery_stale : undefined
  };
}

export async function fetchCryptoStatus(signal?: AbortSignal): Promise<CryptoStatus> {
  const data = await http<CryptoStatusDTO>('/crypto/status', { signal, skipCsrf: true });
  return {
    initialized: Boolean(data?.initialized),
    locked: Boolean(data?.locked)
  };
}

export async function fetchRecoveryKeyStatus(signal?: AbortSignal): Promise<RecoveryKeyStatus> {
  const data = await http<RecoveryKeyStatusDTO>('/crypto/recovery-key', { signal });
  return {
    present: Boolean(data?.present),
    stale: typeof data?.stale === 'boolean' ? data.stale : undefined
  };
}

export async function createAdmin(password: string, signal?: AbortSignal): Promise<void> {
  await http('/auth/setup', { method: 'POST', json: { password }, signal });
}

export async function initCrypto(password: string, signal?: AbortSignal): Promise<void> {
  await http('/crypto/setup', { method: 'POST', json: { password }, signal });
}

export async function unlockCrypto(password: string, signal?: AbortSignal): Promise<void> {
  await http('/crypto/unlock', { method: 'POST', json: { password }, signal });
}

export async function resetPasswordWithRecovery(params: { recoveryKey: string; newPassword: string }, signal?: AbortSignal): Promise<void> {
  await http('/crypto/reset-password', {
    method: 'POST',
    json: { recovery_key: params.recoveryKey, new_password: params.newPassword },
    signal
  });
}

export async function acknowledgeStaleness(params: { password?: boolean; recovery?: boolean }, signal?: AbortSignal): Promise<void> {
  const payload: { password?: boolean; recovery?: boolean } = {};
  if (params.password) payload.password = true;
  if (params.recovery) payload.recovery = true;
  if (!payload.password && !payload.recovery) return;
  await http('/auth/staleness/ack', { method: 'POST', json: payload, signal });
}

export async function generateRecoveryKey(signal?: AbortSignal): Promise<string[]> {
  const data = await http<{ words?: string[] }>('/crypto/recovery-key/generate', {
    method: 'POST',
    signal
  });
  return Array.isArray(data?.words) ? data.words.map((word) => String(word)) : [];
}

export async function login(password: string, signal?: AbortSignal): Promise<void> {
  await http('/auth/login', { method: 'POST', json: { username: 'admin', password }, signal });
}

export async function logout(signal?: AbortSignal): Promise<void> {
  await http('/auth/logout', { method: 'POST', signal });
}
