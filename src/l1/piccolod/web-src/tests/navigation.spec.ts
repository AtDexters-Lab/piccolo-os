import { test, expect } from '@playwright/test';

// Fail tests on any browser console error
test.beforeEach(async ({ page }) => {
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      throw new Error(`Console error: ${msg.text()}`);
    }
  });
});

test.describe('Top-level navigation and deep links', () => {
  test('home loads without redirects and assets are reachable', async ({ page, request }) => {
    const resp = await page.goto('/');
    expect(resp?.status()).toBe(200);
    await expect(page.locator('header h1')).toHaveText('Piccolo OS');

    // Ensure main asset JS is reachable
    const scriptHref = await page.locator('script[type="module"][src^="/assets/"]').first().getAttribute('src');
    expect(scriptHref).toBeTruthy();
    const js = await request.get(scriptHref!);
    expect(js.ok()).toBeTruthy();
  });

  test('navigate via nav: Apps/Storage/Updates/Remote', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('link', { name: 'Apps' }).click();
    await expect(page.locator('h2')).toHaveText('Apps');

    await page.getByRole('link', { name: 'Storage' }).click();
    await expect(page.locator('h2')).toHaveText('Storage');

    await page.getByRole('link', { name: 'Updates' }).click();
    await expect(page.locator('h2')).toHaveText('Updates');

    await page.getByRole('link', { name: 'Remote' }).click();
    await expect(page.locator('h2')).toHaveText('Remote');
  });

  test('deep-link directly to /#/apps', async ({ page }) => {
    const resp = await page.goto('/#/apps');
    expect(resp?.status()).toBe(200);
    await expect(page.locator('h2')).toHaveText('Apps');
  });
});

test('apps actions show toasts (demo)', async ({ page }) => {
  await page.goto('/#/apps');
  const startBtn = page.getByRole('button', { name: 'Start' }).first();
  await startBtn.click();
  await expect(page.getByText('Started', { exact: false }).last()).toBeVisible();
});
