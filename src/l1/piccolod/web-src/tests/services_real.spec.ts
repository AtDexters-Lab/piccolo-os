import { test, expect } from '@playwright/test';

test.describe('Service discovery (real API)', () => {
  const adminPass = 'password';

  test('list services and app services shape', async ({ page }) => {
    // Setup admin and login
    await page.request.post('/api/v1/auth/setup', { data: { password: adminPass } }).catch(() => {});
    await page.goto('/#/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill(adminPass);
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.locator('h2')).toHaveText('Dashboard');

    // GET /services returns array; items (if any) include host_port and optional local_url
    const all = await page.request.get('/api/v1/services').then(r => r.json());
    expect(Array.isArray(all.services)).toBeTruthy();
    if (all.services.length > 0) {
      const s = all.services[0];
      expect(typeof s.name).toBe('string');
      expect(typeof s.guest_port).toBe('number');
      expect(typeof s.host_port).toBe('number');
      expect(['string', 'object']).toContain(typeof s.local_url); // string or null
    }

    // Unknown app services returns 404
    const r404 = await page.request.get('/api/v1/apps/unknown-app/services');
    expect(r404.status()).toBe(404);
  });
});

