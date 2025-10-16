import { expect, type APIRequestContext, type Page } from '@playwright/test';

export const ADMIN_PASSWORD = process.env.PICCOLO_E2E_ADMIN_PASSWORD || 'password';

export async function seedAdmin(request: APIRequestContext, password: string = ADMIN_PASSWORD): Promise<void> {
  await request.post('/api/v1/auth/setup', { data: { password } }).catch(() => {});
  await request.post('/api/v1/crypto/setup', { data: { password } }).catch(() => {});
  await request.post('/api/v1/crypto/unlock', { data: { password } }).catch(() => {});
}

export async function login(page: Page, password: string = ADMIN_PASSWORD): Promise<void> {
  await page.goto('/#/login');

  const unlockField = page.getByPlaceholder('admin password');
  if (await unlockField.isVisible({ timeout: 2000 }).catch(() => false)) {
    await unlockField.fill(password);
    await page.getByRole('button', { name: /unlock/i }).click();
    await expect(unlockField).toBeHidden({ timeout: 15000 });
  }

  const usernameInput = page.getByPlaceholder('admin');
  await expect(usernameInput).toBeVisible({ timeout: 15000 });
  await usernameInput.fill('admin');
  await page.getByPlaceholder('••••••••').fill(password);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page).toHaveURL(/#\/?$/);
  await expect(page.locator('h2', { hasText: 'Dashboard' })).toBeVisible();
}

export async function ensureSignedIn(page: Page, password: string = ADMIN_PASSWORD): Promise<void> {
  await seedAdmin(page.request, password);
  await login(page, password);
}
