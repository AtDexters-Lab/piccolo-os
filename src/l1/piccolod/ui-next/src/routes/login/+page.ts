import type { PageLoad } from './$types';
import { platformController, getPlatformState } from '$lib/stores/platform';

export const ssr = false;

export const load: PageLoad = async ({ url }) => {
  await platformController.refreshSession();
  const session = getPlatformState().session;
  if (session?.authenticated) {
    const raw = url.searchParams.get('redirect');
    const decoded = raw ? safeDecode(raw) : null;
    const valid = decoded && decoded.startsWith('/') && !decoded.startsWith('//');
    return {
      redirectTo: valid ? decoded : '/'
    };
  }
  return {};
};

function safeDecode(value: string): string | null {
  try {
    return decodeURIComponent(value);
  } catch {
    return null;
  }
}

