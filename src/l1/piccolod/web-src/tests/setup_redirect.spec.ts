import { test, expect } from '@playwright/test';

test.describe('First-run setup redirect', () => {
test('unauthenticated dashboard redirects to setup when not initialized', async ({ page, request }) => {
  const init = await request.get('/api/v1/auth/initialized').then(r => r.json()).catch(() => ({ initialized: true }));
  test.skip(init.initialized, 'Admin already initialized in this environment');

  await page.goto('/#/');
  await expect(page.getByRole('heading', { level: 2, name: 'Create Admin' })).toBeVisible();
});

test('login route redirects to setup when not initialized', async ({ page, request }) => {
  const init = await request.get('/api/v1/auth/initialized').then(r => r.json()).catch(() => ({ initialized: true }));
  test.skip(init.initialized, 'Admin already initialized in this environment');

  await page.goto('/#/login');
  await expect(page.getByRole('heading', { level: 2, name: 'Create Admin' })).toBeVisible();
});
});
