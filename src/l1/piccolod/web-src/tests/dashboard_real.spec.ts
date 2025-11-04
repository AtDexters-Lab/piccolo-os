import { test, expect } from '@playwright/test';
import { ADMIN_PASSWORD, ensureSignedIn } from './support/session';

test.describe('Dashboard panels (real-backed status)', () => {
  test('renders OS/Remote/Storage panels with real API data', async ({ page }) => {
    // Ensure admin is set up for real auth flows; ignore if already initialized
    await ensureSignedIn(page, ADMIN_PASSWORD);
    await expect(page.getByRole('heading', { level: 2, name: 'What matters now' })).toBeVisible();

    const systemPanel = page.getByTestId('home-widget-system');
    await expect(systemPanel).toBeVisible();
    await expect(systemPanel.getByRole('heading', { level: 3, name: 'Health' })).toBeVisible();

    const updatesPanel = page.getByTestId('home-widget-updates');
    await expect(updatesPanel).toBeVisible();
    await expect(updatesPanel.getByRole('heading', { level: 3, name: 'OS & apps' })).toBeVisible();
    await expect(updatesPanel).toContainText(/Running|Update|Unable to load updates|Check for updates/i, { timeout: 5000 });

    const networkPanel = page.getByTestId('home-widget-network');
    await expect(networkPanel).toBeVisible();
    await expect(networkPanel.getByRole('heading', { level: 3, name: 'Reachability' })).toBeVisible();
    await expect(networkPanel).toContainText(/Primary IP|Network diagnostics|Troubleshoot/i, { timeout: 5000 });

    const storagePanel = page.getByTestId('home-widget-storage');
    await expect(storagePanel).toBeVisible();
    await expect(storagePanel.getByRole('heading', { level: 3, name: 'Capacity' })).toBeVisible();
    await expect(storagePanel).toContainText(/disk|Unable to load storage|Add storage|Manage storage|No disks detected/i, { timeout: 5000 });
  });
});
