import { writable } from 'svelte/store';

export type DeviceSummary = {
  name: string;
  remoteEnabled?: boolean;
  remoteHostname?: string | null;
  remoteState?: string | null;
  remoteWarnings?: string[];
  remoteHydrated?: boolean;
  remoteError?: string | null;
  remoteSupported?: boolean;
};

export const deviceStore = writable<DeviceSummary>({
  name: 'Piccolo Home',
  remoteEnabled: false,
  remoteHostname: null,
  remoteState: 'disabled',
  remoteWarnings: [],
  remoteHydrated: false,
  remoteError: null,
  remoteSupported: true
});

type RemoteSummaryUpdate = {
  enabled?: boolean;
  hostname?: string | null;
  state?: string | null;
  warnings?: string[];
} | null;

export function setRemoteSummary(summary: RemoteSummaryUpdate) {
  const enabled = !!summary?.enabled;
  const hostname = summary?.hostname ?? null;
  const state = summary?.state ?? (enabled ? 'active' : 'disabled');
  const warnings = summary?.warnings ?? [];
  deviceStore.update((prev) => ({
    ...prev,
    remoteEnabled: enabled,
    remoteHostname: hostname,
    remoteState: state,
    remoteWarnings: warnings,
    remoteHydrated: true,
    remoteError: null,
    remoteSupported: true
  }));
}

export function markRemoteHydrated() {
  deviceStore.update((prev) => ({
    ...prev,
    remoteHydrated: true,
    remoteError: null
  }));
}

export function recordRemoteError(message: string | null) {
  deviceStore.update((prev) => ({
    ...prev,
    remoteError: message
  }));
}

export function resetRemoteSummary() {
  deviceStore.update((prev) => ({
    ...prev,
    remoteEnabled: false,
    remoteHostname: null,
    remoteState: 'disabled',
    remoteWarnings: [],
    remoteHydrated: false,
    remoteError: null,
    remoteSupported: true
  }));
}

export function markRemoteUnsupported() {
  deviceStore.update((prev) => ({
    ...prev,
    remoteEnabled: false,
    remoteHostname: null,
    remoteState: 'unsupported',
    remoteWarnings: [],
    remoteHydrated: true,
    remoteError: null,
    remoteSupported: false
  }));
}
