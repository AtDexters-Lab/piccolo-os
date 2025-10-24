import { expect, type APIRequestContext, type Page } from '@playwright/test';

export const ADMIN_PASSWORD = process.env.PICCOLO_E2E_ADMIN_PASSWORD || 'password';

export async function seedAdmin(request: APIRequestContext, password: string = ADMIN_PASSWORD): Promise<void> {
  await request.post('/api/v1/crypto/setup', { data: { password }, failOnStatusCode: false }).catch(() => {});
  const setup = await request.post('/api/v1/auth/setup', { data: { password }, failOnStatusCode: false }).catch(() => undefined);
  if (setup && setup.status() === 423) {
    await request.post('/api/v1/crypto/unlock', { data: { password }, failOnStatusCode: false }).catch(() => {});
    await request.post('/api/v1/auth/setup', { data: { password }, failOnStatusCode: false }).catch(() => {});
  }
}

export async function login(page: Page, password: string = ADMIN_PASSWORD): Promise<void> {
  // Try API login first for stability; fall back to UI if session still unauthenticated.
  const apiLogin = await page.request.post('/api/v1/auth/login', {
    data: { username: 'admin', password }
  });
  if (apiLogin.ok()) {
    await page.goto('/#/');
    await expect(page.locator('h2', { hasText: 'Dashboard' })).toBeVisible({ timeout: 15000 });
    return;
  }

  await page.goto('/#/login');

  const unlockField = page.getByPlaceholder('admin password');
  if (await unlockField.isVisible({ timeout: 2000 }).catch(() => false)) {
    await unlockField.fill(password);
    await page.getByRole('button', { name: /unlock/i }).click();
    await expect(unlockField).toBeHidden({ timeout: 15000 });
  }

  const usernameInput = page.getByPlaceholder('admin');
  const onLogin = await usernameInput.isVisible({ timeout: 5000 }).catch(() => false);
  if (onLogin) {
    await usernameInput.fill('admin');
    await page.getByPlaceholder('••••••••').fill(password);
    await page.getByRole('button', { name: 'Sign in' }).click({ timeout: 15000 });
    await expect(page).toHaveURL(/#\/?$/);
  }
  await expect(page.locator('h2', { hasText: 'Dashboard' })).toBeVisible({ timeout: 15000 });
}

export async function ensureSignedIn(page: Page, password: string = ADMIN_PASSWORD): Promise<void> {
  await seedAdmin(page.request, password);
  await login(page, password);
}
