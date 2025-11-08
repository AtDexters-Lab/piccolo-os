import { http } from './http';

export type InitStatus = {
  initialized: boolean;
};

export async function fetchInitializationStatus(): Promise<InitStatus> {
  return http<InitStatus>('/auth/initialized');
}

export async function createAdmin(password: string): Promise<void> {
  await http('/auth/setup', { method: 'POST', json: { password } });
}

export async function initCrypto(password: string): Promise<void> {
  await http('/crypto/setup', { method: 'POST', json: { password } });
}
