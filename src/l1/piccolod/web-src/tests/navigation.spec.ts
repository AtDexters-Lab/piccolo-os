import { test, expect } from '@playwright/test';

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
    await request.post('/api/v1/auth/setup', { data: { password: 'password' } }).catch(() => {});
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    const heading = page.locator('h2');
    const text = (await heading.textContent()) || '';
    if (/Sign in/i.test(text)) {
      await page.getByLabel('Username').fill('admin');
      await page.getByLabel('Password').fill('password');
      await page.getByRole('button', { name: 'Sign in' }).click();
      await expect(page.locator('h2')).toHaveText('Dashboard');
    } else {
      await expect(heading).toHaveText('Dashboard');
    }
  });

  test('home loads without redirects and assets are reachable', async ({ page, request }) => {
    const response = await page.goto('/');
    expect(response?.status()).toBe(200);
    await expect(page.locator('header img[alt="Piccolo"]')).toBeVisible();

    const scriptHref = await page.locator('script[type="module"][src^="/assets/"]').first().getAttribute('src');
    expect(scriptHref).toBeTruthy();
    const js = await request.get(scriptHref!);
    expect(js.ok()).toBeTruthy();

    const logo = await request.get('/branding/piccolo.svg');
    expect(logo.ok()).toBeTruthy();
  });

  test('navigate via sidebar to core pages', async ({ page }) => {
    await page.locator('aside').getByRole('link', { name: 'Apps' }).click();
    await expect(page.locator('h2')).toHaveText('Apps');

    await page.locator('aside').getByRole('link', { name: 'Storage' }).click();
    await expect(page.locator('h2')).toHaveText('Storage');

    await page.locator('aside').getByRole('link', { name: 'Updates' }).click();
    await expect(page.locator('h2')).toHaveText('Updates');

    await page.locator('aside').getByRole('link', { name: 'Remote' }).click();
    await expect(page.locator('h2')).toHaveText('Remote');
  });

  test('deep-link directly to /#/apps', async ({ page }) => {
    const resp = await page.goto('/#/apps');
    expect(resp?.status()).toBe(200);
    await expect(page.locator('h2')).toHaveText('Apps');
  });
});
