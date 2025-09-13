import { test, expect } from '@playwright/test';

test.describe('Crypto recovery + rewrap (real API)', () => {
  const adminPass = 'password';
  const newPass = 'password2';

  test('generate recovery, unlock via recovery, and rewrap on password change', async ({ page }) => {
    // Setup admin and login
    await page.request.post('/api/v1/auth/setup', { data: { password: adminPass } }).catch(() => {});
    await page.goto('/#/login');
    await page.getByLabel('Username').fill('admin');
    await page.getByLabel('Password').fill(adminPass);
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.locator('h2')).toHaveText('Dashboard');

    const csrf = await page.request.get('/api/v1/auth/csrf').then(r => r.json()).then(j => j.token as string);

    // Ensure crypto initialized and unlocked
    const st = await page.request.get('/api/v1/crypto/status').then(r => r.json());
    if (!st.initialized) await page.request.post('/api/v1/crypto/setup', { headers: { 'X-CSRF-Token': csrf }, data: { password: adminPass } });
    const st2 = await page.request.get('/api/v1/crypto/status').then(r => r.json());
    if (st2.locked) await page.request.post('/api/v1/crypto/unlock', { headers: { 'X-CSRF-Token': csrf }, data: { password: adminPass } });

    // Generate recovery key if not present
    const rk0 = await page.request.get('/api/v1/crypto/recovery-key').then(r => r.json());
    let words: string[] = [];
    if (!rk0.present) {
      const gen = await page.request.post('/api/v1/crypto/recovery-key/generate', { headers: { 'X-CSRF-Token': csrf } });
      expect(gen.ok()).toBeTruthy();
      const j = await gen.json();
      words = j.words;
      expect(Array.isArray(words) && words.length > 0).toBeTruthy();
    }

    // Lock and unlock via recovery key
    await page.request.post('/api/v1/crypto/lock', { headers: { 'X-CSRF-Token': csrf } });
    const rk = words.length ? words.join(' ') : (await page.request.post('/api/v1/crypto/recovery-key/generate', { headers: { 'X-CSRF-Token': csrf } }).then(r => r.json())).words.join(' ');
    const u = await page.request.post('/api/v1/crypto/unlock', { headers: { 'X-CSRF-Token': csrf }, data: { recovery_key: rk } });
    expect(u.ok()).toBeTruthy();

    // Rewrap: change admin password and ensure crypto unlock works with new password
    await page.request.post('/api/v1/auth/password', { headers: { 'X-CSRF-Token': csrf }, data: { old_password: adminPass, new_password: newPass } });
    await page.request.post('/api/v1/crypto/lock', { headers: { 'X-CSRF-Token': csrf } });
    const oldTry = await page.request.post('/api/v1/crypto/unlock', { headers: { 'X-CSRF-Token': csrf }, data: { password: adminPass } });
    expect(oldTry.status()).toBe(401);
    const newTry = await page.request.post('/api/v1/crypto/unlock', { headers: { 'X-CSRF-Token': csrf }, data: { password: newPass } });
    expect(newTry.ok()).toBeTruthy();
  });
});

