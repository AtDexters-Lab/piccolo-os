import { test, expect } from '@playwright/test';

test.beforeEach(async ({ page }) => {
  await page.route('**/favicon.ico', async (route) => {
    await route.fulfill({ status: 200, body: '' });
  });
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      const text = msg.text();
      if (/Failed to load resource: .* 404/.test(text)) return;
      throw new Error(`Console error: ${text}`);
    }
  });
});

test('catalog lists curated apps and demo install toasts', async ({ page }) => {
  await page.goto('/#/apps/catalog');
  await expect(page.locator('h2')).toHaveText('App Catalog');
  const firstInstall = page.getByRole('button', { name: 'Install' }).first();
  await firstInstall.click();
  await expect(page.getByText(/Installed .*demo/i)).toBeVisible();
  // Navigates to Apps
  await expect(page.getByRole('heading', { name: 'Apps' })).toBeVisible();
});

test('install from YAML (demo) shows success and navigates to Apps', async ({ page }) => {
  await page.goto('/#/apps/catalog');
  await page.getByRole('textbox').fill('name: demo-app\nimage: alpine:latest\n');
  await page.getByRole('button', { name: 'Install' }).nth(1).click();
  await expect(page.getByText(/Installed .*demo/i)).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Apps' })).toBeVisible();
});

