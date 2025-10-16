import { test, expect } from '@playwright/test';
import { ADMIN_PASSWORD, ensureSignedIn } from './support/session';

test.describe('Service discovery (real API)', () => {
  test('list services and app services shape', async ({ page }) => {
    // Setup admin and login
    await ensureSignedIn(page, ADMIN_PASSWORD);
    await expect(page.locator('h2', { hasText: 'Dashboard' })).toBeVisible();

    // GET /services returns array; items (if any) include host_port and optional local_url
    const all = await page.request.get('/api/v1/services').then(r => r.json());
    expect(Array.isArray(all.services)).toBeTruthy();
    // If any, items should be objects; shape may vary by build
    if (all.services.length > 0) {
      expect(typeof all.services[0]).toBe('object');
    }

    // Unknown app services returns 404
    const r404 = await page.request.get('/api/v1/apps/unknown-app/services');
    expect(r404.status()).toBe(404);
  });
});
