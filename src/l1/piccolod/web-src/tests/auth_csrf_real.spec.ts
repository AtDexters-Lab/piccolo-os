import { test, expect } from '@playwright/test';

test.describe('Auth + CSRF enforcement (real API)', () => {
  const adminPass = 'password';

  test('state-changing requests require session + CSRF', async ({ page }) => {
    // Setup admin and login via UI
    await page.request.post('/api/v1/auth/setup', { data: { password: adminPass } }).catch(() => {});
    await page.goto('/#/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill(adminPass);
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.locator('h2')).toHaveText('Dashboard');

    // Prepare invalid YAML to avoid container work later
    const badYaml = 'name: foo\n';

    // 1) Without CSRF header: expect 403 (auth ok, CSRF enforced)
    const resNoCsrf = await page.request.post('/api/v1/apps', {
      headers: { 'Content-Type': 'application/x-yaml' },
      data: badYaml,
    });
    expect(resNoCsrf.status()).toBe(403);

    // 2) With CSRF header: expect validation error (400) instead of 403
    const csrf = await page.request.get('/api/v1/auth/csrf').then(r => r.json()).then(j => j.token as string);
    expect(csrf).toBeTruthy();
    const resWithCsrf = await page.request.post('/api/v1/apps', {
      headers: { 'Content-Type': 'application/x-yaml', 'X-CSRF-Token': csrf },
      data: badYaml,
    });
    expect(resWithCsrf.status()).toBe(400);
  });
});

