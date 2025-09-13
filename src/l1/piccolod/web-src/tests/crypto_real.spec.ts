import { test, expect } from '@playwright/test';

test.describe('Crypto setup and unlock (real API)', () => {
  const adminPass = 'password';

  test('setup -> locked gates app install -> unlock allows reaching handler', async ({ page }) => {
    // Ensure admin exists
    await page.request.post('/api/v1/auth/setup', { data: { password: adminPass } }).catch(() => {});
    // Login to establish session cookies
    await page.goto('/#/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill(adminPass);
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.locator('h2')).toHaveText('Dashboard');

    // Setup crypto (locked by default)
    const setupResp = await page.request.post('/api/v1/crypto/setup', { data: { password: adminPass } });
    expect(setupResp.ok()).toBeTruthy();

    // Session should report volumes locked
    const s1 = await page.request.get('/api/v1/auth/session');
    const js1 = await s1.json();
    expect(js1.authenticated).toBeTruthy();
    expect(js1.volumes_locked).toBeTruthy();

    // CSRF token for state-changing requests
    const csrf = await page.request.get('/api/v1/auth/csrf').then(r => r.json()).then(j => j.token as string);
    expect(csrf).toBeTruthy();

    // While locked, app install must be forbidden (403) regardless of payload
    const badYaml = 'name: foo\n';
    const resLocked = await page.request.post('/api/v1/apps', {
      headers: { 'Content-Type': 'application/x-yaml', 'X-CSRF-Token': csrf },
      data: badYaml,
    });
    expect(resLocked.status()).toBe(403);

    // Unlock
    const u = await page.request.post('/api/v1/crypto/unlock', { data: { password: adminPass } });
    expect(u.ok()).toBeTruthy();

    // Session should report unlocked
    const s2 = await page.request.get('/api/v1/auth/session');
    const js2 = await s2.json();
    expect(js2.volumes_locked).toBeFalsy();

    // Now the same invalid YAML should reach the handler and fail validation with 400 (not 403)
    const resAfter = await page.request.post('/api/v1/apps', {
      headers: { 'Content-Type': 'application/x-yaml', 'X-CSRF-Token': csrf },
      data: badYaml,
    });
    expect(resAfter.status()).toBe(400);
  });
});

