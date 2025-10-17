import { test, expect } from '@playwright/test';

test.describe('Unlock-first login UI (real API)', () => {
  const adminPass = 'password';

  test('shows unlock box immediately when locked and unlocks to dashboard', async ({ page, request, context }) => {
    // Ensure admin exists with a known password (idempotent setup)
    await request.post('/api/v1/auth/setup', { data: { password: adminPass } }).catch(() => {});

    // Initialize crypto if needed (public endpoint); after setup the device is locked by default
    const cs = await request.get('/api/v1/crypto/status').then(r => r.json());
    if (!cs.initialized) {
      await request.post('/api/v1/crypto/setup', { data: { password: adminPass } });
    }
    await request.post('/api/v1/auth/login', { data: { username: 'admin', password: adminPass } });
    const csrf = await request.get('/api/v1/auth/csrf').then(r => r.json()).then(j => j.token as string);
    await request.post('/api/v1/crypto/lock', { headers: { 'X-CSRF-Token': csrf } }).catch(() => {});
    await context.clearCookies();

    const locked = await request.get('/api/v1/crypto/status').then(r => r.json()).then(j => j.locked as boolean);
    test.skip(!locked, 'Unable to force locked crypto state for unlock-first scenario');

    // Go to login; expect unlock panel immediately (without attempting to sign in)
    await page.goto('/#/login');
    await expect(page.getByText('Device is locked')).toBeVisible();

    // Unlock with admin password
    await page.getByPlaceholder('admin password').fill(adminPass);
    await page.getByRole('button', { name: 'Unlock' }).click();

    // After unlock, UI should redirect to dashboard (auto-session)
    await page.waitForURL(/#\/$/, { timeout: 10000 });
    await expect(page.locator('h2')).toContainText('Dashboard');
  });
});
