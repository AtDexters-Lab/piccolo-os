import { test, expect } from '@playwright/test';

test.describe('Catalog + YAML install (real API)', () => {
  const adminPass = 'password';

  test('validate and attempt install from catalog template', async ({ page }) => {
    // Setup admin + login
    await page.request.post('/api/v1/auth/setup', { data: { password: adminPass } }).catch(() => {});
    await page.goto('/#/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill(adminPass);
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.locator('h2')).toHaveText('Dashboard');

    // Ensure crypto initialized and locked
    const csrf = await page.request.get('/api/v1/auth/csrf').then(r => r.json()).then(j => j.token as string);
    const st = await page.request.get('/api/v1/crypto/status').then(r => r.json());
    if (!st.initialized) await page.request.post('/api/v1/crypto/setup', { headers: { 'X-CSRF-Token': csrf }, data: { password: adminPass } });
    const st2 = await page.request.get('/api/v1/crypto/status').then(r => r.json());
    if (!st2.locked) await page.request.post('/api/v1/crypto/lock', { headers: { 'X-CSRF-Token': csrf } });

    // Load catalog + template
    const cat = await page.request.get('/api/v1/catalog').then(r => r.json());
    expect(Array.isArray(cat.apps)).toBeTruthy();
    const tpl = await page.request.get('/api/v1/catalog/vaultwarden/template');
    expect(tpl.ok()).toBeTruthy();
    const yaml = await tpl.text();
    expect(yaml).toContain('name: vaultwarden');

    // Validate YAML
    const v = await page.request.post('/api/v1/apps/validate', { headers: { 'Content-Type': 'application/x-yaml', 'X-CSRF-Token': csrf }, data: yaml });
    expect(v.ok()).toBeTruthy();

    // Attempt install while locked → 403
    const lockedResp = await page.request.post('/api/v1/apps', { headers: { 'Content-Type': 'application/x-yaml', 'X-CSRF-Token': csrf }, data: yaml });
    expect(lockedResp.status()).toBe(403);

    // Unlock and attempt install → expect failure (podman missing) but not 403; accept 400/500
    const u = await page.request.post('/api/v1/crypto/unlock', { headers: { 'X-CSRF-Token': csrf }, data: { password: adminPass } });
    expect(u.ok()).toBeTruthy();
    const install = await page.request.post('/api/v1/apps', { headers: { 'Content-Type': 'application/x-yaml', 'X-CSRF-Token': csrf }, data: yaml });
    expect([400, 500]).toContain(install.status());
  });
});

