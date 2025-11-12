import type { LayoutLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { platformController } from '$lib/stores/platform';

export const ssr = false;

const matchesPath = (pathname: string, target: string) =>
  pathname === target || pathname.startsWith(`${target}/`);

export const load: LayoutLoad = async ({ url }) => {
  const crypto = await platformController.refreshCrypto();

  if (!crypto.initialized && !matchesPath(url.pathname, '/setup')) {
    throw redirect(307, `/setup?redirect=${encodeURIComponent(url.pathname + url.search)}`);
  }

  if (crypto.initialized && crypto.locked && !matchesPath(url.pathname, '/unlock')) {
    throw redirect(307, `/unlock?redirect=${encodeURIComponent(url.pathname + url.search)}`);
  }

  if (!crypto.locked) {
    void platformController.refreshSession();
  }

  return { crypto };
};
