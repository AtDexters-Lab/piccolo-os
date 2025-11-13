import type { LayoutLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { platformController, getPlatformState } from '$lib/stores/platform';
import { primeCsrfToken } from '$lib/api/http';

export const ssr = false;

const matchesPath = (pathname: string, target: string) =>
  pathname === target || pathname.startsWith(`${target}/`);

const lockedAllowedPaths = ['/unlock', '/password-recovery'];
const preSetupPaths = ['/setup', '/password-recovery', '/unlock', '/login'];
const authOptionalPaths = ['/login', '/password-recovery'];

const isLockedAllowedPath = (pathname: string) => lockedAllowedPaths.some((path) => matchesPath(pathname, path));

export const load: LayoutLoad = async ({ url }) => {
  const crypto = await platformController.refreshCrypto();

  if (!crypto.initialized && !matchesPath(url.pathname, '/setup')) {
    throw redirect(307, `/setup?redirect=${encodeURIComponent(url.pathname + url.search)}`);
  }

  if (crypto.initialized && crypto.locked && !isLockedAllowedPath(url.pathname)) {
    throw redirect(307, `/unlock?redirect=${encodeURIComponent(url.pathname + url.search)}`);
  }

  if (crypto.initialized && !crypto.locked) {
    await platformController.refreshSession();
    const session = getPlatformState().session;
    const requiresAuth = !authOptionalPaths.some((path) => matchesPath(url.pathname, path));
    if (requiresAuth && (!session || !session.authenticated)) {
      throw redirect(307, `/login?redirect=${encodeURIComponent(url.pathname + url.search)}`);
    }
    await primeCsrfToken();
  }

  return { crypto };
};
