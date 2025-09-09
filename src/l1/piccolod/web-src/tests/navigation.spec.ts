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

    // Additional BFS routes validated separately
  });

  test('deep-link directly to /#/apps', async ({ page }) => {
    const resp = await page.goto('/#/apps');
    expect(resp?.status()).toBe(200);
    await expect(page.locator('h2')).toHaveText('Apps');
  });
});

test('apps actions show toasts (demo)', async ({ page }) => {
  if (test.info().project.name === 'mobile-chromium') test.skip();
  await page.goto('/#/apps');
  const startBtn = page.getByRole('button', { name: 'Start' }).first();
  await startBtn.click();
  await expect(page.getByText('Started', { exact: false }).last()).toBeVisible();

  // Navigate to details and perform update (toast check)
  await page.getByRole('link', { name: /vaultwarden/i }).click();
  await expect(page.locator('h2')).toHaveText(/App: vaultwarden/);
  await page.getByRole('button', { name: 'Update' }).click();
  await expect(page.getByText('Updated', { exact: false }).last()).toBeVisible();
});

test('updates OS apply shows toast (demo)', async ({ page }) => {
  await page.goto('/#/updates');
  const applyBtn = page.getByRole('button', { name: 'Apply' });
  await applyBtn.click();
  await expect(page.getByText('OS update applied')).toBeVisible();
});

test('remote simulate DNS error shows error toast (demo)', async ({ page }) => {
  await page.goto('/#/remote');
  const btn = page.getByRole('button', { name: 'Simulate DNS error' });
  await btn.click();
  await expect(page.getByText(/Configure failed|DNS/i)).toBeVisible();
});

test('storage action shows toast (demo)', async ({ page }) => {
  await page.goto('/#/storage');
  // Prefer non-destructive action: Set default if available, else Use as-is
  const setDefault = page.getByRole('button', { name: 'Set default' }).first();
  if (await setDefault.count() > 0) {
    await setDefault.click();
    await expect(page.getByText('Default data root updated')).toBeVisible();
  } else {
    const useBtn = page.getByRole('button', { name: 'Use as-is' }).first();
    await useBtn.click();
    await expect(page.getByText('Using', { exact: false })).toBeVisible();
  }
});
