import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
  try {
    const res = await fetch('/api/v1/auth/initialized');
    if (!res.ok) {
      return { initialized: false };
    }
    const data = await res.json();
    return { initialized: Boolean(data?.initialized) };
  } catch (err) {
    console.error('Failed to check initialization', err);
    return { initialized: false };
  }
};
