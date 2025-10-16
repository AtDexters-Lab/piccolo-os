import { test, expect } from '@playwright/test';
import { ADMIN_PASSWORD, ensureSignedIn } from './support/session';

test.describe('Dashboard panels (real-backed status)', () => {
  test('renders OS/Remote/Storage panels with real API data', async ({ page }) => {
    // Ensure admin is set up for real auth flows; ignore if already initialized
    await ensureSignedIn(page, ADMIN_PASSWORD);
    await expect(page.locator('h2', { hasText: 'Dashboard' })).toBeVisible();

    const updatesPanel = page.getByRole('heading', { name: 'Updates' }).first().locator('xpath=..');
    await expect(updatesPanel).toBeVisible();
    await expect(updatesPanel.locator('p').first()).toHaveText(/OS:|Failed to load updates|not found/i, { timeout: 5000 });

    const remotePanel = page.getByRole('heading', { name: 'Remote Access' }).first().locator('xpath=..');
    await expect(remotePanel).toBeVisible();
    await expect(remotePanel).toContainText(/Remote access is disabled|ACTIVE|PROVISIONING|PREFLIGHT_REQUIRED|WARNING|ERROR|No portal host/i, { timeout: 5000 });

    const storagePanel = page.getByRole('heading', { name: 'Storage' }).first().locator('xpath=..');
    await expect(storagePanel).toBeVisible();
    await expect(storagePanel).toContainText(/disks detected|Failed to load storage/, { timeout: 5000 });
  });
});
