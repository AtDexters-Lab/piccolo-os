import { test, expect } from '@playwright/test';

test.describe('UI unlock flow (real API)', () => {
  const adminPass = 'password';

  test('unlock volumes from Storage page', async ({ page }) => {
    // Setup admin and login
    await page.request.post('/api/v1/auth/setup', { data: { password: adminPass } }).catch(() => {});
    await page.goto('/#/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill(adminPass);
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.locator('h2')).toHaveText('Dashboard');

    // Ensure crypto initialized and locked
    const csrf = await page.request.get('/api/v1/auth/csrf').then(r => r.json()).then(j => j.token as string);
    const st = await page.request.get('/api/v1/crypto/status').then(r => r.json());
    if (!st.initialized) {
      await page.request.post('/api/v1/crypto/setup', { headers: { 'X-CSRF-Token': csrf }, data: { password: adminPass } });
    }
    const st2 = await page.request.get('/api/v1/crypto/status').then(r => r.json());
    if (!st2.locked) {
      await page.request.post('/api/v1/crypto/lock', { headers: { 'X-CSRF-Token': csrf } });
    }

    // Go to Storage and perform unlock
    await page.goto('/#/storage');
    await page.getByLabel('Password').fill(adminPass);
    await page.getByRole('button', { name: 'Unlock volumes' }).click();
    await expect(page.getByText('Volumes unlocked')).toBeVisible();
    // Badge should flip to Unlocked
    await expect(page.getByText('Unlocked')).toBeVisible();
  });
});

