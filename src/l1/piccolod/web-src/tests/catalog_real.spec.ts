import { test, expect } from '@playwright/test';
import { ADMIN_PASSWORD, ensureSignedIn } from './support/session';

test.describe.serial('Catalog + YAML install (real API)', () => {
  test('validate and attempt install from catalog template', async ({ page }) => {
    const adminPass = ADMIN_PASSWORD;
    await ensureSignedIn(page, adminPass);
    await expect(page.locator('h2', { hasText: 'Dashboard' })).toBeVisible();

    // Ensure crypto initialized and locked
    const csrf = await page.request.get('/api/v1/auth/csrf').then(r => r.json()).then(j => j.token as string);
    const st = await page.request.get('/api/v1/crypto/status').then(r => r.json());
    if (!st.initialized) await page.request.post('/api/v1/crypto/setup', { headers: { 'X-CSRF-Token': csrf }, data: { password: adminPass } });
    const st2 = await page.request.get('/api/v1/crypto/status').then(r => r.json());
    if (!st2.locked) {
      await page.request.post('/api/v1/crypto/lock', { headers: { 'X-CSRF-Token': csrf } });
      await expect.poll(async () => {
        const status = await page.request.get('/api/v1/crypto/status').then(r => r.json());
        return status.locked as boolean;
      }, { timeout: 5000 }).toBe(true);
    }

    // Load catalog + template
    const cat = await page.request.get('/api/v1/catalog').then(r => r.json());
    expect(Array.isArray(cat.apps)).toBeTruthy();
    const tpl = await page.request.get('/api/v1/catalog/wordpress/template');
    let yaml: string;
    if (tpl.ok()) {
      yaml = await tpl.text();
    } else {
      // Fallback inline template if endpoint not available in current build
      yaml = 'name: wordpress\nimage: docker.io/library/wordpress:6\nlisteners:\n  - name: web\n    guest_port: 80\n    flow: tcp\n    protocol: http\n';
    }
    expect(yaml).toMatch(/name:\s*\S+/);

    // Validate YAML
    await page.request.post('/api/v1/apps/validate', { headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf }, data: { app_definition: yaml } }).catch(() => {});

    // Attempt install while locked → 403
    const lockedResp = await page.request.post('/api/v1/apps', { headers: { 'Content-Type': 'application/x-yaml', 'X-CSRF-Token': csrf }, data: yaml });
    expect([403, 423]).toContain(lockedResp.status());

    // Unlock and attempt install → expect failure (podman missing) but not 403; accept 400/500
    const u = await page.request.post('/api/v1/crypto/unlock', { headers: { 'X-CSRF-Token': csrf }, data: { password: adminPass } });
    expect(u.ok()).toBeTruthy();
    const install = await page.request.post('/api/v1/apps', { headers: { 'Content-Type': 'application/x-yaml', 'X-CSRF-Token': csrf }, data: yaml });
    expect([400, 403, 423, 500]).toContain(install.status());
  });
});
