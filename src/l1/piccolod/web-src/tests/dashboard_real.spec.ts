import { test, expect } from '@playwright/test';

test.describe('Dashboard panels (real-backed status)', () => {
  test('renders OS/Remote/Storage panels with real API data', async ({ page, request }) => {
    // Ensure admin is set up for real auth flows; ignore if already initialized
    await request.post('/api/v1/auth/setup', { data: { password: 'password' } }).catch(() => {});

    // Login via UI to establish session
    await page.goto('/#/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill('password');
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.locator('h2')).toHaveText('Dashboard');

    // Updates panel (real /updates/os)
    await expect(page.getByText(/OS:\s+.*→\s+.*/)).toBeVisible();

    // Remote panel (real /remote/status)
    await expect(page.getByText('Remote Access')).toBeVisible();
    await expect(page.getByText('Disabled')).toBeVisible();

    // Storage panel (real /storage/disks for disks count; mounts still from demo)
    const storagePanel = page.locator('h3', { hasText: 'Storage' }).locator('xpath=..');
    await expect(storagePanel).toBeVisible();
    await expect(storagePanel.getByText(/disks;\s+\d+\s+mounts/)).toBeVisible();
  });
});
