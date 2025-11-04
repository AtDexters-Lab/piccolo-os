import { test, expect } from '@playwright/test';
import { ADMIN_PASSWORD, ensureSignedIn, seedAdmin } from './support/session';

test.beforeEach(async ({ page }) => {
  await page.route('**/favicon.ico', async (route) => {
    await route.fulfill({ status: 200, body: '' });
  });
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      const text = msg.text();
      if (/Failed to load resource: .* (404|401|403)/.test(text)) return;
      throw new Error(`Console error: ${text}`);
    }
  });
});

test.describe('Navigation basics', () => {
  test.beforeAll(async ({ request }) => {
    await seedAdmin(request, ADMIN_PASSWORD);
  });

  test.beforeEach(async ({ page }) => {
  await ensureSignedIn(page, ADMIN_PASSWORD);
  await expect(page.getByRole('heading', { level: 2, name: 'What matters now' })).toBeVisible();
  });

  test('home loads without redirects and assets are reachable', async ({ page, request }) => {
    const response = await page.goto('/');
    expect(response?.status()).toBe(200);
    await expect(page.getByAltText('Piccolo logo')).toBeVisible();

    const scriptHref = await page.locator('script[type="module"][src^="/assets/"]').first().getAttribute('src');
    expect(scriptHref).toBeTruthy();
    const js = await request.get(scriptHref!);
    expect(js.ok()).toBeTruthy();

    const logo = await request.get('/branding/piccolo.svg');
    expect(logo.ok()).toBeTruthy();
  });

  test('navigate via sidebar to core pages', async ({ page }) => {
    const sidebar = page.getByRole('complementary', { name: 'Primary' });
    await sidebar.getByRole('button', { name: 'Apps' }).click();
    await expect(page.getByRole('heading', { level: 2, name: 'Apps' })).toBeVisible();

    await sidebar.getByRole('button', { name: 'Devices' }).click();
    await expect(page.getByRole('heading', { level: 2, name: 'Devices' })).toBeVisible();

    await sidebar.getByRole('button', { name: 'Settings' }).click();
    await expect(page.getByRole('heading', { level: 2, name: 'Settings' })).toBeVisible();

    await sidebar.getByRole('button', { name: 'Home' }).click();
    await expect(page.getByRole('heading', { level: 2, name: 'What matters now' })).toBeVisible();
  });

  test('deep-link directly to /#/apps', async ({ page }) => {
    await page.goto('/#/apps');
    await page.waitForLoadState('networkidle');
    await expect(page.getByRole('heading', { level: 2, name: 'Apps' })).toBeVisible();
  });
});
