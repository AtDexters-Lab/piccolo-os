import { test, expect } from '@playwright/test';

// Fail tests on any browser console error
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

test.describe('Backup & Restore (demo)', () => {
  test('import config via demo simulation shows toast', async ({ page }) => {
    await page.goto('/#/backup');
    const btn = page.getByRole('button', { name: 'Simulate Import' });
    await btn.click();
    await expect(page.getByText('Configuration import applied successfully')).toBeVisible();
  });

  test('per-app backup and restore show toasts', async ({ page }) => {
    await page.goto('/#/apps/vaultwarden');
    await expect(page.locator('h2')).toHaveText(/App: vaultwarden/);
    await page.getByRole('button', { name: 'Backup app' }).click();
    await expect(page.getByText("Backup created for app 'vaultwarden'"))
      .toBeVisible();
    page.once('dialog', (dialog) => dialog.accept());
    await page.getByRole('button', { name: 'Restore app' }).click();
    await expect(page.getByText("Restore started for app 'vaultwarden'"))
      .toBeVisible();
  });
});

test('restore app confirmation flow', async ({ page }) => {
  await page.goto('/#/apps/vaultwarden');
  page.once('dialog', (dialog) => dialog.accept());
  await page.getByRole('button', { name: 'Restore app' }).click();
  await expect(page.getByText("Restore started for app 'vaultwarden'"))
    .toBeVisible();
});
