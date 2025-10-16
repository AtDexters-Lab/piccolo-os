import { test, expect } from '@playwright/test';
import { ADMIN_PASSWORD, ensureSignedIn } from './support/session';

test.describe('Crypto setup and unlock (real API)', () => {
  test('setup -> locked gates app install -> unlock allows reaching handler', async ({ page }) => {
    const adminPass = ADMIN_PASSWORD;
    await ensureSignedIn(page, adminPass);
    await expect(page.locator('h2', { hasText: 'Dashboard' })).toBeVisible();

    // CSRF token for state-changing requests
    const csrf = await page.request.get('/api/v1/auth/csrf').then(r => r.json()).then(j => j.token as string);
    expect(csrf).toBeTruthy();

    // Ensure crypto initialized, and force locked state for deterministic test
    const status = await page.request.get('/api/v1/crypto/status');
    const js = await status.json();
    if (!js.initialized) {
      const setupResp = await page.request.post('/api/v1/crypto/setup', { headers: { 'X-CSRF-Token': csrf }, data: { password: adminPass } });
      expect(setupResp.ok()).toBeTruthy();
    } else if (!js.locked) {
      const lockResp = await page.request.post('/api/v1/crypto/lock', { headers: { 'X-CSRF-Token': csrf } });
      expect(lockResp.ok()).toBeTruthy();
    }

    // Session should report volumes locked
    const lockedState = await page.request.get('/api/v1/crypto/status').then(r => r.json());
    expect(lockedState.locked).toBeTruthy();


    // While locked, app install must be forbidden (403) regardless of payload
    const badYaml = 'name: foo\n';
    const resLocked = await page.request.post('/api/v1/apps', {
      headers: { 'Content-Type': 'application/x-yaml', 'X-CSRF-Token': csrf },
      data: badYaml,
    });
    expect(resLocked.status()).toBe(403);

    // Unlock
    const u = await page.request.post('/api/v1/crypto/unlock', { headers: { 'X-CSRF-Token': csrf }, data: { password: adminPass } });
    expect(u.ok()).toBeTruthy();

    await expect.poll(async () => {
      const statusResp = await page.request.get('/api/v1/crypto/status');
      const body = await statusResp.json();
      return body.locked as boolean;
    }, { timeout: 5000 }).toBe(false);

    // Now the same invalid YAML should reach the handler and fail validation with 400 (not 403)
    const refreshedCsrf = await page.request.get('/api/v1/auth/csrf').then(r => r.json()).then(j => j.token as string);
    expect(refreshedCsrf).toBeTruthy();

    const resAfter = await page.request.post('/api/v1/apps', {
      headers: { 'Content-Type': 'application/x-yaml', 'X-CSRF-Token': refreshedCsrf },
      data: badYaml,
    });
    expect(resAfter.status()).not.toBe(403);
  });
});
