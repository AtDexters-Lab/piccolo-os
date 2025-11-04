import { writable } from 'svelte/store';
import { apiProd, type ErrorResponse } from '@api/client';

export type ActivityItem = {
  id?: string;
  title: string;
  detail?: string;
  ts?: string;
  status?: 'success' | 'warning' | 'error' | 'running';
};

type ActivityState = {
  items: ActivityItem[];
  loading: boolean;
  error: string | null;
  unsupported: boolean;
};

const initialState: ActivityState = {
  items: [],
  loading: false,
  error: null,
  unsupported: false
};

const activityStore = writable<ActivityState>(initialState);

export async function refreshActivity() {
  activityStore.update((state) => ({ ...state, loading: true, error: null, unsupported: false }));
  try {
    const response = await apiProd<{ items?: ActivityItem[] }>('/activity');
    activityStore.set({
      items: response?.items ?? [],
      loading: false,
      error: null,
      unsupported: false
    });
  } catch (err: any) {
    const error = err as ErrorResponse | undefined;
    if (error?.code === 404) {
      activityStore.set({
        items: [],
        loading: false,
        error: null,
        unsupported: true
      });
    } else {
      activityStore.update((state) => ({
        ...state,
        loading: false,
        error: error?.message || 'Failed to load activity.',
        unsupported: false
      }));
    }
  }
}

export { activityStore };
