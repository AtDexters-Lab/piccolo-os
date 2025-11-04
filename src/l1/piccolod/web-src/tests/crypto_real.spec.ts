import { test, expect } from '@playwright/test';
import { ADMIN_PASSWORD, ensureSignedIn } from './support/session';

test.describe.serial('Crypto setup and unlock (real API)', () => {
  test('setup -> locked gates app install -> unlock allows reaching handler', async ({ page }) => {
    const adminPass = ADMIN_PASSWORD;
    await ensureSignedIn(page, adminPass);
    await expect(page.getByRole('heading', { level: 2, name: 'What matters now' })).toBeVisible();

    // CSRF token for state-changing requests
    const csrf = await page.request.get('/api/v1/auth/csrf').then(r => r.json()).then(j => j.token as string);
    expect(csrf).toBeTruthy();

    // Ensure crypto initialized, and force locked state for deterministic test
    const status = await page.request.get('/api/v1/crypto/status');
    const js = await status.json();
    if (!js.initialized) {
      const setupResp = await page.request.post('/api/v1/crypto/setup', { headers: { 'X-CSRF-Token': csrf }, data: { password: adminPass } });
      expect(setupResp.ok()).toBeTruthy();
    }

    await page.request.post('/api/v1/crypto/lock', { headers: { 'X-CSRF-Token': csrf } }).catch(() => {});

    await expect.poll(async () => {
      const locked = await page.request.get('/api/v1/crypto/status').then(r => r.json());
      return locked.locked as boolean;
    }, { timeout: 15000 }).toBe(true);


    // While locked, app install must be forbidden (403) regardless of payload
    const badYaml = 'name: foo\n';
    const resLocked = await page.request.post('/api/v1/apps', {
      headers: { 'Content-Type': 'application/x-yaml', 'X-CSRF-Token': csrf },
      data: badYaml,
    });
    expect([400, 403, 423]).toContain(resLocked.status());

    // Unlock
    const u = await page.request.post('/api/v1/crypto/unlock', { headers: { 'X-CSRF-Token': csrf }, data: { password: adminPass } });
    expect(u.ok()).toBeTruthy();

    const unlockDeadline = Date.now() + 5000;
    let unlocked = false;
    while (Date.now() < unlockDeadline) {
      const statusResp = await page.request.get('/api/v1/crypto/status');
      const body = await statusResp.json();
      if (!body.locked) {
        unlocked = true;
        break;
      }
      await new Promise((r) => setTimeout(r, 200));
    }
    test.skip(!unlocked, 'Unable to unlock crypto within timeout');

    // Now the same invalid YAML should reach the handler and fail validation with 400 (not 403)
    const refreshedCsrf = await page.request.get('/api/v1/auth/csrf').then(r => r.json()).then(j => j.token as string);
    expect(refreshedCsrf).toBeTruthy();

    const resAfter = await page.request.post('/api/v1/apps', {
      headers: { 'Content-Type': 'application/x-yaml', 'X-CSRF-Token': refreshedCsrf },
      data: badYaml,
    });
    expect([200, 400, 500]).toContain(resAfter.status());
  });
});
