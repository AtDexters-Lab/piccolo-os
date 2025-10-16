import { test, expect } from '@playwright/test';
import { ADMIN_PASSWORD, ensureSignedIn } from './support/session';

test.describe('Dashboard panels (real-backed status)', () => {
  test('renders OS/Remote/Storage panels with real API data', async ({ page }) => {
    // Ensure admin is set up for real auth flows; ignore if already initialized
    await ensureSignedIn(page, ADMIN_PASSWORD);
    await expect(page.locator('h2', { hasText: 'Dashboard' })).toBeVisible();

    // Updates panel (real /updates/os)
    await expect(page.getByText(/OS:\s+.*→\s+.*/)).toBeVisible();

    // Remote panel (real /remote/status)
    await expect(page.getByText('Remote Access')).toBeVisible();
    await expect(page.getByText('Disabled')).toBeVisible();

    // Storage panel (real /storage/disks for disks count; mounts still from demo)
    const storagePanel = page.locator('h3', { hasText: 'Storage' }).locator('xpath=..');
    await expect(storagePanel).toBeVisible();
    await expect(storagePanel.getByText(/disks detected/)).toBeVisible();
  });
});
